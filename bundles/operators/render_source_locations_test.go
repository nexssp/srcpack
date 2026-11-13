package operators

import (
	"context"
	"strings"
	"testing"

	"github.com/nexssp/flow/extensions/fs"
)

func TestRenderFileBlockIncludesSourceLocationMetadata(t *testing.T) {
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
	for _, want := range []string{
		"<!-- srcpack-source:", `"path":"pkg/example.go"`,
		`"start_line":8`, `"end_line":12`, `"name":"Answer"`,
		`"start_line":9`, `"end_line":11`, "package example",
		`"line_spans":[{"output_line":1,"source_start_line":9,"source_end_line":11}]`,
		`"url":"https://github.com/nexssp/flow/blob/0123456789abcdef/pkg/example.go#L8-L12"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered block missing %q:\n%s", want, got)
		}
	}
}

func TestRenderFileBlockOmitsRedundantIdentityLineSpans(t *testing.T) {
	lineSpans := make([]SourceLineSpan, 12)
	for i := range lineSpans {
		lineSpans[i] = SourceLineSpan{OutputLine: i + 1, SourceStartLine: 52 + i, SourceEndLine: 52 + i}
	}
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte(strings.Repeat("source line\n", len(lineSpans))),
		SourceStartLine: 52,
		SourceEndLine:   63,
		LineSpans:       lineSpans,
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}\n", ""))
	if strings.Contains(got, `"line_spans"`) {
		t.Fatalf("redundant identity line map was rendered:\n%s", got)
	}
	if !strings.Contains(got, `"start_line":52`) {
		t.Fatalf("source range was lost when omitting the redundant map:\n%s", got)
	}
}

func TestRenderFileBlockKeepsIncompleteIdentityLikeLineSpans(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("first\nsecond\n"),
		SourceStartLine: 52,
		SourceEndLine:   53,
		LineSpans:       []SourceLineSpan{{OutputLine: 1, SourceStartLine: 52, SourceEndLine: 52}},
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}\n", ""))
	if !strings.Contains(got, `"line_spans":[{"output_line":1,"source_start_line":52,"source_end_line":52}]`) {
		t.Fatalf("incomplete line map was incorrectly omitted:\n%s", got)
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
	location, ok := files[0]["source_location"].(string)
	if !ok || !strings.Contains(location, `"source_start_line":20`) || !strings.Contains(location, `"line_spans"`) {
		t.Fatalf("document source_location = %#v", files[0]["source_location"])
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
