package operators

import (
	"iter"
	"reflect"
	"testing"
)

func TestStatsCollectPreservesParsedFileRecords(t *testing.T) {
	files := []ParsedFile{
		{Path: "/workspace/first.go", RelPath: "first.go", Size: 10, Lang: "go", Content: []byte("café"), Snippets: []string{"ignored because content is present"}, FuncCount: 2},
		{Path: "/workspace/second.go", RelPath: "second.go", Size: 11, Lang: "go", Snippets: []string{"é", "xy"}, FuncCount: 3},
		{Path: "/workspace/omitted.go", RelPath: "omitted.go", Size: 90, Lang: "go", Content: []byte("not counted"), FuncCount: 99, Omitted: true},
		{Path: "/workspace/empty.go", RelPath: "empty.go", Size: 20, Lang: "go", FuncCount: 7},
	}

	collected, err := collectAny(StatsCollect(StatsCollectConfig{Quiet: true})(anyParsedFiles(files)))
	if err != nil {
		t.Fatalf("stats.collect: %v", err)
	}
	gotFiles, err := parsedFilesFromRecords(collected)
	if err != nil {
		t.Fatalf("decode stats.collect records: %v", err)
	}
	if !reflect.DeepEqual(gotFiles, files) {
		t.Fatalf("stats.collect changed or reordered records:\n got: %#v\nwant: %#v", gotFiles, files)
	}
}

func TestFlowProjectedParsedFileRestoresBinaryContent(t *testing.T) {
	raw := []byte{0, 1, 0x7f, 0x80, 0xff}
	projected := map[string]any{
		"content": []any{uint8(0), uint8(1), uint8(0x7f), uint8(0x80), uint8(0xff)},
		"path":    "/workspace/binary.bin", "rel_path": "binary.bin", "size": int64(len(raw)), "lang": "text",
		"func_count": 2, "lines": 3, "source_start_line": 3, "source_end_line": 3,
		"sliced": true, "source_url": "https://example.test/binary.bin",
		"symbol_spans": []any{map[string]any{"name": "run", "kind": "function", "start_line": 3, "end_line": 3}},
		"line_spans":   []any{map[string]any{"output_line": 1, "source_start_line": 3, "source_end_line": 3}},
	}

	got, err := parsedFilesFromRecords([]any{projected})
	if err != nil {
		t.Fatal(err)
	}
	want := []ParsedFile{{
		Path: "/workspace/binary.bin", RelPath: "binary.bin", Size: int64(len(raw)), Lang: "text",
		Content: raw, FuncCount: 2, Lines: 3, SourceStartLine: 3, SourceEndLine: 3,
		Sliced: true, SourceURL: "https://example.test/binary.bin",
		SymbolSpans: []SymbolSpan{{Name: "run", Kind: "function", StartLine: 3, EndLine: 3}},
		LineSpans:   []SourceLineSpan{{OutputLine: 1, SourceStartLine: 3, SourceEndLine: 3}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projected record changed: got %#v, want %#v", got, want)
	}
}

func TestFlowProjectedStatsCollectEnvelopeRestoresNestedStats(t *testing.T) {
	projected := map[string]any{
		"files": []any{map[string]any{
			"content": []any{uint8('x')}, "path": "/workspace/main.go", "rel_path": "main.go",
			"size": int64(7), "lang": "go", "func_count": 2, "lines": 1,
		}},
		"stats": map[string]any{
			"FileCount": 1, "FuncCount": 2, "PackedBytes": int64(1), "RawBytes": int64(7), "EstimatedTokens": int64(0),
		},
	}

	envelope, err := unpackPackStatsEnvelope(projected)
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.Files) != 1 || string(envelope.Files[0].Content) != "x" {
		t.Fatalf("projected files were not restored: %#v", envelope.Files)
	}
	want := PackStats{FileCount: 1, FuncCount: 2, PackedBytes: 1, RawBytes: 7, EstimatedTokens: 0}
	if !reflect.DeepEqual(envelope.Stats, want) {
		t.Fatalf("nested projected stats = %#v, want %#v", envelope.Stats, want)
	}
}

func anyParsedFiles(files []ParsedFile) iter.Seq2[any, error] {
	return func(yield func(any, error) bool) {
		for _, file := range files {
			if !yield(file, nil) {
				return
			}
		}
	}
}

func collectAny(stream iter.Seq2[any, error]) ([]any, error) {
	var values []any
	var streamErr error
	stream(func(value any, err error) bool {
		if err != nil {
			streamErr = err
			return false
		}
		values = append(values, value)
		return true
	})
	return values, streamErr
}
