package crud

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

type memoryStore struct {
	records        map[string]any
	reads, patches int
}

func (m *memoryStore) Read(_ context.Context, _, _ string) (Snapshot, error) {
	m.reads++
	entries := map[string]any{}
	maps.Copy(entries, m.records)
	return Snapshot{Entries: entries, Token: "memory-token"}, nil
}

func (m *memoryStore) Patch(_ context.Context, _, _ string, s Snapshot, p Patch) error {
	if s.Token != "memory-token" {
		return errors.New("bad token")
	}
	m.patches++
	if p.Remove {
		delete(m.records, p.Key)
	} else {
		m.records[p.Key] = p.Value
	}
	return nil
}

func TestCRUDPolicyWithMemoryStore(t *testing.T) {
	m := &memoryStore{records: map[string]any{"z": 9, "a": 1}}
	ctx := WithStore(context.Background(), m)
	call := func(in map[string]any) (map[string]any, error) {
		in["path"] = "never-access-this-path"
		return Execute(ctx, in)
	}
	list, err := call(map[string]any{"operation": "list"})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, list["names"], any([]any{"a", "z"}))
	ktest.RequireEqual(t, list["records"], any(m.records))
	got, err := call(map[string]any{"operation": "get", "name": "a"})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, got["value"], any(1))
	for _, op := range []string{"get", "delete"} {
		_, err = call(map[string]any{"operation": op, "name": "missing"})
		ktest.RequireCondition(t, errors.Is(err, ErrNotFound), "missing %s: %v", op, err)
	}
	_, err = call(map[string]any{"operation": "save", "name": "a", "value": 2})
	ktest.RequireCondition(t, errors.Is(err, ErrConflict), "conflict: %v", err)
	ktest.RequireEqual(t, m.records["a"], any(1))
	_, err = call(map[string]any{"operation": "save", "name": "a", "value": 2, "replace": true})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, m.records["a"], any(2))
	_, err = call(map[string]any{"operation": "delete", "name": "z"})
	ktest.RequireNoError(t, err)
	_, exists := m.records["z"]
	ktest.RequireCondition(t, !exists, "delete did not remove")
	ktest.RequireEqual(t, m.reads, 7)
	ktest.RequireEqual(t, m.patches, 2)
	// Null, scalars and nested records need no configuration identity.
	for _, value := range []any{nil, false, "text", []any{1, "x"}, map[string]any{"nested": true}} {
		_, err = call(map[string]any{"operation": "save", "name": "a", "value": value, "replace": true})
		ktest.RequireNoError(t, err)
		got, err = call(map[string]any{"operation": "get", "name": "a"})
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, got["value"], value)
	}
	reads, writes := m.reads, m.patches
	for _, in := range []map[string]any{
		{"operation": "execute"},
		{"operation": "save", "name": ""},
		{"operation": "save", "name": "a", "replace": "yes"},
		{"operation": "list", "section": 3},
	} {
		_, err = call(in)
		ktest.RequireCondition(t, err != nil, "invalid request accepted: %#v", in)
	}
	ktest.RequireEqual(t, m.reads, reads)
	ktest.RequireEqual(t, m.patches, writes)
}

func TestYAMLStorePreservesUnknownAndFailsClosed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "data.yml")
	before := []byte("outside: keep\nrecords:\n  old: 1\nother: [x, y]\n")
	ktest.RequireNoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	ktest.RequireNoError(t, os.WriteFile(p, before, 0o600))
	s := YAMLStore{}
	snap, err := s.Read(context.Background(), p, "records")
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, snap.Token, any(string(before)))
	ktest.RequireEqual(t, snap.Entries["old"], any(1))
	ktest.RequireNoError(t, s.Patch(context.Background(), p, "records", snap, Patch{Key: "new", Value: map[string]any{"ok": true}}))
	after, err := os.ReadFile(p)
	ktest.RequireNoError(t, err)
	for _, part := range []string{"outside: keep", "other: [x, y]", "old: 1", "new:", "ok: true"} {
		ktest.RequireStringContains(t, string(after), part)
	}
	for _, original := range []string{"records: [invalid]\noutside: untouched\n", "[broken", "a: 1\n---\nb: 2\n", "records: {x: 1, x: 2}\n"} {
		ktest.RequireNoError(t, os.WriteFile(p, []byte(original), 0o600))
		_, err = Execute(context.Background(), map[string]any{"operation": "save", "path": p, "name": "x", "value": 1})
		ktest.RequireCondition(t, err != nil, "invalid YAML accepted: %s", original)
		now, readErr := os.ReadFile(p)
		ktest.RequireNoError(t, readErr)
		ktest.RequireEqual(t, string(now), original)
	}
}

func TestYAMLStoreRejectsUnserializableValueWithoutWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data.yml")
	original := []byte("known: true\n")
	ktest.RequireNoError(t, os.WriteFile(p, original, 0o600))
	s := YAMLStore{}
	snap, err := s.Read(context.Background(), p, "records")
	ktest.RequireNoError(t, err)
	err = s.Patch(context.Background(), p, "records", snap, Patch{Key: "bad", Value: func() {}})
	ktest.RequireCondition(t, err != nil, "function serialized")
	now, err := os.ReadFile(p)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, now, original)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Execute(ctx, map[string]any{"operation": "save", "path": p, "name": "x", "value": 1})
	ktest.RequireCondition(t, errors.Is(err, context.Canceled), "cancel lost: %v", err)
}

func TestAtomicFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "destination")
	ktest.RequireNoError(t, os.Mkdir(destination, 0o700))
	marker := filepath.Join(destination, "keep")
	ktest.RequireNoError(t, os.WriteFile(marker, []byte("unchanged"), 0o600))
	err := (YAMLStore{}).Patch(context.Background(), destination, "records", Snapshot{Token: "other: keep\n"}, Patch{Key: "x", Value: 1})
	ktest.RequireCondition(t, err != nil, "rename over directory accepted")
	data, err := os.ReadFile(marker)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, string(data), "unchanged")
	entries, err := os.ReadDir(dir)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, len(entries), 1)
}

type failingStore struct{ readError, patchError error }

func (s failingStore) Read(context.Context, string, string) (Snapshot, error) {
	return Snapshot{Entries: map[string]any{}, Token: "opaque"}, s.readError
}

func (s failingStore) Patch(context.Context, string, string, Snapshot, Patch) error {
	return s.patchError
}

func TestCustomStoreErrorsPropagate(t *testing.T) {
	failure := errors.New("custom persistence failure")
	for _, s := range []failingStore{{readError: failure}, {patchError: failure}} {
		_, err := Execute(WithStore(context.Background(), s), map[string]any{"operation": "save", "path": "memory", "name": "x", "value": 1})
		ktest.RequireCondition(t, errors.Is(err, failure), "custom store error lost: %v", err)
	}
}
