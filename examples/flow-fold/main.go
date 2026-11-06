// Command flow-fold demonstrates srcpack's production fold and a DSL-only extension.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	tclinexssflow "github.com/nexssp/transport/tcli/nexssflow"

	"github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

//go:embed report.nflow
var reportExtension string

const (
	extendedRenderMode = "extended-render"
	renderMode         = "render"
)

type demoResult struct {
	Baseline map[string]any
	Extended map[string]any
	Context  string
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "flow-fold demo:", err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	root, err := os.MkdirTemp("", "srcpack-flow-fold-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if fixtureErr := writeFixture(root); fixtureErr != nil {
		return fixtureErr
	}
	result, err := executeDemo(context.Background(), root)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "SRCPACK + Flow: build once, extend in the DSL")
	fmt.Fprintln(out, "srcpack.source -> ast.parse -> pack.stats_input -> pack.batch -> pack_fold.@pack_stats_fold")
	for _, section := range []struct {
		label string
		value map[string]any
	}{
		{"1. Production fold (original file records retained for rendering)", result.Baseline},
		{"2. Same fold + report.nflow (no new Go operator)", result.Extended},
	} {
		data, err := json.MarshalIndent(section.value, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "\n%s\n%s\n", section.label, data)
	}
	fmt.Fprintf(out, "\n3. The same fold still feeds srcpack's renderer\n%s\n", result.Context)
	fmt.Fprintln(out, "The demo writes only a temporary fixture, removes it on exit, and never uses the clipboard.")
	return nil
}

func writeFixture(root string) error {
	files := map[string]string{
		"hello.go":      "package demo\n\n// Café returns a greeting.\nfunc Café() string { return \"café\" }\n",
		"math.go":       "package demo\n\nfunc Add(a, b int) int { return a + b }\n",
		"hello_test.go": "package demo\n\nfunc TestHello() {}\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func executeDemo(ctx context.Context, root string) (demoResult, error) {
	var result demoResult
	pack, err := nflows.FS.ReadFile("pack.nflow")
	if err != nil {
		return result, err
	}
	cfg, err := runner.BuildConfig(append(native.Bundles(), nexssflow.FullBundle(nil), tclinexssflow.RoutingOnlyBundle()))
	if err != nil {
		return result, err
	}
	// Absolute targets avoid changing the host process's working directory.
	payload := map[string]any{"targets": []string{root}, "no_tests": true, "only_sig": true}
	for _, mode := range []string{"baseline", "extended", renderMode, extendedRenderMode} {
		source, err := demoSource(string(pack), mode)
		if err != nil {
			return result, err
		}
		loaderCtx := compiler.WithSourceLoader(ctx, nflows.Loader())
		prepared, err := runner.Preflight(loaderCtx, cfg, source, "flow_fold_demo.nflow")
		if err != nil {
			return result, fmt.Errorf("%s preflight: %w", mode, err)
		}
		execution, err := runner.ExecutePrepared(loaderCtx, cfg, prepared, runner.EntryPoint{
			Name: "pipeline.pack", Payload: payload,
		})
		if err != nil {
			return result, fmt.Errorf("%s execution: %w", mode, err)
		}
		if mode == renderMode || mode == extendedRenderMode {
			text, ok := execution.Output.(string)
			if !ok {
				return result, fmt.Errorf("render output is %T, want string", execution.Output)
			}
			if mode == renderMode {
				result.Context = text
			} else if text != result.Context {
				return result, errors.New("DSL report extension changed the rendered context")
			}
			continue
		}
		report, err := reportFromOutput(execution.Output)
		if err != nil {
			return result, fmt.Errorf("%s: %w", mode, err)
		}
		// Hide opaque parser records only in the console presentation.
		// The untouched execution output still contains all original files.
		delete(report, "files")
		if mode == "baseline" {
			result.Baseline = report
		} else {
			result.Extended = report
		}
	}
	return result, nil
}

// demoSource keeps production schema/aggregation verbatim, omits cache writes,
// and replaces the presentation tail. Production CLI never rewrites its source.
// Guard the seam so pipeline changes fail loudly.
func demoSource(pack, mode string) (string, error) {
	const seam = "    -> stats.collect @{\n"
	if strings.Count(pack, seam) != 1 || !strings.Contains(pack, "-> pack_fold.@pack_stats_fold()") {
		return "", errors.New("production pack pipeline changed: expected one presentation seam after stream.fold")
	}
	head, _, _ := strings.Cut(pack, seam)
	// The demo must not update the host workspace cache. Production retains it.
	head = strings.Replace(head, "    -> workspace.save_state:enabled=$save_state\n", "", 1)
	// The production body is shared by write and preview pipelines via a macro.
	// The demo retains that body verbatim but declares one side-effect-free entry.
	head = strings.Replace(head, "@macro pack_body(save_state) {", "@pipeline pack", 1)
	var tail string
	switch mode {
	case "baseline":
		tail = "    -> collect\n"
	case "extended":
		tail = reportExtension + "\n    -> collect\n"
	case "render", "extended-render":
		tail = "    -> stats.collect:quiet:color=never\n    -> srcpack.render:editor=markdown\n    -> collect\n    -> srcpack.join\n"
		if mode == extendedRenderMode {
			tail = reportExtension + "\n" + tail
		}
	default:
		return "", fmt.Errorf("unknown demo mode %q", mode)
	}
	return head + tail + "@end\n", nil
}

func reportFromOutput(output any) (map[string]any, error) {
	items, ok := output.([]any)
	if !ok || len(items) != 1 {
		return nil, fmt.Errorf("fold output is %T (%v), want one collected envelope", output, output)
	}
	fields, ok := items[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fold envelope is %T, want map", items[0])
	}
	// Do not mutate Flow's returned envelope while formatting the demo.
	copied := make(map[string]any, len(fields))
	maps.Copy(copied, fields)
	return copied, nil
}
