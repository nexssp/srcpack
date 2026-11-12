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

func TestASTParse_PythonArchUsesStubSyntax(t *testing.T) {
	file := operators.ParsedFile{
		FileMeta: mustMeta("example.py", "python"),
		Content:  []byte("def load_config(path: str) -> dict:\n    return {}\n\nclass UserService:\n    def get_user(self, user_id: int) -> dict:\n        return {}\n"),
		ParseOptions: operators.ParseOptions{
			OnlySig:        true,
			Comments:       false,
			ExportedOnly:   true,
			TrimBlankLines: true,
		},
		ParseOptionsSet: true,
	}

	combined := combinedContent(runASTParse(t, file))
	for _, want := range []string{
		"def load_config(path: str) -> dict: ...",
		"class UserService:\n    def get_user(self, user_id: int) -> dict: ...",
	} {
		if !strings.Contains(combined, want) {
			t.Errorf("Python architecture output missing %q:\n%s", want, combined)
		}
	}
	if strings.Contains(combined, "::") || strings.Contains(combined, "{}") {
		t.Errorf("Python architecture output contains synthetic parser artifacts:\n%s", combined)
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
			Mask:          true,
			IncludeConfig: true,
		},
		ParseOptionsSet: true,
	}
	out := runASTParse(t, file)
	if len(out) != 1 {
		t.Fatalf("ASTParse returned %d records, want one config record", len(out))
	}
	got := string(out[0].Content)
	if !strings.Contains(got, `api_key: "[REDACTED_CREDENTIAL]"`) || strings.Contains(got, "test-secret-value-123") {
		t.Fatal("included config credential was not redacted")
	}

	file.ParseOptions.IncludeConfig = false
	out = runASTParse(t, file)
	if len(out) != 1 || len(out[0].Content) != 0 {
		t.Fatalf("config without --include-config was not omitted: %#v", out)
	}
}

func TestASTParse_IncludeConfigHonorsDisabledMasking(t *testing.T) {
	const secret = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
	file := operators.ParsedFile{
		FileMeta: mustMeta("config.yml", "text"),
		Content:  []byte("token: " + secret + "\n"),
		ParseOptions: operators.ParseOptions{
			IncludeConfig: true,
			Mask:          false,
		},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)
	if len(out) != 1 {
		t.Fatalf("ASTParse returned %d records, want one config record", len(out))
	}
	got := string(out[0].Content)
	if !strings.Contains(got, secret) || strings.Contains(got, "[REDACTED_SECRET]") {
		t.Fatal("disabled masking changed config content")
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

func TestASTParse_SourceLineSpansUseOriginalFileLines(t *testing.T) {
	const source = "package sample\n\nfunc Public() int {\n\treturn 7\n}\n"
	file := operators.ParsedFile{
		FileMeta:        mustMeta("sample.go", "go"),
		Content:         []byte(source),
		SourceStartLine: 40,
		SourceEndLine:   44,
		ParseOptions:    operators.ParseOptions{Comments: true},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)
	if len(out) != 1 {
		t.Fatalf("ASTParse returned %d records, want one", len(out))
	}
	if len(out[0].LineSpans) == 0 {
		t.Fatal("ASTParse returned no source-line spans")
	}
	span := out[0].LineSpans[0]
	ktest.RequireEqual(t, span.OutputLine, 1)
	ktest.RequireEqual(t, span.SourceStartLine, 40)
	ktest.RequireEqual(t, span.SourceEndLine, 40)
}

func TestASTParse_DenseSignatureLineSpanUsesOriginalDeclaration(t *testing.T) {
	const source = "package sample\n\n// Public is visible.\nfunc Public(\n\tvalue int,\n) string {\n\treturn \"ok\"\n}\n"
	file := operators.ParsedFile{
		FileMeta:        mustMeta("sample.go", "go"),
		Content:         []byte(source),
		SourceStartLine: 1,
		SourceEndLine:   8,
		ParseOptions:    operators.ParseOptions{OnlySig: true, Dense: true, ExportedOnly: true},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)
	if len(out) != 1 {
		t.Fatalf("ASTParse returned %d records, want one", len(out))
	}
	if len(out[0].LineSpans) != 1 {
		t.Fatalf("dense line spans = %#v, want one declaration span", out[0].LineSpans)
	}
	span := out[0].LineSpans[0]
	ktest.RequireEqual(t, span.OutputLine, 1)
	ktest.RequireEqual(t, span.SourceStartLine, 4)
	ktest.RequireEqual(t, span.SourceEndLine, 8)
}

func TestASTParse_DenseGroupedDeclarationsDoNotMisattributeReferences(t *testing.T) {
	const source = "package sample\n\ntype (\n\tFirst struct{}\n\tShared struct{}\n)\n\nfunc Public(value Shared) {}\n"
	file := operators.ParsedFile{
		FileMeta:        mustMeta("sample.go", "go"),
		Content:         []byte(source),
		SourceStartLine: 1,
		SourceEndLine:   8,
		ParseOptions:    operators.ParseOptions{OnlySig: true, Dense: true, ExportedOnly: true},
		ParseOptionsSet: true,
	}

	out := runASTParse(t, file)
	if len(out) != 1 {
		t.Fatalf("ASTParse returned %d records, want one", len(out))
	}
	for _, span := range out[0].LineSpans {
		if span.SourceStartLine == 8 && span.SourceEndLine == 8 {
			return
		}
	}
	t.Fatalf("dense function line was not mapped to Public's original declaration: %#v", out[0].LineSpans)
}

func TestASTParse_PassThroughCountsOnlyWhenRequested(t *testing.T) {
	source := []byte("package sample\nfunc One() {}\nfunc Two() {}\n")
	for _, tc := range []struct {
		name          string
		needFuncCount bool
		wantCount     int
	}{
		{name: "not needed", wantCount: 0},
		{name: "summary or tree needs count", needFuncCount: true, wantCount: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := operators.ParsedFile{
				FileMeta: mustMeta("sample.go", "go"), Content: source,
				ParseOptions: operators.ParseOptions{Comments: true, NeedFuncCount: tc.needFuncCount}, ParseOptionsSet: true,
			}
			out := runASTParse(t, file)
			if len(out) != 1 {
				t.Fatalf("got %d records, want one", len(out))
			}
			if out[0].FuncCount != tc.wantCount {
				t.Fatalf("function count = %d, want %d", out[0].FuncCount, tc.wantCount)
			}
			if string(out[0].Content) != string(source) {
				t.Fatalf("pass-through content changed: %q", out[0].Content)
			}
		})
	}
}

func TestASTParse_PreservesTreeExpansionMarker(t *testing.T) {
	file := operators.ParsedFile{
		FileMeta:   mustMeta("sample.go", "go"),
		Content:    []byte("package sample\n"),
		ExpandTree: true,
	}
	out := runASTParse(t, file)
	if len(out) != 1 || !out[0].ExpandTree {
		t.Fatalf("AST parse lost folder expansion metadata: %#v", out)
	}
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
