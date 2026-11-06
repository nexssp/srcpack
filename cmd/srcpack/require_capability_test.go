package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/transport/tcli"

	"github.com/nexssp/srcpack/nflows"
)

func TestCLIRequiresSelectCapabilitiesRatherThanApplicationImports(t *testing.T) {
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	transport := tcli.New(tcli.WithArgs("--help"), tcli.WithOutput(io.Discard, io.Discard))
	cfg, err := cliRunnerConfig(ctx, "@require tcli\ntcli.listen", transport)
	ktest.RequireNoError(t, err)
	_, ok := cfg.Resolver.Action("tcli.listen")
	ktest.RequireCondition(t, ok, "explicit tcli requirement must mount listener")
	for _, name := range []string{"init.inspect", "packcmd.resolve", "manage.load", "system.help"} {
		_, ok := cfg.Resolver.Action(name)
		ktest.RequireCondition(t, !ok, "%s was mounted merely because Go imported the package", name)
	}
	_, err = runner.Compile(ctx, cfg, "@require tcli\ntcli.listen", "require-tcli.nflow")
	ktest.RequireNoError(t, err)
}

func TestProductionMissingTcliRequirementFailsCompilation(t *testing.T) {
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	body, err := nflows.FS.ReadFile("srcpack.nflow")
	ktest.RequireNoError(t, err)
	source := string(body)
	ktest.RequireCondition(t, strings.Contains(source, "@require tcli\n"), "production source must declare tcli explicitly")
	transport := tcli.New(tcli.WithOutput(io.Discard, io.Discard))
	cfg, err := cliRunnerConfig(ctx, source, transport)
	ktest.RequireNoError(t, err)
	_, err = runner.Compile(ctx, cfg, source, "srcpack.nflow")
	ktest.RequireNoError(t, err)
	source = strings.Replace(source, "@require tcli\n", "", 1)
	cfg, err = cliRunnerConfig(ctx, source, transport)
	ktest.RequireNoError(t, err)
	_, ok := cfg.Resolver.Action("tcli.listen")
	ktest.RequireCondition(t, !ok, "host must not hide a missing @require tcli")
	_, err = runner.Compile(ctx, cfg, source, "srcpack.nflow")
	ktest.RequireCondition(t, err != nil, "missing capability must fail compilation")
}

func TestCLIUnknownRequiredCapabilityIsNotIgnored(t *testing.T) {
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	_, err := cliRunnerConfig(ctx, "@require srcpack_missing_capability\nruntime.noop", tcli.New())
	ktest.RequireCondition(t, err != nil, "unavailable declared bundle must fail")
	ktest.RequireStringContains(t, err.Error(), "srcpack_missing_capability")
}
