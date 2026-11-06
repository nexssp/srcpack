package nflows

import (
	"embed"
	"io/fs"
	"path/filepath"

	"github.com/nexssp/flow/compiler"
)

//go:embed *.nflow
var FS embed.FS

// Loader returns a compiler.SourceLoader that resolves @include targets
// against the embedded .nflow sources. Paths inside the embedded FS
// use forward slashes only.
func Loader() compiler.SourceLoader {
	return embedLoader{}
}

// LoaderWithOverrides returns an embedded loader with in-memory overrides by
// basename. It is used when a CLI invocation must bind resolved values into a
// pipeline before Flow compiles its nested @include source.
func LoaderWithOverrides(overrides map[string][]byte) compiler.SourceLoader {
	return embedLoader{overrides: overrides}
}

type embedLoader struct {
	overrides map[string][]byte
}

func (l embedLoader) Load(_, target string) ([]byte, string, error) {
	// include/directive.go resolves targets to absolute filesystem
	// paths before calling the loader. The embedded sources form a flat
	// namespace, so we address them by base name.
	name := filepath.Base(target)
	if data, ok := l.overrides[name]; ok {
		return append([]byte(nil), data...), name, nil
	}
	data, err := fs.ReadFile(FS, name)
	if err != nil {
		return nil, "", err
	}
	return data, name, nil
}
