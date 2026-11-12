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
	if err := json.Unmarshal([]byte(result["content"].(string)), &payload); err != nil {
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
