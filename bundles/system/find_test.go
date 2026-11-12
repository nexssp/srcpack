package systemcmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDeclarationsReturnsGoAndNFlowSpans(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\n\nfunc Lookup() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "flow.nflow"), []byte("@pipeline lookup\n  -> one\n@end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := findDeclarations(context.Background(), root, "lookup", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want Go and NFlow declarations", matches)
	}
	if matches[0].Path != "flow.nflow" || matches[0].StartLine != 1 || matches[0].EndLine != 3 {
		t.Fatalf("NFlow match = %#v", matches[0])
	}
	if matches[1].Path != "sample.go" || matches[1].StartLine != 3 || matches[1].EndLine != 3 {
		t.Fatalf("Go match = %#v", matches[1])
	}
}

func TestFindJSONIncludesQueryAndMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc Lookup() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := find(context.Background(), map[string]any{"query": "Lookup", "root": root, "json": true})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Query   string      `json:"query"`
		Matches []findMatch `json:"matches"`
	}
	content, ok := result["content"].(string)
	if !ok {
		t.Fatalf("find result content = %T, want string", result["content"])
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Query != "Lookup" || len(payload.Matches) != 1 || !strings.HasSuffix(payload.Matches[0].Path, "sample.go") {
		t.Fatalf("JSON payload = %#v", payload)
	}
}

func TestFindDeclarationsSupportsAllSourceLanguages(t *testing.T) {
	cases := []struct {
		ext, source, query, kind string
	}{
		{".ts", "export interface Widget { id: string }\n", "Widget", "interface"},
		{".tsx", "export function Widget() { return null }\n", "Widget", "func"},
		{".js", "export class Widget {}\n", "Widget", "class"},
		{".jsx", "export const Widget = () => null\n", "Widget", "const"},
		{".mjs", "export function Widget() {}\n", "Widget", "func"},
		{".cjs", "class Widget {}\n", "Widget", "class"},
		{".py", "class Widget:\n    pass\n", "Widget", "class"},
		{".pyi", "def Widget() -> None: ...\n", "Widget", "func"},
		{".rs", "pub struct Widget { }\n", "Widget", "struct"},
		{".php", "<?php\nclass Widget {}\n", "Widget", "class"},
		{".zig", "const Widget = struct { };\n", "Widget", "struct"},
		{".svelte", "<script>\nfunction Widget() {}\n</script>\n", "Widget", "func"},
		{".vue", "<script>\nexport const Widget = {};\n</script>\n", "Widget", "const"},
		{".md", "# Widget\n", "Widget", "heading"},
		{".mdx", "# Widget\n", "Widget", "heading"},
		{".markdown", "## Widget\n", "Widget", "heading"},
		{".html", "<Widget></Widget>\n", "Widget", "element"},
		{".htm", "<Widget></Widget>\n", "Widget", "element"},
		{".css", ".Widget { color: red; }\n", ".Widget", "selector"},
		{".scss", "$Widget: red;\n", "$Widget", "variable"},
	}
	for _, tc := range cases {
		t.Run(tc.ext, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "sample"+tc.ext)
			if err := os.WriteFile(path, []byte(tc.source), 0o600); err != nil {
				t.Fatal(err)
			}
			matches, err := findDeclarations(context.Background(), root, tc.query, tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 1 || matches[0].Name != tc.query || matches[0].Kind != tc.kind || matches[0].StartLine < 1 {
				t.Fatalf("matches = %#v, want one %s declaration", matches, tc.kind)
			}
		})
	}
}

func TestFindFallsBackToTextAndKeepsDeclarationMatches(t *testing.T) {
	root := t.TempDir()
	for path, source := range map[string]string{
		"Makefile":      "build:\n\t@echo needle\n",
		"guide.md":      "# Guide\nThe needle is in the body.\n",
		"reference.md":  "# Needle\nNeedle appears again.\n",
		"settings.yaml": "service:\n  name: Needle\n",
		"source.go":     "package sample\n// needle in a comment\nfunc Run() {}\n",
	} {
		writeFindFile(t, root, path, source)
	}

	matches, err := findDeclarations(context.Background(), root, "NEEDLE", "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]findMatch{
		"Makefile":      {Name: "@echo needle", Kind: "text", Path: "Makefile", StartLine: 2, EndLine: 2},
		"guide.md":      {Name: "The needle is in the body.", Kind: "text", Path: "guide.md", StartLine: 2, EndLine: 2},
		"reference.md":  {Name: "Needle", Kind: "heading", Path: "reference.md", StartLine: 1, EndLine: 1},
		"settings.yaml": {Name: "name: Needle", Kind: "text", Path: "settings.yaml", StartLine: 2, EndLine: 2},
		"source.go":     {Name: "// needle in a comment", Kind: "text", Path: "source.go", StartLine: 2, EndLine: 2},
	}
	if len(matches) != len(want) {
		t.Fatalf("matches = %#v, want one result per file: %#v", matches, want)
	}
	for _, match := range matches {
		if expected, ok := want[match.Path]; !ok || expected != match {
			t.Errorf("match = %#v, want %#v", match, expected)
		}
	}

	textMatches, err := findDeclarations(context.Background(), root, "needle", "text")
	if err != nil {
		t.Fatal(err)
	}
	var referenceLines []int
	for _, match := range textMatches {
		if match.Path == "reference.md" {
			referenceLines = append(referenceLines, match.StartLine)
		}
	}
	if len(referenceLines) != 2 || referenceLines[0] != 1 || referenceLines[1] != 2 {
		t.Fatalf("text-kind matches in reference.md = %v, want lines 1 and 2", referenceLines)
	}
}

func TestFindHonorsPackerIgnoresAndFileExclusions(t *testing.T) {
	root := t.TempDir()
	for path, source := range map[string]string{
		".gitignore":                  "ignored.yaml\nignored-dir/\n",
		"nexss.srcpack.ignore":        "srcpack.txt\n",
		"config.yaml":                 "needle: keep\n",
		"ignored.yaml":                "needle: ignore\n",
		"ignored-dir/child.txt":       "needle: ignore\n",
		"srcpack.txt":                 "needle: ignore\n",
		"dist/generated.txt":          "needle: ignore\n",
		"node_modules/dependency.txt": "needle: ignore\n",
		"vendor/library.txt":          "needle: ignore\n",
		"_srcpack/output.txt":         "needle: ignore\n",
		"credentials.json":            "needle: ignore\n",
		"private.pem":                 "needle: ignore\n",
		"package-lock.json":           "needle: ignore\n",
		"image.png":                   "needle: ignore\n",
	} {
		writeFindFile(t, root, path, source)
	}

	matches, err := findDeclarations(context.Background(), root, "needle", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Path != "config.yaml" || matches[0].Kind != "text" {
		t.Fatalf("matches = %#v, want only config.yaml text match", matches)
	}
}

func writeFindFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
