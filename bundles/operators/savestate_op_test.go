package operators

import (
	"encoding/json"
	"errors"
	"iter"
	"os"
	"reflect"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestSaveStateConfigAcceptsBoolAndString(t *testing.T) {
	for _, tc := range []struct {
		input   string
		enabled bool
	}{
		{`{}`, true}, {`{"enabled":null}`, true}, {`{"enabled":true}`, true}, {`{"enabled":"true"}`, true}, {`{"enabled":false}`, false}, {`{"enabled":"false"}`, false},
	} {
		var cfg SaveStateConfig
		ktest.RequireNoError(t, json.Unmarshal([]byte(tc.input), &cfg))
		ktest.RequireEqual(t, cfg.isEnabled(), tc.enabled)
	}
}

func TestSaveStateDefaultWritesAndForwards(t *testing.T) {
	withStatePath(t)
	first := ParsedFile{RelPath: "a.go", Hash: "hash-a"}
	second := ParsedFile{RelPath: "b.go", Hash: "hash-b"}
	got := collectSaveState(t, SaveStateConfig{}, saveStateSequence(first, second))
	ktest.RequireEqual(t, got, []ParsedFile{first, second})
	cache := LoadState(StateFileName)
	ktest.RequireCondition(t, cache != nil, "default config did not create state")
	ktest.RequireEqual(t, cache.Files["a.go"].Hash, "hash-a")
	ktest.RequireEqual(t, cache.Files["b.go"].Hash, "hash-b")
}

func TestSaveStateDisabledDoesNotWriteAndPreservesExistingState(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  SaveStateConfig
	}{
		{"bool", SaveStateConfig{Enabled: new(false)}},
		{"string", mustSaveStateConfig(t, `{"enabled":"false"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withStatePath(t)
			ktest.RequireNoError(t, SaveState(StateFileName, []ParsedFile{{RelPath: "old.go", Hash: "old"}}))
			before, err := os.ReadFile(StateFileName)
			ktest.RequireNoError(t, err)
			want := ParsedFile{RelPath: "new.go", Hash: "new"}
			got := collectSaveState(t, tc.cfg, saveStateSequence(want))
			ktest.RequireEqual(t, got, []ParsedFile{want})
			after, err := os.ReadFile(StateFileName)
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, after, before)
		})
	}
}

func TestSaveStateUpstreamErrorDoesNotWrite(t *testing.T) {
	withStatePath(t)
	wantErr := errors.New("upstream failed")
	var got []error
	for _, err := range saveState(SaveStateConfig{})(saveStateSequenceWithError(ParsedFile{RelPath: "a.go"}, wantErr)) {
		got = append(got, err)
	}
	ktest.RequireCondition(t, len(got) == 1 && errors.Is(got[0], wantErr), "upstream error was not forwarded")
	if _, err := os.Stat(StateFileName); !os.IsNotExist(err) {
		t.Fatalf("state file stat error = %v, want absent", err)
	}
}

func TestSaveStateDisabledDoesNotCreateStateAndForwards(t *testing.T) {
	withStatePath(t)
	want := ParsedFile{RelPath: "a.go", Content: []byte("original")}
	got := collectSaveState(t, SaveStateConfig{Enabled: new(false)}, saveStateSequence(want))
	ktest.RequireCondition(t, reflect.DeepEqual(got, []ParsedFile{want}), "disabled operator changed forwarded item")
	if _, err := os.Stat(StateFileName); !os.IsNotExist(err) {
		t.Fatalf("state file stat error = %v, want absent", err)
	}
}

func withStatePath(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

func collectSaveState(t *testing.T, cfg SaveStateConfig, input iter.Seq2[ParsedFile, error]) []ParsedFile {
	t.Helper()
	var got []ParsedFile
	for file, err := range saveState(cfg)(input) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, file)
	}
	return got
}

func saveStateSequence(files ...ParsedFile) iter.Seq2[ParsedFile, error] {
	return func(yield func(ParsedFile, error) bool) {
		for _, f := range files {
			if !yield(f, nil) {
				return
			}
		}
	}
}

func saveStateSequenceWithError(file ParsedFile, want error) iter.Seq2[ParsedFile, error] {
	return func(yield func(ParsedFile, error) bool) { yield(file, nil); yield(ParsedFile{}, want) }
}

func mustSaveStateConfig(t *testing.T, raw string) SaveStateConfig {
	t.Helper()
	var cfg SaveStateConfig
	ktest.RequireNoError(t, json.Unmarshal([]byte(raw), &cfg))
	return cfg
}
