package config_views

import (
	"context"
	"errors"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/crud"
)

// adapterStore deliberately performs no filesystem access. It records the exact
// envelope and section selected by NFlow, not by the storage implementation.
type adapterStore struct {
	sections map[string]map[string]any
	writes   int
	reads    int
	fail     error
}

func (s *adapterStore) Read(_ context.Context, _, section string) (crud.Snapshot, error) {
	s.reads++
	if s.fail != nil {
		return crud.Snapshot{}, s.fail
	}
	entries := s.sections[section]
	if entries == nil {
		entries = map[string]any{}
		s.sections[section] = entries
	}
	return crud.Snapshot{Entries: entries, Token: "snapshot-version"}, nil
}

func (s *adapterStore) Patch(_ context.Context, _, section string, snapshot crud.Snapshot, patch crud.Patch) error {
	if s.fail != nil {
		return s.fail
	}
	if snapshot.Token != "snapshot-version" {
		return errors.New("snapshot token changed")
	}
	s.writes++
	if patch.Remove {
		delete(s.sections[section], patch.Key)
	} else {
		s.sections[section][patch.Key] = patch.Value
	}
	return nil
}

func TestAdapterUsesReplaceableCRUDStore(t *testing.T) {
	store := &adapterStore{sections: map[string]map[string]any{}}
	ctx := crud.WithStore(context.Background(), store)
	call := func(input map[string]any) (map[string]any, error) {
		input["path"] = "not-a-file://presets"
		return Execute(ctx, input)
	}
	value := map[string]any{"mode": "review"}
	_, err := call(map[string]any{"operation": "save", "name": "one", "app": "demo", "schema": "v1", "value": value})
	ktest.RequireNoError(t, err)
	section, ok := store.sections["config_views"]
	if !ok {
		t.Fatal("config_views section was not stored")
	}
	record, ok := section["one"].(map[string]any)
	if !ok {
		t.Fatalf("stored record has type %T, want map[string]any", section["one"])
	}
	ktest.RequireEqual(t, record["app"], any("demo"))
	ktest.RequireEqual(t, record["schema"], any("v1"))
	ktest.RequireEqual(t, record["value"], any(value))
	_, err = call(map[string]any{"operation": "save", "name": "one", "app": "demo", "schema": "v1"})
	ktest.RequireCondition(t, errors.Is(err, crud.ErrConflict), "adapter must delegate conflicts: %v", err)
	_, err = call(map[string]any{"operation": "delete", "name": "one", "app": "other"})
	ktest.RequireCondition(t, errors.Is(err, ErrCompatibility), "identity mismatch accepted: %v", err)
	ktest.RequireEqual(t, store.writes, 1)
	// Legacy permits historical empty names and null values; CRUD itself does not
	// relax standalone name validation to accommodate srcpack.
	_, err = call(map[string]any{"operation": "save", "name": "", "legacy": true, "value": nil})
	ktest.RequireNoError(t, err)
	_, exists := store.sections["views"][""]
	ktest.RequireCondition(t, exists, "legacy value saved to wrong section")
	got, err := call(map[string]any{"operation": "get", "name": "", "legacy": true})
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, got["value"] == nil, "legacy null changed: %#v", got)
	_, err = call(map[string]any{"operation": "save", "name": "bad", "legacy": true, "value": []any{3}})
	ktest.RequireCondition(t, err != nil, "legacy invalid value accepted")
	ktest.RequireEqual(t, store.writes, 2)
	got, err = call(map[string]any{"operation": "list"})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, got["views"], any(store.sections["config_views"]))
	ktest.RequireCondition(t, got["records"] == nil, "adapter leaked generic output layout")
}

func TestAdapterPropagatesStoreFailure(t *testing.T) {
	failure := errors.New("custom store unavailable")
	store := &adapterStore{fail: failure}
	_, err := Execute(crud.WithStore(context.Background(), store), map[string]any{"operation": "list", "path": "memory"})
	ktest.RequireCondition(t, errors.Is(err, failure), "store error lost: %v", err)
	ktest.RequireEqual(t, store.writes, 0)
}
