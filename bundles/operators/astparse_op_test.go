package operators_test

import (
	"strings"
	"testing"

	"github.com/nexssp/flow/extensions/fs"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/operators"
)

const mixedSource = `package sample

// Exported comment that should be preserved under api.
func ExportedFunc() int {
	return 42
}

func unexportedFunc() int {
	return 0
}

type ExportedType struct {
	Field int
}

type unexportedType struct{}
`

func TestASTParse_ArchFiltersUnexported(t *testing.T) {
	file := operators.ParsedFile{
		FileMeta: mustMeta("sample.go", "go"),
		Content:  []byte(mixedSource),
		ParseOptions: operators.ParseOptions{
			OnlySig:        true,
			Dense:          false,
			Comments:       false,
			ExportedOnly:   true,
			TrimBlankLines: true,
		},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)

	combined := combinedContent(out)
	ktest.RequireStringContains(t, combined, "ExportedFunc")
	ktest.RequireStringContains(t, combined, "ExportedType")
	if strings.Contains(combined, "unexportedFunc") {
		t.Errorf("unexported function leaked into arch output:\n%s", combined)
	}
	if strings.Contains(combined, "unexportedType") {
		t.Errorf("unexported type leaked into arch output:\n%s", combined)
	}
}

func TestASTParse_ImplKeepsUnexported(t *testing.T) {
	file := operators.ParsedFile{
		FileMeta: mustMeta("sample.go", "go"),
		Content:  []byte(mixedSource),
		ParseOptions: operators.ParseOptions{
			OnlySig:        false,
			Dense:          false,
			Comments:       true,
			ExportedOnly:   false,
			TrimBlankLines: true,
		},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)

	combined := combinedContent(out)
	ktest.RequireStringContains(t, combined, "ExportedFunc")
	ktest.RequireStringContains(t, combined, "unexportedFunc")
	ktest.RequireStringContains(t, combined, "unexportedType")
}

func TestASTParse_TrimIndentStripsLeadingWhitespace(t *testing.T) {
	file := operators.ParsedFile{
		FileMeta: mustMeta("sample.go", "go"),
		Content:  []byte(mixedSource),
		ParseOptions: operators.ParseOptions{
			OnlySig:    false,
			Dense:      false,
			Comments:   true,
			TrimIndent: true,
		},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)

	combined := combinedContent(out)
	for line := range strings.SplitSeq(combined, "\n") {
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
			t.Errorf("line still indented under TrimIndent: %q", line)
		}
	}
}

func TestASTParse_MaxLinesHeadTail(t *testing.T) {
	body := strings.Repeat("func f() {}\n\n", 500)
	source := "package sample\n\n" + body

	file := operators.ParsedFile{
		FileMeta: mustMeta("big.go", "go"),
		Content:  []byte(source),
		ParseOptions: operators.ParseOptions{
			OnlySig:      false,
			Dense:        false,
			Comments:     false,
			MaxLines:     40,
			MaxLinesHead: 20,
			MaxLinesTail: 10,
		},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)

	combined := combinedContent(out)
	lineCount := strings.Count(combined, "\n") + 1
	if lineCount > 80 {
		t.Errorf("expected head+tail truncation near 40 lines, got %d", lineCount)
	}
	ktest.RequireStringContains(t, combined, "package sample")
	ktest.RequireStringContains(t, combined, "TRUNCATED MIDDLE")
}

func TestASTParse_IncludeConfigIsOptInAndMasked(t *testing.T) {
	file := operators.ParsedFile{
		FileMeta: mustMeta(".golangci.yml", "text"),
		Content:  []byte("api_key: \"test-secret-value-123\"\n"),
		ParseOptions: operators.ParseOptions{
			OnlySig:       true,
			IncludeConfig: true,
		},
		ParseOptionsSet: true,
	}
	out := runASTParse(t, file)
	if len(out) != 1 {
		t.Fatalf("ASTParse returned %d records, want one config record", len(out))
	}
	got := string(out[0].Content)
	ktest.RequireStringContains(t, got, `api_key: "[REDACTED_CREDENTIAL]"`)
	ktest.RequireStringNotContains(t, got, "test-secret-value-123")

	file.ParseOptions.IncludeConfig = false
	out = runASTParse(t, file)
	if len(out) != 1 || len(out[0].Content) != 0 {
		t.Fatalf("config without --include-config was not omitted: %#v", out)
	}
}

func TestASTParse_GoSymbolSpansUseOriginalLines(t *testing.T) {
	source := "package sample\n\nfunc Exported() {\n\t_ = 1\n}\n\nfunc hidden() {}\n"
	file := operators.ParsedFile{
		FileMeta:        mustMeta("sample.go", "go"),
		Content:         []byte(source),
		SourceStartLine: 40,
		SourceEndLine:   47,
		ParseOptions: operators.ParseOptions{
			OnlySig:      true,
			ExportedOnly: true,
		},
		ParseOptionsSet: true,
	}
	out := runASTParse(t, file)
	if len(out) != 1 {
		t.Fatalf("ASTParse returned %d records, want one", len(out))
	}
	if len(out[0].SymbolSpans) != 1 {
		t.Fatalf("got %d symbol spans, want only the exported function: %#v", len(out[0].SymbolSpans), out[0].SymbolSpans)
	}
	span := out[0].SymbolSpans[0]
	ktest.RequireEqual(t, span.Name, "Exported")
	ktest.RequireEqual(t, span.Kind, "func")
	ktest.RequireEqual(t, span.StartLine, 42)
	ktest.RequireEqual(t, span.EndLine, 44)
}

// ── helpers ──────────────────────────────────────────────────────────

func runASTParse(t *testing.T, file operators.ParsedFile) []operators.ParsedFile {
	t.Helper()

	upstream := func(yield func(operators.ParsedFile, error) bool) {
		yield(file, nil)
	}
	op := operators.ASTParse(operators.ASTConfig(file.ParseOptions))
	seq := op(upstream)

	var out []operators.ParsedFile
	seq(func(item operators.ParsedFile, err error) bool {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		out = append(out, item)
		return true
	})
	return out
}

func combinedContent(files []operators.ParsedFile) string {
	var b strings.Builder
	for _, f := range files {
		b.Write(f.Content)
		for _, s := range f.Snippets {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func mustMeta(path, lang string) fs.FileMeta {
	return fs.FileMeta{Path: path, RelPath: path, Lang: lang}
}
