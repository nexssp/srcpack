package operators

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

// ── header and legend ────────────────────────────────────────────────

func TestGenerateContextTree_HeaderAndLegend(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "a.go", RelPath: "a.go", Lang: "go", Content: []byte("package a\n"), FuncCount: 1, Lines: 5},
		{Path: "b.go", RelPath: "b.go", Lang: "go", Content: []byte("package a\n"), FuncCount: 2, Lines: 10},
	}

	tree := GenerateContextTree(files, 2, false, true)

	ktest.RequireStringContains(t, tree, "📦 Context Tree (2 files")
	ktest.RequireStringContains(t, tree, "Languages: go")
	ktest.RequireStringContains(t, tree, "📄 a.go")
	ktest.RequireStringContains(t, tree, "📄 b.go")
	ktest.RequireStringContains(t, tree, "1 fn")
	ktest.RequireStringContains(t, tree, "2 fn")
}

func TestGenerateContextTree_EmptyInput(t *testing.T) {
	t.Parallel()

	ktest.RequireEqual(t, GenerateContextTree(nil, 2, false, false), "")
}

// ── omitted markers ──────────────────────────────────────────────────

func TestGenerateContextTree_OmittedMarker(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "main.go", RelPath: "main.go", Lang: "go", Content: []byte("package a\n"), Lines: 3},
		{Path: "main_test.go", RelPath: "main_test.go", Lang: "go", Omitted: true, OmitReason: "test"},
	}

	tree := GenerateContextTree(files, 2, false, true)

	ktest.RequireStringContains(t, tree, "1 files")
	ktest.RequireStringContains(t, tree, "1 excluded")
	ktest.RequireStringContains(t, tree, "🧪 main_test.go")
	ktest.RequireStringContains(t, tree, "excluded")
}

func TestGenerateContextTree_IgnoresNonTreeVisibleReasons(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "main.go", RelPath: "main.go", Lang: "go", Content: []byte("package a\n"), Lines: 1},
		{Path: "vendor/lib.go", RelPath: "vendor/lib.go", Lang: "go", Omitted: true, OmitReason: "deps"},
		{Path: "binary.wasm", RelPath: "binary.wasm", Lang: "text", Omitted: true, OmitReason: "binary"},
	}

	tree := GenerateContextTree(files, 2, false, false)

	if strings.Contains(tree, "vendor") {
		t.Errorf("deps exclusions should not appear in tree:\n%s", tree)
	}
	if strings.Contains(tree, "binary.wasm") {
		t.Errorf("binary exclusions should not appear in tree:\n%s", tree)
	}
}

// ── test pairing ─────────────────────────────────────────────────────

func TestGenerateContextTree_TestPairing(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "svc.go", RelPath: "svc.go", Lang: "go", Content: []byte("package a\n"), Lines: 3},
		{Path: "svc_test.go", RelPath: "svc_test.go", Lang: "go", Content: []byte("package a\n"), Lines: 3},
	}

	tree := GenerateContextTree(files, 2, false, true)

	ktest.RequireStringContains(t, tree, "↔ svc_test.go")
	ktest.RequireStringContains(t, tree, "↔ svc.go")
}

// ── role icons ───────────────────────────────────────────────────────

func TestGenerateContextTree_RoleIcons(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "main.go", RelPath: "main.go", Lang: "go", Content: []byte("package a\n"), Lines: 1},
		{Path: "README.md", RelPath: "README.md", Lang: "markdown", Content: []byte("# hi\n"), Lines: 1},
		{Path: "go.mod", RelPath: "go.mod", Lang: "text", Content: []byte("module x\n"), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, false, true)

	ktest.RequireStringContains(t, tree, "🚀 main.go")
	ktest.RequireStringContains(t, tree, "📝 README.md")
}

// ── rollup and depth ─────────────────────────────────────────────────

func TestGenerateContextTree_DirectoryRollup(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "pkg/a.go", RelPath: "pkg/a.go", Lang: "go", Content: []byte("package pkg\n"), FuncCount: 3, Lines: 5},
		{Path: "pkg/b.go", RelPath: "pkg/b.go", Lang: "go", Content: []byte("package pkg\n"), FuncCount: 4, Lines: 6},
	}

	tree := GenerateContextTree(files, 2, false, false)

	ktest.RequireStringContains(t, tree, "📁 pkg/")
	ktest.RequireStringContains(t, tree, "2 files")
	ktest.RequireStringContains(t, tree, "7 fn")
}

func TestGenerateContextTree_MaxDepthCollapses(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "a/b/c/deep.go", RelPath: "a/b/c/deep.go", Lang: "go", Content: []byte("package a\n"), Lines: 1},
	}

	tree := GenerateContextTree(files, 1, false, false)

	ktest.RequireStringContains(t, tree, "📁 a/")
	if strings.Contains(tree, "deep.go") {
		t.Errorf("maxDepth=1 should hide deep.go, got:\n%s", tree)
	}
}

// ── sorting ──────────────────────────────────────────────────────────

