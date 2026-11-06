package operators

import (
	"testing"

	"github.com/nexssp/flow/extensions/fs"
)

func TestJoinRenderedConcatenatesContentWithoutChangingFraming(t *testing.T) {
	files := []fs.FileContent{
		{Content: []byte("# Header\n\n")},
		{Content: []byte("### a.go\n```go\npackage a\n```\n\n")},
		{Content: []byte("### b.go\n```go\npackage b\n```\n")},
	}
	want := "# Header\n\n### a.go\n```go\npackage a\n```\n\n### b.go\n```go\npackage b\n```\n"
	if got := JoinRendered(files); got != want {
		t.Fatalf("JoinRendered() = %q, want %q", got, want)
	}
}

func TestJoinRenderedEmptyInput(t *testing.T) {
	if got := JoinRendered(nil); got != "" {
		t.Fatalf("JoinRendered(nil) = %q, want empty string", got)
	}
}
