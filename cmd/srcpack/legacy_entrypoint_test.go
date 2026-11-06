package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	tclinexssflow "github.com/nexssp/transport/tcli/nexssflow"

	srcpackflow "github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

// executeRootPipeline passes resolved domain input and compile-time config to
// the unchanged embedded source. Only srcpack.nflow chooses a command pipeline.
func executeRootPipeline(ctx context.Context, cli cliOptions, run resolved) (runner.Execution, error) {
	var zero runner.Execution
	source, err := nflows.FS.ReadFile("runtime.nflow")
	if err != nil {
		return zero, fmt.Errorf("read embedded runtime.nflow: %w", err)
	}
	values, err := rootConfig(cli, run)
	if err != nil {
		return zero, err
	}
	payload, err := rootPayload(cli.Command, run)
	if err != nil {
		return zero, err
	}
	payload["io"], err = rootIO(cli, run)
	if err != nil {
		return zero, err
	}
	bundles := append(native.Bundles(), srcpackflow.FullBundle(nil), tclinexssflow.RoutingOnlyBundle())
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		return zero, fmt.Errorf("build runner config: %w", err)
	}
	cfg.CompileOpts = append(cfg.CompileOpts, compiler.WithConfigMap(values))
	ctx = compiler.WithSourceLoader(ctx, nflows.Loader())
	// No named EntryPoint: runner executes the root match(.command) expression.
	return runner.Execute(ctx, cfg, string(source), "runtime.nflow", payload)
}

// rootIO exposes facts, not decisions: output.nflow owns their interpretation.
func rootIO(cli cliOptions, run resolved) (map[string]any, error) {
	startedAt, err := strconv.ParseInt(run.Stats.StartedAt, 10, 64)
	if err != nil && run.Stats.StartedAt != "" {
		return nil, fmt.Errorf("decode start time: %w", err)
	}
	return map[string]any{
		"output_path": run.OutputPath, "explicit_output": cli.Config.Output,
		"config_path": run.ConfigPath, "copy": cli.Config.Copy,
		"quiet": cli.Config.Quiet, "check": cli.Check,
		"explicit_pack": cli.ExplicitPack, "started_at": startedAt,
	}, nil
}

func rootPayload(command string, run resolved) (map[string]any, error) {
	payload, err := jsonObject(run.Pack)
	if err != nil {
		return nil, fmt.Errorf("encode resolved pack request: %w", err)
	}
	// A present list must be [] rather than JSON null at Flow's schema boundary.
	for _, name := range []string{"targets", "files", "examples"} {
		if payload[name] == nil {
			payload[name] = []any{}
		}
	}
	payload["command"] = command
	return payload, nil
}

// Keep display labels separate from source input (notably targets and prompt)
// and from the actual output destination. Values are bound by Flow after parsing,
// never interpolated into source, so quotes/newlines cannot become DSL syntax.
func rootConfig(_ cliOptions, run resolved) (map[string]string, error) {
	values := make(map[string]string)
	for _, part := range []struct {
		prefix string
		value  any
	}{
		{"stats_", run.Stats},
		{"render_", run.Render},
	} {
		fields, err := jsonObject(part.value)
		if err != nil {
			return nil, fmt.Errorf("encode resolved NFlow config: %w", err)
		}
		for key, value := range fields {
			values[part.prefix+key] = configString(value)
		}
	}
	return values, nil
}

func jsonObject(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func configString(value any) string {
	if items, ok := value.([]string); ok {
		return strings.Join(items, ",")
	}
	return fmt.Sprint(value)
}
