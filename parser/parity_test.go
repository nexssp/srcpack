package parser

import (
	"reflect"
	"testing"
)

func TestParseSignatureOutputParity(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		lang      string
		source    string
		snippets  []string
		funcCount int
	}{
		{
			name:      "Go",
			path:      "sample.go",
			lang:      "go",
			source:    "package sample\nfunc Visible() int { return 1 }\nfunc hidden() {}\n",
			snippets:  []string{"func Visible() int {}"},
			funcCount: 1,
		},
		{
			name:      "Rust",
			path:      "sample.rs",
			lang:      "rust",
			source:    "pub fn visible() -> i32 { 1 }\nfn hidden() {}\n",
			snippets:  []string{"pub fn visible() -> i32 {}"},
			funcCount: 1,
		},
		{
			name:      "TypeScript",
			path:      "sample.ts",
			lang:      "typescript",
			source:    "export function visible(): number { return 1; }\nfunction hidden() {}\n",
			snippets:  []string{"export function visible(): number {}"},
			funcCount: 1,
		},
		{
			name:      "JavaScript",
			path:      "sample.js",
			lang:      "javascript",
			source:    "export function visible() { return 1; }\nfunction hidden() {}\n",
			snippets:  []string{"export function visible() {}"},
			funcCount: 1,
		},
		{
			name:      "PHP",
			path:      "sample.php",
			lang:      "php",
			source:    "<?php\nfunction visible() { return 1; }\nfunction hidden() {}\n",
			snippets:  []string{"function visible() {}", "function hidden() {}"},
			funcCount: 2,
		},
		{
			name:      "Python",
			path:      "sample.py",
			lang:      "python",
			source:    "def visible():\n    return 1\n\ndef _hidden():\n    return 0\n",
			snippets:  []string{"def visible(): {}"},
			funcCount: 1,
		},
		{
			name:   "Svelte",
			path:   "App.svelte",
			lang:   "svelte",
			source: "<script>\nexport function visible() { return 1; }\n</script>\n<Widget title={name} />\n",
			snippets: []string{
				"export function visible() {}",
				"// --- UI COMPONENTS ---",
				"<Widget title />",
			},
			funcCount: 1,
		},
		{
			name:   "Vue",
			path:   "App.vue",
			lang:   "vue",
			source: "<script>\nexport function visible() { return 1; }\n</script>\n<Widget title={name} />\n",
			snippets: []string{
				"export function visible() {}",
				"// --- UI COMPONENTS ---",
				"<Widget title />",
			},
			funcCount: 1,
		},
		{
			name:      "Zig",
			path:      "sample.zig",
			lang:      "zig",
			source:    "pub fn visible() void { return; }\nfn hidden() void {}\n",
			snippets:  []string{"pub fn visible() void {}"},
			funcCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.path, []byte(tt.source), tt.lang, Options{ExportedOnly: true})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			want := Result{Snippets: tt.snippets, FuncCount: tt.funcCount}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Parse() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestParseWholeDocumentAndNoBodyFallbackParity(t *testing.T) {
	const source = "workflow: source\n"
	whole, err := Parse("sample.nflow", []byte(source), "nflow", Options{RemoveBodyFunc: true})
	if err != nil {
		t.Fatalf("Parse(nflow) error = %v", err)
	}
	if whole.Content != source || len(whole.Snippets) != 0 {
		t.Fatalf("Parse(nflow) = %#v, want unchanged full content", whole)
	}

	noBody, err := Parse("sample.unknown", []byte("private body\n"), "unknown", Options{RemoveBodyFunc: true})
	if err != nil {
		t.Fatalf("Parse(unknown) error = %v", err)
	}
	if noBody.Content != "" || len(noBody.Snippets) != 0 {
		t.Fatalf("Parse(unknown, RemoveBodyFunc) = %#v, want no content", noBody)
	}
}
