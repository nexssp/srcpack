package config_views

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/crud"
)

func TestListSortedAndDelete(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	invoke := func(in map[string]any) (map[string]any, error) {
		in["path"] = p
		return Execute(context.Background(), in)
	}
	for _, name := range []string{"zeta", "alpha", "middle"} {
		_, err := invoke(map[string]any{"operation": "save", "name": name, "value": map[string]any{"n": 1}, "app": "demo", "schema": "v1"})
		ktest.RequireNoError(t, err)
	}
	got, err := invoke(map[string]any{"operation": "list"})
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, reflect.DeepEqual(got["names"], []any{"alpha", "middle", "zeta"}), "list names not sorted: %#v", got["names"])
	views, ok := got["views"].(map[string]any)
	if !ok {
		t.Fatalf("views have type %T, want map[string]any", got["views"])
	}
	ktest.RequireCondition(t, len(views) == 3, "expected 3 views: %#v", views)
	_, err = invoke(map[string]any{"operation": "delete", "name": "middle"})
	ktest.RequireNoError(t, err)
	_, err = invoke(map[string]any{"operation": "delete", "name": "middle"})
	ktest.RequireCondition(t, errors.Is(err, ErrNotFound), "expected not found: %v", err)
	got, err = invoke(map[string]any{"operation": "list"})
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, reflect.DeepEqual(got["names"], []any{"alpha", "zeta"}), "unexpected names after deletion: %#v", got["names"])
}

func TestFailedWritesPreserveBytes(t *testing.T) {
	dir := t.TempDir()
	// Rename over a directory must fail, even when the test runner is privileged.
	destination := filepath.Join(dir, "destination")
	ktest.RequireNoError(t, os.Mkdir(destination, 0o700))
	marker := filepath.Join(destination, "keep")
	original := []byte("retain exact bytes\n")
	ktest.RequireNoError(t, os.WriteFile(marker, original, 0o600))
	err := (crud.YAMLStore{}).Patch(context.Background(), destination, "config_views",
		crud.Snapshot{Token: "other: keep\n"}, crud.Patch{Key: "new", Value: map[string]any{"app": "demo", "schema": "v1", "value": 1}})
	ktest.RequireCondition(t, err != nil, "expected atomic write failure")
	got, readErr := os.ReadFile(marker)
	ktest.RequireNoError(t, readErr)
	ktest.RequireEqual(t, got, original)
	entries, err := os.ReadDir(dir)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, len(entries), 1)
}

func TestMalformedMultidocumentAndDuplicateKeysDoNotOverwrite(t *testing.T) {
	for _, original := range []string{"[broken", "a: 1\n---\nb: 2\n", "config_views:\n  x: 1\n  x: 2\n"} {
		t.Run(strings.ReplaceAll(strings.TrimSpace(original), "\n", "_"), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yml")
			ktest.RequireNoError(t, os.WriteFile(p, []byte(original), 0o600))
			_, err := Execute(context.Background(), map[string]any{"operation": "save", "path": p, "name": "new", "value": 3, "app": "a", "schema": "1"})
			ktest.RequireCondition(t, err != nil, "invalid YAML accepted")
			got, readErr := os.ReadFile(p)
			ktest.RequireNoError(t, readErr)
			ktest.RequireCondition(t, string(got) == original, "rejected input was overwritten: %q", got)
		})
	}
}

func TestCanceledReadAndWriteAndUnknownOperationDoNotWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	original := []byte("keep: me\n")
	ktest.RequireNoError(t, os.WriteFile(p, original, 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, op := range []string{"get", "save"} {
		input := map[string]any{"operation": op, "path": p, "name": "x", "value": 1, "app": "a", "schema": "1"}
		_, err := Execute(ctx, input)
		ktest.RequireCondition(t, errors.Is(err, context.Canceled), "operation %s did not return cancellation: %v", op, err)
	}
	_, err := Execute(context.Background(), map[string]any{"operation": "nonsense", "path": p, "name": "x", "value": 1, "app": "a", "schema": "1"})
	ktest.RequireCondition(t, err != nil, "unknown operation accepted")
	got, readErr := os.ReadFile(p)
	ktest.RequireNoError(t, readErr)
	ktest.RequireCondition(t, string(got) == string(original), "cancellation or invalid operation wrote data: %q", got)
}
