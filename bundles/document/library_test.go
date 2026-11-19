package document

import (
	"context"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"
)

func TestBundleRetainsCompatibilityWithoutActions(t *testing.T) {
	bundle := Bundle(nil)
	ktest.RequireNoError(t, compiler.ValidateBundle(bundle))
	ktest.RequireCondition(t, len(bundle.Libraries) == 1, "document compatibility bundle must retain its library")
	ktest.RequireCondition(t, len(bundle.Libraries[0].Actions) == 0, "document bundle must not register Go actions")

	cfg, err := runner.BuildConfig(append(native.Bundles(), bundle))
	ktest.RequireNoError(t, err)
	_, found := cfg.Resolver.Action("document.interpolate")
	ktest.RequireCondition(t, !found, "document.interpolate must be implemented by NFlow, not registered as a Go action")
}

func TestInterpolationPipelinePreservesSinglePassAndBytes(t *testing.T) {
	cfg, err := runner.BuildConfig(native.Bundles())
	ktest.RequireNoError(t, err)
	ctx := compiler.WithSourceLoader(context.Background(), Loader())
	source := `@include "document.nflow" AS document
document.interpolation.pipeline.document_interpolate`
	payload := map[string]any{
		"blocks": []any{
			map[string]any{"parts": []any{
				map[string]any{
					"text":    "${A}|${B}|${UNKNOWN}|${BROKEN",
					"values":  map[string]any{"A": "${B}", "B": "value"},
					"expand":  true,
					"newline": true,
				},
				map[string]any{"text": string([]byte{0, 0xff}), "newline": true},
			}},
			map[string]any{"parts": []any{
				map[string]any{
					"text":   "${}|${A}${A}|${BROKEN${A}|${A",
					"values": map[string]any{"": "empty", "A": "ok"},
					"expand": true,
				},
				map[string]any{"text": "${A}", "values": map[string]any{"A": "changed"}, "expand": false},
			}},
			map[string]any{"parts": []any{
				map[string]any{"text": "already\n", "newline": true},
				map[string]any{"text": "", "newline": true},
			}},
		},
	}

	execution, err := runner.Execute(ctx, cfg, source, "document-interpolation-test.nflow", payload)
	ktest.RequireNoError(t, err)
	output, ok := execution.Output.(map[string]any)
	if !ok {
		t.Fatalf("interpolation output has type %T, want map[string]any", execution.Output)
	}
	blocks, ok := output["blocks"].([]any)
	if !ok {
		t.Fatalf("interpolation blocks have type %T, want []any", output["blocks"])
	}
	ktest.RequireEqual(t, blocks, []any{
		"${B}|value|${UNKNOWN}|${BROKEN\n\x00\xff\n",
		"empty|okok|${BROKEN${A}|${A${A}",
		"already\n",
	})
}