func TestGenerateContextTree_DirectoriesSortByTokenWeight(t *testing.T) {
	t.Parallel()

	// small/ has ~1 token, big/ has ~300 tokens. big/ must render first
	// even though "big" sorts before "small" alphabetically.
	files := []ParsedFile{
		{Path: "small/a.go", RelPath: "small/a.go", Lang: "go", Content: []byte("x"), Lines: 1},
		{Path: "big/a.go", RelPath: "big/a.go", Lang: "go", Content: []byte(strings.Repeat("x", 1000)), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, false, false)

	bigIdx := strings.Index(tree, "📁 big/")
	smallIdx := strings.Index(tree, "📁 small/")
	if bigIdx < 0 || smallIdx < 0 {
		t.Fatalf("expected both directories in tree:\n%s", tree)
	}
	if bigIdx > smallIdx {
		t.Errorf("big/ should render before small/ (by token weight):\n%s", tree)
	}
}

func TestGenerateContextTree_FilesSortAlphabeticallyWithinDirectory(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "pkg/z.go", RelPath: "pkg/z.go", Lang: "go", Content: []byte("z"), Lines: 1},
		{Path: "pkg/a.go", RelPath: "pkg/a.go", Lang: "go", Content: []byte("a"), Lines: 1},
		{Path: "pkg/m.go", RelPath: "pkg/m.go", Lang: "go", Content: []byte("m"), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, false, true)

	aIdx := strings.Index(tree, "a.go")
	mIdx := strings.Index(tree, "m.go")
	zIdx := strings.Index(tree, "z.go")
	if aIdx < 0 || mIdx < 0 || zIdx < 0 {
		t.Fatalf("expected all three files in tree:\n%s", tree)
	}
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("files should sort alphabetically a < m < z:\n%s", tree)
	}
}

// ── color ───────────────────────────────────────────────────────────

func TestGenerateContextTree_ColorDisabledByDefault(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "pkg/a.go", RelPath: "pkg/a.go", Lang: "go", Content: []byte("package pkg\n"), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, false, false)

	if strings.Contains(tree, "\x1b[") {
		t.Errorf("color=false must not emit ANSI escapes:\n%q", tree)
	}
}

func TestGenerateContextTree_ColorAddsANSIOnShare(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "pkg/a.go", RelPath: "pkg/a.go", Lang: "go", Content: []byte("package pkg\n"), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, true, false)

	ktest.RequireStringContains(t, tree, "\x1b[")
	ktest.RequireStringContains(t, tree, "100.0%")
}

// ── files hidden by default ──────────────────────────────────────────

func TestGenerateContextTree_HidesFilesByDefault(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "pkg/a.go", RelPath: "pkg/a.go", Lang: "go", Content: []byte("package pkg\n"), Lines: 1},
		{Path: "pkg/b.go", RelPath: "pkg/b.go", Lang: "go", Content: []byte("package pkg\n"), Lines: 1},
	}

	foldersOnly := GenerateContextTree(files, 2, false, false)
	if strings.Contains(foldersOnly, "a.go") || strings.Contains(foldersOnly, "b.go") {
		t.Errorf("files must be hidden by default:\n%s", foldersOnly)
	}
	ktest.RequireStringContains(t, foldersOnly, "📁 pkg/")
	ktest.RequireStringContains(t, foldersOnly, "2 files")

	withFiles := GenerateContextTree(files, 2, false, true)
	ktest.RequireStringContains(t, withFiles, "a.go")
	ktest.RequireStringContains(t, withFiles, "b.go")
}

func TestGenerateContextTree_NoLangInFileMeta(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "pkg/a.go", RelPath: "pkg/a.go", Lang: "go", Content: []byte("package pkg\n"), FuncCount: 2, Lines: 5},
	}

	tree := GenerateContextTree(files, 2, false, true)

	// The extension already says "go". The meta must not repeat it.
	line := findLineContaining(tree, "a.go")
	if strings.Contains(line, "[go ·") || strings.Contains(line, "[go]") {
		t.Errorf("language should not appear in file meta:\n%s", line)
	}
}

// ── footer ───────────────────────────────────────────────────────────

func TestGenerateContextTree_HasTotalFooter(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "pkg/a.go", RelPath: "pkg/a.go", Lang: "go", Size: 100, Content: []byte("package pkg\n"), Lines: 1},
		{Path: "pkg/b.go", RelPath: "pkg/b.go", Lang: "go", Size: 100, Content: []byte("package pkg\n"), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, false, false)

	ktest.RequireStringContains(t, tree, "TOTAL")
	ktest.RequireStringContains(t, tree, "2 files")
	ktest.RequireStringContains(t, tree, "tokens")
}

