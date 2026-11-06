// Package tree provides the tree command's embedded NFlow files. The command
// intentionally reuses packcmd's resolver and pack_execute pipeline.
package tree

import "embed"

//go:embed *.nflow
var FS embed.FS
