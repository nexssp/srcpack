package operators

import "testing"

func TestSourceLineSpansSkipAmbiguousMatches(t *testing.T) {
	source := []byte("repeat\nkeep\nrepeat\n")
	output := []byte("repeat\nkeep\n")

	got := sourceLineSpans(source, output, 10, false)
	if len(got) != 1 {
		t.Fatalf("got %#v, want only the unambiguous line mapping", got)
	}
	want := SourceLineSpan{OutputLine: 2, SourceStartLine: 11, SourceEndLine: 11}
	if got[0] != want {
		t.Fatalf("got %#v, want %#v", got[0], want)
	}
}
