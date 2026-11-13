package operators

import (
	"context"
	"testing"

	"github.com/nexssp/flow/extensions/fs"
)

func TestRenderFileBlockEmitsCompactSourceLocation(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("package example\n"),
		SourceStartLine: 8,
		SourceEndLine:   12,
		SymbolSpans: []SymbolSpan{{
			Name: "Answer", Kind: "func", StartLine: 9, EndLine: 11,
		}},
		LineSpans: []SourceLineSpan{{OutputLine: 1, SourceStartLine: 9, SourceEndLine: 11}},
		SourceURL: "https://github.com/nexssp/flow/blob/0123456789abcdef/pkg/example.go#L8-L12",
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}\n```go\n", "```\n"))
	want := "<!-- pkg/example.go:8-12 -->\n### pkg/example.go\n```go\npackage example\n```\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderFileBlockCollapsesSingleLineRange(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("package example\n"),
		SourceStartLine: 52,
		SourceEndLine:   52,
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}\n", ""))
	want := "<!-- pkg/example.go:52 -->\n### pkg/example.go\npackage example\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderFileBlockOmitsSourceLocationWhenRangeMissing(t *testing.T) {
	file := ParsedFile{
		FileMeta: sourceLocationTestMeta(),
		Content:  []byte("package example\n"),
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}\n", ""))
	want := "### pkg/example.go\npackage example\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderFileBlockIgnoresLineSpansForComment(t *testing.T) {
	// Even with dense non-linear spans, the rendered comment stays compact.
	// Line spans remain available on the ParsedFile record for structured
	// consumers; only the human/AI-readable prefix is reduced.
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("first\nsecond\n\nthird\nfourth\n"),
		SourceStartLine: 52,
		SourceEndLine:   62,
		LineSpans: []SourceLineSpan{
			{OutputLine: 1, SourceStartLine: 52, SourceEndLine: 52},
			{OutputLine: 2, SourceStartLine: 53, SourceEndLine: 53},
			{OutputLine: 4, SourceStartLine: 60, SourceEndLine: 62},
			{OutputLine: 5, SourceStartLine: 60, SourceEndLine: 62},
		},
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}\n", ""))
	want := "<!-- pkg/example.go:52-62 -->\n### pkg/example.go\nfirst\nsecond\n\nthird\nfourth\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDocumentInputRetainsSourceLocationFacts(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("func Answer() {}"),
		SourceStartLine: 20,
		SourceEndLine:   20,
		SymbolSpans: []SymbolSpan{{
			Name: "Answer", Kind: "func", StartLine: 20, EndLine: 20,
		}},
		LineSpans: []SourceLineSpan{{OutputLine: 1, SourceStartLine: 20, SourceEndLine: 20}},
	}
	result, err := documentInput(context.Background(), RenderConfig{}, []ParsedFile{file}, PackStats{})
	if err != nil {
		t.Fatal(err)
	}
	files, ok := result["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("document files have type %T, want one map record", result["files"])
	}
	if files[0]["source_start_line"] != 20 || files[0]["source_end_line"] != 20 {
		t.Fatalf("document location fields = %#v", files[0])
	}
	if got, want := files[0]["source_location"], "<!-- pkg/example.go:20 -->\n"; got != want {
		t.Fatalf("document source_location = %#v, want %q", got, want)
	}
	spans, ok := files[0]["symbol_spans"].([]SymbolSpan)
	if !ok || len(spans) != 1 || spans[0].Name != "Answer" {
		t.Fatalf("document symbol spans = %#v", files[0]["symbol_spans"])
	}
}

func sourceLocationTestMeta() fs.FileMeta {
	return fs.FileMeta{Path: "pkg/example.go", RelPath: "pkg/example.go", Lang: "go"}
}

func TestRemoteSourceURLIsCommitPinned(t *testing.T) {
	got := remoteSourceURL("github/nexssp/flow@0123456789abcdef", "pkg/example.go", 8, 12)
	want := "https://github.com/nexssp/flow/blob/0123456789abcdef/pkg/example.go#L8-L12"
	if got != want {
		t.Fatalf("remote source URL = %q, want %q", got, want)
	}
}
