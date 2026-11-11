package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/extensions/require"
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
	const tcliRequirement = "@require github.com/nexssp/transport/tcli v0.10.0\n"
	ktest.RequireCondition(t, strings.Contains(source, tcliRequirement), "production source must declare tcli explicitly")
	transport := tcli.New(tcli.WithOutput(io.Discard, io.Discard))
	cfg, err := cliRunnerConfig(ctx, source, transport)
	ktest.RequireNoError(t, err)
	_, err = runner.Compile(ctx, cfg, source, "srcpack.nflow")
	ktest.RequireNoError(t, err)
	source = strings.Replace(source, tcliRequirement, "", 1)
	cfg, err = cliRunnerConfig(ctx, source, transport)
	ktest.RequireNoError(t, err)
	_, ok := cfg.Resolver.Action("tcli.listen")
	ktest.RequireCondition(t, !ok, "host must not hide a missing @require tcli")
	_, err = runner.Compile(ctx, cfg, source, "srcpack.nflow")
	ktest.RequireCondition(t, err != nil, "missing capability must fail compilation")
}

func TestPackageRootRequirementsResolve(t *testing.T) {
	tests := []struct {
		importPath string
		bundleID   string
	}{
		{"github.com/nexssp/srcpack", "srcpack"},
		{"github.com/nexssp/srcpack/bundles/cliinput", "srcpack_cli"},
		{"github.com/nexssp/srcpack/bundles/config_cli", "srcpack_config"},
		{"github.com/nexssp/srcpack/bundles/config_views", "config_views"},
		{"github.com/nexssp/srcpack/bundles/crud", "crud"},
		{"github.com/nexssp/srcpack/bundles/document", "document"},
		{"github.com/nexssp/srcpack/bundles/init", "srcpack_init"},
		{"github.com/nexssp/srcpack/bundles/manage", "srcpack_manage"},
		{"github.com/nexssp/srcpack/bundles/pack", "srcpack_pack"},
		{"github.com/nexssp/srcpack/bundles/system", "srcpack_system"},
		{"github.com/nexssp/srcpack/bundles/terminal", "terminal"},
		{"github.com/nexssp/srcpack/bundles/update", "srcpack_update"},
		{"github.com/nexssp/transport/tcli", "tcli"},
	}
	for _, tt := range tests {
		t.Run(tt.bundleID, func(t *testing.T) {
			bundles, err := require.ResolveBundles([]require.Requirement{{Import: tt.importPath}})
			ktest.RequireNoError(t, err)
			ktest.RequireCondition(t, len(bundles) == 1, "resolved %d bundles, want one", len(bundles))
			ktest.RequireCondition(t, bundles[0].ID == tt.bundleID, "resolved bundle ID %q, want %q", bundles[0].ID, tt.bundleID)
		})
	}
}

func TestCLIUnknownRequiredCapabilityIsNotIgnored(t *testing.T) {
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	_, err := cliRunnerConfig(ctx, "@require srcpack_missing_capability\nruntime.noop", tcli.New())
	ktest.RequireCondition(t, err != nil, "unavailable declared bundle must fail")
	ktest.RequireStringContains(t, err.Error(), "srcpack_missing_capability")
}
