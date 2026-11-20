package operators

import (
	"strings"

	"github.com/nexssp/flow/extensions/fs"
)

// JoinRendered concatenates rendered file content without adding separators.
// The separate srcpack.join_output action still uses this helper; render-stream
// joining in NFlow uses bytes.concat instead.
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
