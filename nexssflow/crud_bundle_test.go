package nexssflow_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

func TestCRUDCompositionThroughSrcpack(t *testing.T) {
	t.Parallel()
	cfg, err := runner.BuildConfig(append(native.Bundles(), nexssflow.FullBundle(nil)))
	ktest.RequireNoError(t, err)
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	source := `@include "bundles/crud/crud.nflow"
@pipeline custom_records
	crud.pipeline.crud
@end
pipeline.custom_records`
	path := filepath.Join(t.TempDir(), "records.yml")
	// No app/schema; the host's config adapter must not leak into generic CRUD.
	value := map[string]any{"arbitrary": []any{1, "data", false}}
	ex, err := runner.Execute(ctx, cfg, source, "records-composition.nflow", map[string]any{
		"operation": "save", "path": path, "name": "demo", "value": value,
	})
	ktest.RequireNoError(t, err)
	output, ok := ex.Output.(map[string]any)
	if !ok {
		t.Fatalf("save output has type %T, want map[string]any", ex.Output)
	}
	ktest.RequireEqual(t, output["name"], any("demo"))
	ex, err = runner.Execute(ctx, cfg, source, "records-composition.nflow", map[string]any{
		"operation": "get", "path": path, "name": "demo",
	})
	ktest.RequireNoError(t, err)
	output, ok = ex.Output.(map[string]any)
	if !ok {
		t.Fatalf("get output has type %T, want map[string]any", ex.Output)
	}
	ktest.RequireEqual(t, output["value"], any(value))
	_, _, err = nflows.Loader().Load("host.nflow", "bundles/other/crud.nflow")
	ktest.RequireCondition(t, err != nil, "unknown namespace must not alias CRUD")
}
