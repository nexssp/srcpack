package nexssflow_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/extensions/require"
	"github.com/nexssp/flow/native"
	flowrunner "github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/kernel/xtest/ktest"
	"github.com/nexssp/transport/tcli"

	// Blank import triggers compiler.Register("srcpack", Bundle) in init().
	_ "github.com/nexssp/srcpack/nexssflow"
)

// TestBundleResolvesThroughRequireContract proves the @require path
// works end to end without native injection: preprocess a real .nflow,
// extract its @require declarations, resolve them through the same code
// path the CLI uses, compile the pipeline, run it.
//
// This is the SSOT test. If any operator is unregistered, misnamed, or
// unreachable via the module ID, this fails.
func TestBundleResolvesThroughRequireContract(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(filepath.Join("..", "tests", "fixtures", "nflows", "main.nflow"))
	ktest.RequireNoError(t, err)

	ctx := context.Background()

	// 1. Preprocess the .nflow to extract @require declarations.
	//    This is exactly what cli.buildConfig does.
	_, meta, err := compiler.Preprocess(ctx, native.Directives(), string(source), "main.nflow")
	ktest.RequireNoError(t, err)
	reqs, err := require.FromMeta(meta)
	ktest.RequireNoError(t, err)
	if len(reqs) == 0 {
		t.Fatal("main.nflow does not declare @require; the test would exercise nothing")
	}

	// 2. Resolve them through compiler.Lookup, the same lookup the CLI does.
	resolved, err := require.ResolveBundles(reqs)
	ktest.RequireNoError(t, err)
	if len(resolved) == 0 {
		t.Fatal("ResolveBundles returned no bundles")
	}

	// 3. Build config from native + resolved. No native injection of srcpack.
	bundles := append([]compiler.Bundle{}, native.Bundles()...)
	bundles = append(bundles, resolved...)
	cfg, err := flowrunner.BuildConfig(bundles)
	ktest.RequireNoError(t, err)

	// 4. Compile the pipeline. If any operator is unreachable by name,
	//    this fails.
	compiled, err := compiler.CompileAction(
		cfg.Resolver,
		cfg.Directives,
		cfg.Modifiers,
		cfg.Operators,
		cfg.Primaries,
	).Do(ctx, compiler.CompileReq{Source: string(source), Name: "main.nflow"})
	ktest.RequireNoError(t, err)
	if compiled.Program == nil {
		t.Fatal("compiled program is nil")
	}
}

func TestPipelineCLIBindingSurvivesMaterialization(t *testing.T) {
	t.Parallel()

	cliModifier := compiler.String("cli", func(b *action.Builder[any, any], raw string) *action.Builder[any, any] {
		cmd, desc, _ := strings.Cut(raw, ":")
		return b.Route(tcli.Command(strings.TrimSpace(cmd), strings.TrimSpace(desc)))
	})
	source := `
@pipeline pack:cli="pack:Package source context"
  runtime.noop
@end

pipeline.pack
`

	bundles := append(native.Bundles(), compiler.Bundle{
		ID:        "tcli_contract_test",
		Modifiers: []compiler.Modifier{cliModifier},
	})
	cfg, err := flowrunner.BuildConfig(bundles)
	ktest.RequireNoError(t, err)

	ex, err := flowrunner.Execute(context.Background(), cfg, source, "pack_cli.nflow", nil)
	ktest.RequireNoError(t, err)

	act, found := ex.Resolver.Action("pipeline.pack")
	ktest.RequireCondition(t, found, "pipeline.pack was not materialized")
	for _, binding := range act.GetBindings() {
		if cli, ok := binding.(tcli.CLIBinding); ok {
			ktest.RequireEqual(t, cli.Command, "pack")
			ktest.RequireEqual(t, cli.Description, "Package source context")
			return
		}
	}
	t.Fatalf("pipeline.pack did not preserve tcli binding; got bindings: %v", act.GetBindings())
}
