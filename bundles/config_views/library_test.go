package config_views

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestGenericAndLegacy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yml")
	original := "unknown: {keep: yes}\nviews:\n  old: [a, b]\n"
	if err := os.WriteFile(p, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	call := func(m map[string]any) (map[string]any, error) { m["path"] = p; return Execute(context.Background(), m) }
	value := map[string]any{"nested": map[string]any{"items": []any{"a", 2}}}
	_, e := call(map[string]any{"operation": "save", "name": "n", "value": value, "app": "demo", "schema": "v1"})
	ktest.RequireNoError(t, e)
	_, e = call(map[string]any{"operation": "save", "name": "n", "value": value, "app": "demo", "schema": "v1"})
	ktest.RequireCondition(t, errors.Is(e, ErrConflict), "expected conflict: %v", e)
	_, e = call(map[string]any{"operation": "save", "name": "n", "value": value, "app": "demo", "schema": "v1", "replace": true})
	ktest.RequireNoError(t, e)
	got, e := call(map[string]any{"operation": "get", "name": "n", "app": "demo", "schema": "v1"})
	ktest.RequireNoError(t, e)
	ktest.RequireCondition(t, got["value"] != nil, "missing value")
	_, e = call(map[string]any{"operation": "get", "name": "n", "app": "other", "schema": "v1"})
	ktest.RequireCondition(t, errors.Is(e, ErrCompatibility), "expected mismatch")
	_, e = call(map[string]any{"operation": "save", "name": "", "value": []string(nil), "legacy": true})
	ktest.RequireNoError(t, e)
	data, e := os.ReadFile(p)
	ktest.RequireNoError(t, e)
	ktest.RequireCondition(t, len(data) > 0 && contains(string(data), "unknown:") && contains(string(data), "views:"), "unrelated yaml lost: %s", data)
}

func TestBadFileAndCanceled(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	os.WriteFile(p, []byte("[broken"), 0o600)
	_, e := Execute(context.Background(), map[string]any{"operation": "save", "path": p, "name": "a", "value": 1, "app": "a", "schema": "1"})
	ktest.RequireCondition(t, e != nil, "malformed accepted")
	os.Remove(p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = Execute(ctx, map[string]any{"operation": "save", "path": p, "name": "a", "value": 1, "app": "a", "schema": "1"})
	ktest.RequireCondition(t, errors.Is(e, context.Canceled), "cancel not propagated")
}

func contains(a, b string) bool {
	for i := 0; i+len(b) <= len(a); i++ {
		if a[i:i+len(b)] == b {
			return true
		}
	}
	return false
}
