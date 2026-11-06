package parser

import (
	"strings"
	"testing"
)

func TestTextUtilities(t *testing.T) {
	t.Run("Strip All Comments", func(t *testing.T) {
		input := `// Header comment
package main

/* Multi-line
   comment */
func Hello() {
	var url = "http://example.com" // inline comment
	// standalone comment
}
`
		out := StripComments(input)
		if strings.Contains(out, "Header comment") || strings.Contains(out, "Multi-line") || strings.Contains(out, "inline comment") {
			t.Errorf("comments were not stripped:\n%s", out)
		}
		if !strings.Contains(out, `var url = "http://example.com"`) {
			t.Errorf("string literal with // was corrupted:\n%s", out)
		}
	})

	t.Run("Trim Indentation", func(t *testing.T) {
		input := "func Test() {\n\t\tlineOne()\n   lineTwo()\n}"
		expected := "func Test() {\nlineOne()\nlineTwo()\n}"
		out := TrimIndent(input)
		if out != expected {
			t.Errorf("trim indent failed:\ngot:\n%q\nwant:\n%q", out, expected)
		}
	})

	t.Run("Trim Blank Lines", func(t *testing.T) {
		input := "line1\n\n\n\nline2\n\nline3"
		expected := "line1\n\nline2\n\nline3"
		out := TrimBlankLines(input)
		if out != expected {
			t.Errorf("trim blank lines failed:\ngot:\n%q\nwant:\n%q", out, expected)
		}
	})

	t.Run("Truncate Large File", func(t *testing.T) {
		lines := make([]string, 50)
		for i := range 50 {
			lines[i] = "line"
		}
		src := []byte(strings.Join(lines, "\n"))
		opts := Options{
			MaxLines:     10,
			MaxLinesHead: 3,
			MaxLinesTail: 2,
		}

		out := Truncate(src, opts, []string{"func TruncatedSignatures() {}"})
		if !strings.Contains(out, "// --- TRUNCATED MIDDLE ---") {
			t.Errorf("expected middle truncation marker")
		}
		if !strings.Contains(out, "func TruncatedSignatures() {}") {
			t.Errorf("expected signatures section in truncated file")
		}
	})
}
