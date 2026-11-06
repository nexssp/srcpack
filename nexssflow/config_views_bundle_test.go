package nexssflow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

func TestConfigViewsCompositionThroughSrcpack(t *testing.T) {
	t.Parallel()
	cfg, err := runner.BuildConfig(append(native.Bundles(), nexssflow.FullBundle(nil)))
	ktest.RequireNoError(t, err)
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	source := `@include "bundles/config_views/config_views.nflow"
@pipeline custom_presets
	config_views.pipeline.config_views
@end
pipeline.custom_presets`
	path := filepath.Join(t.TempDir(), "presets.yml")
	value := map[string]any{"enabled": true, "limits": map[string]any{"max": 7}}
	ex, err := runner.Execute(ctx, cfg, source, "preset-composition.nflow", map[string]any{
		"operation": "save", "path": path, "name": "demo", "app": "consumer", "schema": "v1", "value": value,
	})
	ktest.RequireNoError(t, err)
	output, ok := ex.Output.(map[string]any)
	if !ok {
		t.Fatalf("save output has type %T, want map[string]any", ex.Output)
	}
	ktest.RequireEqual(t, output["name"], any("demo"))
	ex, err = runner.Execute(ctx, cfg, source, "preset-composition.nflow", map[string]any{
		"operation": "get", "path": path, "name": "demo", "app": "consumer", "schema": "v1",
	})
	ktest.RequireNoError(t, err)
	output, ok = ex.Output.(map[string]any)
	if !ok {
		t.Fatalf("get output has type %T, want map[string]any", ex.Output)
	}
	ktest.RequireEqual(t, output["value"], any(value))
	_, _, err = nflows.Loader().Load("host.nflow", "bundles/other/config_views.nflow")
	ktest.RequireCondition(t, err != nil, "unknown bundle namespace must not alias config_views")
	data, err := os.ReadFile(path)
	ktest.RequireNoError(t, err)
	ktest.RequireStringContains(t, string(data), "config_views:")
}
