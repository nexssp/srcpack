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
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}\n```go\n", "```\n"))
	for _, want := range []string{
		"<!-- srcpack-source:", `"path":"pkg/example.go"`,
		`"start_line":8`, `"end_line":12`, `"name":"Answer"`,
		`"start_line":9`, `"end_line":11`, "package example",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered block missing %q:\n%s", want, got)
		}
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
	spans, ok := files[0]["symbol_spans"].([]SymbolSpan)
	if !ok || len(spans) != 1 || spans[0].Name != "Answer" {
		t.Fatalf("document symbol spans = %#v", files[0]["symbol_spans"])
	}
}

func sourceLocationTestMeta() fs.FileMeta {
	return fs.FileMeta{Path: "pkg/example.go", RelPath: "pkg/example.go", Lang: "go"}
}