func TestGenerateContextTree_CompressionReportsSavings(t *testing.T) {
	t.Parallel()

	// 1000 raw bytes, 500 packed bytes → 50% saved.
	files := []ParsedFile{
		{Path: "a.go", RelPath: "a.go", Lang: "go", Size: 1000, Content: []byte(strings.Repeat("x", 500)), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, false, false)

	ktest.RequireStringContains(t, tree, "50.0%")
	ktest.RequireStringContains(t, tree, "500 B")
	ktest.RequireStringContains(t, tree, "1000 B")
}

func TestGenerateContextTree_CompressionBarUsesSavingsAndEmptyColours(t *testing.T) {
	t.Parallel()

	// 50% saved → green filled portion and a dim empty tail.
	files := []ParsedFile{
		{Path: "a.go", RelPath: "a.go", Lang: "go", Size: 1000, Content: []byte(strings.Repeat("x", 500)), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, true, false)

	ktest.RequireStringContains(t, tree, "\x1b[38;5;118m") // medium green = 50% savings
	ktest.RequireStringContains(t, tree, "\x1b[2;37m")     // dim = recovered/empty space
	if strings.Contains(tree, "\x1b[1;31m") {
		t.Errorf("50%% savings must not use the low-savings red:")
	}
}

func TestGenerateContextTree_CompressionBarIsGreenFilled(t *testing.T) {
	t.Parallel()

	// 50% saved → 50% filled green, 50% empty.
	files := []ParsedFile{
		{Path: "a.go", RelPath: "a.go", Lang: "go", Size: 1000, Content: []byte(strings.Repeat("x", 500)), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, true, false)

	ktest.RequireStringContains(t, tree, "\x1b[38;5;118m") // medium-green fill at 50% savings
	if strings.Contains(tree, "\x1b[1;31m") {
		t.Errorf("compression bar must not use red:\n%q", tree)
	}
}

func TestGenerateContextTree_FooterReportsExcluded(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "main.go", RelPath: "main.go", Lang: "go", Size: 10, Content: []byte("package a\n"), Lines: 1},
		{Path: "main_test.go", RelPath: "main_test.go", Lang: "go", Omitted: true, OmitReason: "test"},
	}

	tree := GenerateContextTree(files, 2, false, false)

	lines := strings.Split(strings.TrimRight(tree, "\n"), "\n")
	last := lines[len(lines)-1]
	ktest.RequireStringContains(t, last, "TOTAL")
	ktest.RequireStringContains(t, last, "1 files")
	ktest.RequireStringContains(t, last, "1 excluded")
}

// ── full project tree ────────────────────────────────────────────────

func TestGenerateFullProjectTree_TagsContextAndExcluded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTreeFile(t, root, "main.go", "package a\n")
	writeTreeFile(t, root, "README.md", "# hi\n")
	writeTreeFile(t, root, "notes.txt", "x\n")

	files := []ParsedFile{
		{Path: filepath.Join(root, "main.go"), RelPath: "main.go", Lang: "go", Content: []byte("package a\n"), Lines: 1},
	}

	tree := GenerateFullProjectTree(root, files, 3, false, true)

	ktest.RequireStringContains(t, tree, "[context]")
	ktest.RequireStringContains(t, tree, "excluded")
	ktest.RequireStringContains(t, tree, "main.go")
	ktest.RequireStringContains(t, tree, "README.md")
	ktest.RequireStringContains(t, tree, "notes.txt")
	ktest.RequireStringContains(t, tree, "in context")
}

func TestGenerateFullProjectTree_SkipsHeavyDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTreeFile(t, root, "main.go", "package a\n")
	writeTreeFile(t, root, "node_modules/dep.go", "package d\n")
	writeTreeFile(t, root, "vendor/lib.go", "package v\n")

	tree := GenerateFullProjectTree(root, nil, 3, false, false)

	if strings.Contains(tree, "node_modules") {
		t.Errorf("node_modules should be pruned:\n%s", tree)
	}
	if strings.Contains(tree, "vendor") {
		t.Errorf("vendor should be pruned:\n%s", tree)
	}
}

func TestGenerateFullProjectTree_SkipsHiddenDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTreeFile(t, root, "main.go", "package a\n")
	writeTreeFile(t, root, ".cache/thing.go", "package c\n")
	writeTreeFile(t, root, ".git/config", "[core]\n")

	tree := GenerateFullProjectTree(root, nil, 3, false, false)

	if strings.Contains(tree, ".cache") {
		t.Errorf("hidden dirs should be pruned:\n%s", tree)
	}
	if strings.Contains(tree, ".git") {
		t.Errorf(".git should be pruned:\n%s", tree)
	}
}

func TestGenerateFullProjectTree_NoContextTagsInRegularTree(t *testing.T) {
	t.Parallel()

	files := []ParsedFile{
		{Path: "a.go", RelPath: "a.go", Lang: "go", Content: []byte("package a\n"), Lines: 1},
	}

	tree := GenerateContextTree(files, 2, false, false)

	if strings.Contains(tree, "[context]") {
		t.Errorf("regular context tree must not show [context] tags:\n%s", tree)
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func writeTreeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findLineContaining(haystack, needle string) string {
	for line := range strings.SplitSeq(haystack, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
