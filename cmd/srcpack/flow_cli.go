package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/extensions/require"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xerr"
	"github.com/nexssp/transport/tcli"
	tcliflow "github.com/nexssp/transport/tcli/nexssflow"

	"github.com/nexssp/srcpack/bundles/cliinput"
	_ "github.com/nexssp/srcpack/bundles/config_cli"
	_ "github.com/nexssp/srcpack/bundles/init"
	_ "github.com/nexssp/srcpack/bundles/manage"
	packcmd "github.com/nexssp/srcpack/bundles/pack"
	systemcmd "github.com/nexssp/srcpack/bundles/system"
	_ "github.com/nexssp/srcpack/bundles/update"
	_ "github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

// commandInvocation bridges argv to NFlow spelling policy. It never interprets
// a selector or dispatches a command; both are owned by srcpack pipelines.
func commandInvocation(ctx context.Context, args []string) ([]string, string, bool, error) {
	source, err := nflows.FS.ReadFile("srcpack.nflow")
	if err != nil {
		return nil, "", false, err
	}
	ctx = compiler.WithSourceLoader(ctx, nflows.Loader())
	if args == nil {
		args = []string{}
	}
	transport := tcli.New(tcli.WithArgs(), tcli.WithOutput(io.Discard, io.Discard))
	cfg, err := cliRunnerConfig(ctx, string(source), transport)
	if err != nil {
		return nil, "", false, err
	}
	ex, err := runner.Execute(ctx, cfg, string(source), "srcpack.nflow", map[string]any{
		"_srcpack_cli_normalize": true,
		"args":                   args,
	})
	if err != nil {
		return nil, "", false, err
	}
	out, ok := ex.Output.(map[string]any)
	if !ok {
		return nil, "", false, fmt.Errorf("CLI invocation pipeline returned %T", ex.Output)
	}
	var normalized []string
	switch values := out["args"].(type) {
	case []string:
		normalized = values
	case []any:
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, "", false, fmt.Errorf("CLI invocation argument has type %T", value)
			}
			normalized = append(normalized, text)
		}
	default:
		return nil, "", false, fmt.Errorf("CLI invocation args have type %T", out["args"])
	}
	command, _ := out["command"].(string)
	explicit, _ := out["explicit_pack"].(bool)
	return normalized, command, explicit, nil
}

func execute(ctx context.Context, args []string) error {
	_, err := runCommandEntrypoint(ctx, args)
	return err
}

func runCommandEntrypoint(ctx context.Context, args []string) (bool, error) {
	normalized, command, explicitPack, err := commandInvocation(ctx, args)
	if err != nil {
		return true, err
	}
	ctx, cleanup := packcmd.WithRemoteSession(ctx)
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "warning: remote source cleanup: %v\n", cleanupErr)
		}
	}()
	normalized = cliinput.NormalizeFlagNames(normalized)
	protected, marker := cliinput.ProtectCommas(normalized)
	var transportDiagnostics bytes.Buffer
	transport := tcli.New(tcli.WithArgs(protected...), tcli.WithExecutable("srcpack"), tcli.WithOutput(os.Stdout, &transportDiagnostics))
	ctx = cliinput.WithArguments(ctx, normalized)
	ctx = cliinput.WithRawArguments(ctx, protected[1:])
	ctx = cliinput.WithCommandLine(ctx, "srcpack", args)
	ctx = cliinput.WithInvocation(ctx, command, explicitPack, marker)
	ctx = systemcmd.WithVersion(ctx, version)
	ctx = compiler.WithSourceLoader(ctx, nflows.Loader())
	source, err := nflows.FS.ReadFile("srcpack.nflow")
	if err != nil {
		return true, err
	}
	cfg, err := cliRunnerConfig(ctx, string(source), transport)
	if err != nil {
		return true, fmt.Errorf("build CLI runner: %w", err)
	}
	_, err = runner.Execute(ctx, cfg, string(source), "srcpack.nflow", nil)
	if err != nil {
		err = withSelectorOrderHint(args, err)
	}
	flushTransportDiagnostics(os.Stderr, transportDiagnostics.String(), err)
	return true, err
}

const selectorOrderHint = "Selector parts are ordered path~exclude#profile; move '~exclude' before '#profile' (for example: ../flow~*.nflow#arch)."

func withSelectorOrderHint(args []string, err error) error {
	var appErr *xerr.AppError
	if !errors.As(err, &appErr) || appErr == nil || !strings.HasPrefix(appErr.Message, "unknown profile ") {
		return err
	}
	for _, arg := range args {
		profileAt := strings.IndexByte(arg, '#')
		excludeAt := strings.IndexByte(arg, '~')
		if profileAt >= 0 && excludeAt > profileAt {
			return runner.WithDiagnosticHint(err, selectorOrderHint)
		}
	}
	return err
}

// cliRunnerConfig mounts only the capabilities declared by the source. Imports
// register available factories; registration alone never mounts an application
// bundle. The host supplies transport I/O only after @require selects tcli.
func cliRunnerConfig(ctx context.Context, source string, transport *tcli.Transport) (runner.Config, error) {
	bundles := native.Bundles()
	base, err := runner.BuildConfig(bundles)
	if err != nil {
		return runner.Config{}, err
	}
	prepared, err := compiler.PrepareSource(ctx, base.Directives, source, "srcpack.nflow", base.CompileOpts...)
	if err != nil {
		return runner.Config{}, err
	}
	seen := make(map[string]bool, len(bundles))
	for i := range bundles {
		seen[bundles[i].ID] = true
	}
	requirements, err := require.FromMeta(prepared.Metadata())
	if err != nil {
		return runner.Config{}, fmt.Errorf("read require metadata: %w", err)
	}
	for i := range requirements {
		req := &requirements[i]
		selected, err := require.ResolveBundles([]require.Requirement{*req})
		if err != nil {
			return runner.Config{}, err
		}
		for i := range selected {
			bundle := &selected[i]
			if seen[bundle.ID] && req.Alias == "" {
				continue
			}
			if bundle.ID == "tcli" {
				bundle.Libraries[0] = tcliflow.LibraryWithTransport(transport)
			}
			bundles = append(bundles, *bundle)
			seen[bundle.ID] = true
		}
	}
	return runner.BuildConfig(bundles)
}
