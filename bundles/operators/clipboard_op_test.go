package operators

import (
	"encoding/json"
	"errors"
	"iter"
	"reflect"
	"testing"

	"github.com/nexssp/flow/extensions/fs"
)

func TestClipboardDisabledPassesThroughWithoutWriting(t *testing.T) {
	op := clipboardWithWriter(ClipboardConfig{}, func(string) error {
		t.Fatal("disabled clipboard operator attempted a write")
		return nil
	})

	input := fileContentSequence(fs.FileContent{Content: []byte("context")})
	var got []fs.FileContent
	for item, err := range op(input) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if len(got) != 1 || string(got[0].Content) != "context" {
		t.Fatalf("forwarded items = %#v, want the original content", got)
	}
}

func TestClipboardWritesCombinedContentAndPassesItemsThrough(t *testing.T) {
	var copied string
	writes := 0
	writeClipboard := func(value string) error {
		writes++
		copied = value
		return nil
	}

	first := fs.FileContent{Content: []byte("one\n")}
	second := fs.FileContent{Content: []byte("two\n")}
	var got []fs.FileContent
	for item, err := range clipboardWithWriter(ClipboardConfig{Enabled: true, Quiet: true}, writeClipboard)(fileContentSequence(first, second)) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if copied != "one\ntwo\n" {
		t.Fatalf("clipboard content = %q, want concatenated rendered output", copied)
	}
	if writes != 1 {
		t.Fatalf("clipboard writes = %d, want exactly one", writes)
	}
	if !reflect.DeepEqual(got, []fs.FileContent{first, second}) {
		t.Fatalf("forwarded items = %#v, want original items", got)
	}
}

func TestClipboardSanitizesNULAndBinaryBytesOnlyForClipboard(t *testing.T) {
	var copied string
	writeClipboard := func(value string) error {
		copied = value
		return nil
	}

	original := fs.FileContent{Content: []byte("before\x00\xffmiddle\x00after")}
	var got []fs.FileContent
	for item, err := range clipboardWithWriter(ClipboardConfig{Enabled: true, Quiet: true}, writeClipboard)(fileContentSequence(original)) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if want := "before\uFFFDmiddleafter"; copied != want {
		t.Fatalf("sanitized clipboard text = %q, want %q", copied, want)
	}
	if stringsContainsNUL(copied) {
		t.Fatalf("clipboard text still contains NUL: %q", copied)
	}
	if !reflect.DeepEqual(got, []fs.FileContent{original}) {
		t.Fatalf("downstream content changed; got %#v, want original bytes %#v", got, original)
	}
	if got[0].Content[6] != 0 || got[0].Content[7] != 0xff {
		t.Fatalf("original NUL/binary bytes were not preserved downstream: %v", got[0].Content)
	}
}

func TestSanitizeClipboardTextPreservesNormalSourceExactly(t *testing.T) {
	input := "package main\n// UTF-8: café\n"
	got, sanitized := sanitizeClipboardText(input)
	if got != input {
		t.Fatalf("normal source changed: got %q, want %q", got, input)
	}
	if sanitized.nulBytesRemoved != 0 || sanitized.invalidUTF8Replaced {
		t.Fatalf("normal source was marked sanitized: %+v", sanitized)
	}
}

func TestClipboardReportsWriteErrors(t *testing.T) {
	wantErr := errors.New("clipboard unavailable")
	writeClipboard := func(string) error { return wantErr }

	for _, err := range clipboardWithWriter(ClipboardConfig{Enabled: true, Quiet: true}, writeClipboard)(fileContentSequence(fs.FileContent{Content: []byte("text")})) {
		if !errors.Is(err, wantErr) {
			t.Fatalf("clipboard error = %v, want wrapped %v", err, wantErr)
		}
		return
	}
	t.Fatal("expected clipboard write error")
}

func TestClipboardConfigAcceptsFlowStringAndJSONBooleans(t *testing.T) {
	for _, test := range []struct {
		value string
		want  ClipboardConfig
	}{
		{`{"enabled":"true"}`, ClipboardConfig{Enabled: true}},
		{`{"enabled":"false"}`, ClipboardConfig{}},
		{`{"enabled":true,"quiet":true}`, ClipboardConfig{Enabled: true, Quiet: true}},
		{`{"enabled":"true","quiet":"true"}`, ClipboardConfig{Enabled: true, Quiet: true}},
	} {
		var cfg ClipboardConfig
		if err := json.Unmarshal([]byte(test.value), &cfg); err != nil {
			t.Errorf("unmarshal %s: %v", test.value, err)
			continue
		}
		if cfg != test.want {
			t.Errorf("unmarshal %s: config = %+v, want %+v", test.value, cfg, test.want)
		}
	}
	for _, invalid := range []string{`{"enabled":"maybe"}`, `{"quiet":"maybe"}`} {
		var cfg ClipboardConfig
		if err := json.Unmarshal([]byte(invalid), &cfg); err == nil {
			t.Errorf("expected invalid clipboard boolean %s to be rejected", invalid)
		}
	}
}

func stringsContainsNUL(value string) bool {
	for _, r := range value {
		if r == 0 {
			return true
		}
	}
	return false
}

func fileContentSequence(items ...fs.FileContent) iter.Seq2[fs.FileContent, error] {
	return func(yield func(fs.FileContent, error) bool) {
		for _, item := range items {
			if !yield(item, nil) {
				return
			}
		}
	}
}
