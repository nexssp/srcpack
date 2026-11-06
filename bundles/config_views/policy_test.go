package config_views

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestPolicyValidationAndReadOnlyFailures(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.yml")
	original := []byte("unrelated: keep\n")
	ktest.RequireNoError(t, os.WriteFile(path, original, 0o600))
	for _, tc := range []struct {
		name  string
		input map[string]any
	}{
		{"unknown operation", map[string]any{"operation": "execute"}},
		{"missing app", map[string]any{"operation": "save", "name": "x", "schema": "v1"}},
		{"missing schema", map[string]any{"operation": "save", "name": "x", "app": "a"}},
		{"blank name", map[string]any{"operation": "save", "name": " ", "app": "a", "schema": "v1"}},
		{"invalid replace", map[string]any{"operation": "save", "name": "x", "app": "a", "schema": "v1", "replace": "true"}},
		{"invalid operation type", map[string]any{"operation": 12}},
		{"invalid name type", map[string]any{"operation": "save", "name": 12, "app": "a", "schema": "v1"}},
		{"invalid legacy values", map[string]any{"operation": "save", "name": "x", "legacy": true, "value": []any{"ok", 12}}},
		{"unserializable", map[string]any{"operation": "save", "name": "x", "app": "a", "schema": "v1", "value": func() {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "presets.yml")
			ktest.RequireNoError(t, os.WriteFile(path, original, 0o600))
			tc.input["path"] = path
			_, err := Execute(context.Background(), tc.input)
			ktest.RequireCondition(t, err != nil, "invalid request accepted")
			after, err := os.ReadFile(path)
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, after, original)
		})
	}
}

func TestMissingFileAndEmptyLegacyValues(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "presets.yml")
	for _, op := range []string{"list", "get", "delete"} {
		_, err := Execute(context.Background(), map[string]any{"operation": op, "path": path, "name": "missing"})
		if op == "list" {
			ktest.RequireNoError(t, err)
		} else {
			ktest.RequireCondition(t, errors.Is(err, ErrNotFound), "missing %s returned %v", op, err)
		}
		_, err = os.Stat(path)
		ktest.RequireCondition(t, os.IsNotExist(err), "read operation created the file")
	}
	ktest.RequireNoError(t, os.WriteFile(path, []byte("views:\n  old: null\n"), 0o600))
	got, err := Execute(context.Background(), map[string]any{"operation": "get", "path": path, "name": "old", "legacy": true})
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, got["value"] == nil, "legacy null changed: %#v", got)
	_, err = Execute(context.Background(), map[string]any{"operation": "save", "path": path, "name": "", "legacy": true, "value": []string{"a"}})
	ktest.RequireNoError(t, err)
}

func TestStructuredReplacementAndIdentity(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "presets.yml")
	value := map[string]any{"flow": "runtime.fail @{ message: \"not executable\" }", "limits": map[string]any{"depth": 2, "enabled": false}, "items": []any{"x", nil, 7}}
	request := map[string]any{"operation": "save", "path": path, "name": "review", "app": "demo", "schema": "settings/v1", "value": value}
	_, err := Execute(context.Background(), request)
	ktest.RequireNoError(t, err)
	before, err := os.ReadFile(path)
	ktest.RequireNoError(t, err)
	got, err := Execute(context.Background(), map[string]any{"operation": "get", "path": path, "name": "review", "app": "demo", "schema": "settings/v1"})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, got["value"], any(value))
	for _, op := range []string{"get", "delete"} {
		_, err = Execute(context.Background(), map[string]any{"operation": op, "path": path, "name": "review", "schema": "wrong"})
		ktest.RequireCondition(t, errors.Is(err, ErrCompatibility), "identity not enforced for %s: %v", op, err)
	}
	request["value"] = map[string]any{"replacement": true}
	_, err = Execute(context.Background(), request)
	ktest.RequireCondition(t, errors.Is(err, ErrConflict), "implicit overwrite accepted: %v", err)
	after, err := os.ReadFile(path)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, after, before)
	request["replace"] = true
	_, err = Execute(context.Background(), request)
	ktest.RequireNoError(t, err)
	got, err = Execute(context.Background(), map[string]any{"operation": "get", "path": path, "name": "review"})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, got["value"], any(map[string]any{"replacement": true}))
}

func TestMalformedStoredEntriesFailClosed(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"config_views: {bad: {value: 1}}\n",
		"config_views: {bad: {app: a, schema: 1, value: 1}}\n",
		"config_views: {bad: {app: a, schema: v1}}\n",
		"config_views: {1: {app: a, schema: v1, value: 1}}\n",
		"config_views: [bad]\n",
	} {
		path := filepath.Join(t.TempDir(), "presets.yml")
		ktest.RequireNoError(t, os.WriteFile(path, []byte(source), 0o600))
		_, err := Execute(context.Background(), map[string]any{"operation": "save", "path": path, "name": "new", "app": "a", "schema": "v1", "value": nil})
		ktest.RequireCondition(t, err != nil, "invalid entries accepted: %s", source)
		after, err := os.ReadFile(path)
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, string(after), source)
	}
}
