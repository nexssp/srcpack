package operators

import (
	"context"
	"strings"
	"testing"

	"github.com/nexssp/flow/extensions/fs"
)

func TestRenderFileBlockInlinesSlicedRange(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("package example\n"),
		SourceStartLine: 8,
		SourceEndLine:   12,
		Sliced:          true,
		SymbolSpans: []SymbolSpan{{
			Name: "Answer", Kind: "func", StartLine: 9, EndLine: 11,
		}},
		LineSpans: []SourceLineSpan{{OutputLine: 1, SourceStartLine: 9, SourceEndLine: 11}},
		SourceURL: "https://github.com/nexssp/flow/blob/0123456789abcdef/pkg/example.go#L8-L12",
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}${SOURCE_RANGE} (${LANG})\n```go\n", "```\n"))
	want := "### pkg/example.go:8-12 (go)\n```go\npackage example\n```\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if strings.Contains(got, "<!--") {
		t.Fatalf("renderer emitted a duplicate source-location comment: %q", got)
	}
}

func TestRenderFileBlockOmitsRangeForWholeFiles(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("package example\n"),
		SourceStartLine: 1,
		SourceEndLine:   98,
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}${SOURCE_RANGE} (${LANG})\n", ""))
	want := "### pkg/example.go (go)\npackage example\n"
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
		Sliced:          true,
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}${SOURCE_RANGE} (${LANG})\n", ""))
	want := "### pkg/example.go:52 (go)\npackage example\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderFileBlockSummarizesNonlinearSpansInHeader(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("first\nsecond\n\nthird\nfourth\n"),
		SourceStartLine: 52,
		SourceEndLine:   62,
		Sliced:          true,
		LineSpans: []SourceLineSpan{
			{OutputLine: 1, SourceStartLine: 52, SourceEndLine: 52},
			{OutputLine: 2, SourceStartLine: 53, SourceEndLine: 53},
			{OutputLine: 4, SourceStartLine: 60, SourceEndLine: 62},
			{OutputLine: 5, SourceStartLine: 60, SourceEndLine: 62},
		},
	}

	got := string(renderFileBlock(&file, "### ${REL_PATH}${SOURCE_RANGE} (${LANG})\n", ""))
	want := "### pkg/example.go:52-62 (go)\nfirst\nsecond\n\nthird\nfourth\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderFileBlockExposesSourceURLForCustomTemplates(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("package example\n"),
		SourceStartLine: 8,
		SourceEndLine:   12,
		Sliced:          true,
		SourceURL:       "https://github.com/nexssp/flow/blob/abc/pkg/example.go#L8-L12",
	}

	got := string(renderFileBlock(&file, "<${REL_PATH}${SOURCE_RANGE}> ${SOURCE_URL}\n", ""))
	if !strings.Contains(got, "<pkg/example.go:8-12> https://github.com/nexssp/flow/blob/abc/pkg/example.go#L8-L12") {
		t.Fatalf("custom template did not see source placeholders: %q", got)
	}
}

func TestSourceRangeSuffix(t *testing.T) {
	for _, tc := range []struct {
		name string
		file ParsedFile
		want string
	}{
		{"whole file", ParsedFile{SourceStartLine: 1, SourceEndLine: 98}, ""},
		{"slice multi-line", ParsedFile{SourceStartLine: 52, SourceEndLine: 60, Sliced: true}, ":52-60"},
		{"slice single-line", ParsedFile{SourceStartLine: 52, SourceEndLine: 52, Sliced: true}, ":52"},
		{"missing range", ParsedFile{}, ""},
		{"invalid range", ParsedFile{SourceStartLine: 0, SourceEndLine: 4, Sliced: true}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceRangeSuffix(&tc.file); got != tc.want {
				t.Fatalf("sourceRangeSuffix() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDocumentInputRetainsStructuredSourceFacts(t *testing.T) {
	file := ParsedFile{
		FileMeta:        sourceLocationTestMeta(),
		Content:         []byte("func Answer() {}"),
		SourceStartLine: 20,
		SourceEndLine:   21,
		Sliced:          true,
		SourceURL:       "https://github.com/example/repo/blob/0123456789abcdef/pkg/example.go#L20-L21",
		SymbolSpans: []SymbolSpan{{
			Name: "Answer", Kind: "func", StartLine: 20, EndLine: 21,
		}},
		LineSpans: []SourceLineSpan{{OutputLine: 1, SourceStartLine: 20, SourceEndLine: 21}},
	}
	result, err := documentInput(context.Background(), RenderConfig{}, []ParsedFile{file}, PackStats{})
	if err != nil {
		t.Fatal(err)
	}
	files, ok := result["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("document files have type %T, want one map record", result["files"])
	}
	facts := files[0]
	if facts["source_start_line"] != 20 || facts["source_end_line"] != 21 {
		t.Fatalf("document location fields = %#v", facts)
	}
	if got, want := facts["source_range"], ":20-21"; got != want {
		t.Fatalf("document source_range = %#v, want %q", got, want)
	}
	if got, want := facts["source_url"], file.SourceURL; got != want {
		t.Fatalf("document source_url = %#v, want %q", got, want)
	}
	if _, present := facts["source_location"]; present {
		t.Fatalf("verbatim source_location must not be emitted: %#v", facts)
	}
	spans, ok := facts["symbol_spans"].([]SymbolSpan)
	if !ok || len(spans) != 1 || spans[0] != file.SymbolSpans[0] {
		t.Fatalf("document symbol spans = %#v", facts["symbol_spans"])
	}
	lineSpans, ok := facts["line_spans"].([]SourceLineSpan)
	if !ok || len(lineSpans) != 1 || lineSpans[0] != file.LineSpans[0] {
		t.Fatalf("document line spans = %#v", facts["line_spans"])
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
