package operators

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundedOutputPreviewKeepsShortContent(t *testing.T) {
	got, truncated := boundedOutputPreview("one\ntwo\n")
	if got != "one\ntwo" || truncated {
		t.Fatalf("boundedOutputPreview() = %q, %v; want short content without truncation", got, truncated)
	}
}

func TestBoundedOutputPreviewLimitsLinesAndSignalsTruncation(t *testing.T) {
	content := strings.Repeat("line\n", outputPreviewMaxLines+2)
	got, truncated := boundedOutputPreview(content)
	if !truncated {
		t.Fatal("boundedOutputPreview() did not report line truncation")
	}
	want := strings.TrimSuffix(strings.Repeat("line\n", outputPreviewMaxLines), "\n")
	if got != want {
		t.Fatalf("preview = %q, want first %d lines %q", got, outputPreviewMaxLines, want)
	}
}

func TestBoundedOutputPreviewLimitsBytesAtUTF8Boundary(t *testing.T) {
	content := strings.Repeat("a", outputPreviewMaxBytes-1) + "界tail"
	got, truncated := boundedOutputPreview(content)
	if !truncated {
		t.Fatal("boundedOutputPreview() did not report byte truncation")
	}
	if len(got) != outputPreviewMaxBytes-1 {
		t.Fatalf("preview is %d bytes, want %d bytes before the split rune", len(got), outputPreviewMaxBytes-1)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("preview is not valid UTF-8: %q", got)
	}
}

func TestBoundedOutputPreviewSanitizesTerminalControls(t *testing.T) {
	got, _ := boundedOutputPreview("title\x1b[31mred\x1b[0m\tend")
	if strings.ContainsRune(got, '\x1b') || strings.ContainsRune(got, '\t') {
		t.Fatalf("preview contains terminal control characters: %q", got)
	}
	if !strings.Contains(got, "red") || !strings.Contains(got, "end") {
		t.Fatalf("sanitizing controls removed ordinary content: %q", got)
	}
}
