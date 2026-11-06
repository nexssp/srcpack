package document

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/xtest/ktest"
)

func TestBundleAndInterpolationContract(t *testing.T) {
	ktest.RequireNoError(t, compiler.ValidateBundle(Bundle(nil)))
	_, found := compiler.Lookup(ID)
	ktest.RequireCondition(t, found, "document bundle is not registered")
	for _, tc := range []struct{ source, want string }{
		{"${A}|${B}|${UNKNOWN}|${BROKEN", "${B}|value|${UNKNOWN}|${BROKEN"},
		{"${}|${A}${A}", "empty|${B}${B}"},
		{"\x00\xff", "\x00\xff"},
	} {
		ktest.RequireEqual(t, expandOnce(tc.source, map[string]string{"A": "${B}", "B": "value", "": "empty"}), tc.want)
	}
	valid := map[string]any{"blocks": []any{map[string]any{"parts": []any{
		map[string]any{"text": "${A}", "values": map[string]any{"A": "x"}, "expand": true, "newline": true},
		map[string]any{"text": "\x00\xff", "newline": true},
	}}}}
	out, err := interpolate(context.Background(), valid)
	ktest.RequireNoError(t, err)
	blocks, ok := out["blocks"].([]string)
	if !ok {
		t.Fatalf("blocks have type %T, want []string", out["blocks"])
	}
	ktest.RequireEqual(t, blocks, []string{"x\n\x00\xff\n"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = interpolate(ctx, valid)
	ktest.RequireCondition(t, errors.Is(err, context.Canceled), "cancellation must propagate")
	for _, bad := range []map[string]any{
		{},
		{"blocks": "wrong"},
		{"blocks": []any{42}},
		{"blocks": []any{map[string]any{"parts": "wrong"}}},
		{"blocks": []any{map[string]any{"parts": []any{42}}}},
		{"blocks": []any{map[string]any{"parts": []any{map[string]any{"text": 42}}}}},
		{"blocks": []any{map[string]any{"parts": []any{map[string]any{"text": "x", "expand": "yes"}}}}},
		{"blocks": []any{map[string]any{"parts": []any{map[string]any{"text": "x", "expand": true, "values": map[string]any{"A": 42}}}}}},
	} {
		_, err := interpolate(context.Background(), bad)
		ktest.RequireCondition(t, err != nil && strings.Contains(err.Error(), "document.interpolate"), "malformed input must fail with context")
	}
}
