package parser

import (
	"bytes"
	"strings"
	"testing"
)

func TestCleanSource_ParityAndEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("Comments in string literals must not be stripped", func(t *testing.T) {
		t.Parallel()

		code := []byte(`const url = "http://example.com/api"; // comment`)
		var buf bytes.Buffer
		CleanSource(code, Options{StripComments: true}, &buf)

		out := buf.String()
		if !strings.Contains(out, `"http://example.com/api"`) {
			t.Fatalf("URL inside string was corrupted: %s", out)
		}
		if strings.Contains(out, "// comment") {
			t.Fatalf("comment was not stripped: %s", out)
		}
	})

	t.Run("Trim indent and blank line collapsing", func(t *testing.T) {
		t.Parallel()

		code := []byte("line1\n\t   line2\n\n\n\n\nline3\n")
		var buf bytes.Buffer
		CleanSource(code, Options{TrimIndent: true, TrimBlankLines: true}, &buf)

		expected := "line1\nline2\n\nline3\n"
		if buf.String() != expected {
			t.Fatalf("unexpected formatting:\ngot:\n%q\nwant:\n%q", buf.String(), expected)
		}
	})
}
