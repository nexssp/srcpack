package operators

import (
	"context"
	"strings"

	"github.com/nexssp/flow/extensions/fs"
	"github.com/nexssp/kernel/action"
)

// JoinRendered concatenates already-rendered content chunks without adding
// separators. RenderMarkdown owns all framing and newline semantics.
func JoinRendered(files []fs.FileContent) string {
	var total int
	for _, file := range files {
		total += len(file.Content)
	}

	var output strings.Builder
	output.Grow(total)
	for _, file := range files {
		_, _ = output.Write(file.Content)
	}
	return output.String()
}

// JoinRenderedAction turns the []FileContent result of Flow's collect boundary
// into a plain string, which transports such as tcli can emit as text.
func JoinRenderedAction() action.AnyAction {
	return action.New("srcpack.join", func(_ context.Context, files []fs.FileContent) (string, error) {
		return JoinRendered(files), nil
	}).Description("Concatenate rendered file content into one text result").Build()
}
