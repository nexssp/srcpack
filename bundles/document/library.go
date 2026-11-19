// Package document supplies a copyable NFlow document formatter. It depends
// only on Flow and kernel; no source-packaging domain types or I/O effects.
package document

import (
	"embed"
	"io/fs"
	"path/filepath"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"
)

const ID = "document"

//go:embed *.nflow
var FS embed.FS

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/document/nexssflow", Bundle)
}

// Bundle retains the historic registration for Go hosts. It intentionally
// mounts no actions: document formatting and interpolation now live in NFlow.
func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: ID, Libraries: []action.Library{{Name: ID}}}
}

// Loader resolves the embedded NFlow sources of this standalone bundle. The
// parent application may compose it with its own namespaced source loader.
func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

func (sourceLoader) Load(_, target string) ([]byte, string, error) {
	name := filepath.Base(target)
	data, err := fs.ReadFile(FS, name)
	return data, name, err
}
