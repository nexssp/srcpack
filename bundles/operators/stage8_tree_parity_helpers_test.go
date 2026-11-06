package operators

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nexssp/kernel/xtest/ktest"
)

var stage8LangLine = regexp.MustCompile(`(?m)^   Languages: .*\n`)

func stage8ParityFiles() []ParsedFile {
	return []ParsedFile{
		{Path: "main.go", RelPath: "cmd/app/main.go", Lang: "go", Size: 35, Content: []byte("package app\nfunc Main() {}\n"), FuncCount: 1, Lines: 2},
		{Path: "app.go", RelPath: "cmd/app/app.go", Lang: "go", Size: 35, Content: []byte("package app\nfunc App() {}\n"), FuncCount: 1, Lines: 2},
		{Path: "app_test.go", RelPath: "cmd/app/app_test.go", Lang: "go", Size: 22, Snippets: []string{"func TestApp() {}\n"}, FuncCount: 1, Lines: 1},
		{Path: "guide.md", RelPath: "docs/guide.md", Lang: "markdown", Size: 14, Content: []byte("# guide\n"), Lines: 1},
		{Path: "settings.yaml", RelPath: "config/settings.yaml", Lang: "yaml", Size: 10, Content: []byte("x: 1\n"), Lines: 1},
		{Path: "empty.go", RelPath: "cmd/app/empty.go", Lang: "go", Size: 0, Snippets: []string{""}, Lines: 0},
		{Path: "duplicate.go", RelPath: "cmd/app/main.go", Lang: "go", Size: 10, Content: []byte("package app\n"), Lines: 1},
		{Path: "vendor.go", RelPath: "vendor/lib.go", Lang: "go", Size: 30, Omitted: true, OmitReason: "deps"},
		{Path: "secret.go", RelPath: "internal/secret.go", Lang: "go", Size: 19, Omitted: true, OmitReason: "excluded"},
		{Path: "large.bin", RelPath: "assets/large.bin", Size: 2048, Omitted: true, OmitReason: "size"},
		{Path: "test.go", RelPath: "test/examples/sample_test.go", Lang: "go", Size: 18, Omitted: true, OmitReason: "test"},
		{Path: "example.go", RelPath: "examples/demo.go", Lang: "go", Size: 18, Omitted: true, OmitReason: "examples"},
		{Path: "generated.go", RelPath: "generated/output.go", Lang: "go", Size: 18, Omitted: true, OmitReason: "generated"},
		{Path: "extension.go", RelPath: "extensions/ext.go", Lang: "go", Size: 18, Omitted: true, OmitReason: "ext"},
	}
}

func stage8FullFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"cmd/app/main.go", "cmd/app/app.go", "cmd/app/app_test.go", "docs/guide.md", "config/settings.yaml", "cmd/app/empty.go", "vendor/lib.go", "internal/secret.go", "assets/large.bin", "test/examples/sample_test.go", "examples/demo.go", "generated/output.go", "extensions/ext.go", ".hidden/no.go", "node_modules/x.js"} {
		p := filepath.Join(root, rel)
		ktest.RequireNoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		ktest.RequireNoError(t, os.WriteFile(p, []byte("0123456789abcdefghij"), 0o600))
		stamp := time.Now().Add(-48 * time.Hour)
		ktest.RequireNoError(t, os.Chtimes(p, stamp, stamp))
	}
	return root
}

func stage8NormalizeLanguageTies(text string) string {
	return stage8LangLine.ReplaceAllStringFunc(text, func(line string) string {
		body := strings.TrimSuffix(strings.TrimPrefix(line, "   Languages: "), "\n")
		parts := strings.Split(body, " · ")
		sort.Strings(parts)
		return "   Languages: " + strings.Join(parts, " · ") + "\n"
	})
}

func TestStage8ParityEmptyAndAllOmittedInputs(t *testing.T) {
	ktest.RequireEqual(t, GenerateContextTree(nil, 0, false, false), "")
	allOmitted := []ParsedFile{
		{Path: "vendor.go", RelPath: "vendor/lib.go", Lang: "go", Size: 30, Omitted: true, OmitReason: "deps"},
		{Path: "secret.go", RelPath: "internal/secret.go", Lang: "go", Size: 19, Omitted: true, OmitReason: "excluded"},
	}
	ktest.RequireStringContains(t, GenerateContextTree(allOmitted, 0, false, true), "excluded")
}
