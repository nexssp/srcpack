package nexssflow_test

import (
	"context"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

func TestTerminalBundleCompositionThroughSrcpack(t *testing.T) {
	t.Parallel()
	cfg, err := runner.BuildConfig(append(native.Bundles(), nexssflow.FullBundle(nil)))
	ktest.RequireNoError(t, err)
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	source := `@include "bundles/terminal/summary.nflow"
@pipeline custom_report
  { config: {}, stats: {file_count: 0, func_count: 0, raw_bytes: 0, packed_bytes: 0, estimated_tokens: 0, saved_percent: 0.0}, elapsed: "1ms" }
  -> summary.pipeline.terminal_summary
@end
pipeline.custom_report`
	ex, err := runner.Execute(ctx, cfg, source, "terminal-composition.nflow", nil)
	ktest.RequireNoError(t, err)
	out, ok := ex.Output.(map[string]any)
	ktest.RequireCondition(t, ok, "expected object, got %T", ex.Output)
	content, ok := out["content"].(string)
	if !ok {
		t.Fatalf("content has type %T, want string", out["content"])
	}
	ktest.RequireStringContains(t, content, "SRCPACK  Elapsed: 1ms")
}
