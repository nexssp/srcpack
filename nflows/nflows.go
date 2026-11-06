package nflows

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/nexssp/flow/compiler"

	"github.com/nexssp/srcpack/bundles"

	// Register tcli transport actions used by embedded Flow programs.
	_ "github.com/nexssp/transport/tcli/nexssflow"
)

//go:embed *.nflow
var FS embed.FS

// Loader resolves the flat legacy sources and explicitly namespaced command and
// capability bundles. Unknown paths are not flattened.
func Loader() compiler.SourceLoader { return embedLoader{} }

type embedLoader struct{}

func (embedLoader) Load(from, target string) ([]byte, string, error) {
	name := filepath.ToSlash(filepath.Clean(target))
	if !filepath.IsAbs(target) {
		fromName := filepath.ToSlash(filepath.Clean(from))
		name = path.Clean(path.Join(path.Dir(fromName), name))
	}
	// Each command owns its schema, process and Go actions. Resolve only
	// registered namespaces, never arbitrary paths by their basename.
	for _, module := range []struct {
		prefix string
		files  fs.FS
	}{
		{"nflows", FS},
		{"bundles", bundles.FS},
	} {
		pattern := "*.nflow"
		if module.prefix == "bundles" {
			pattern = "*/*.nflow"
		}
		entries, err := fs.Glob(module.files, pattern)
		if err != nil {
			return nil, "", err
		}
		for _, file := range entries {
			identity := module.prefix + "/" + file
			if name == identity || strings.HasSuffix(name, "/"+identity) {
				data, err := fs.ReadFile(module.files, file)
				return data, identity, err
			}
		}
	}
	for _, namespace := range []string{"commands/", "bundles/", "nflows/"} {
		if strings.HasPrefix(name, namespace) {
			return nil, "", fmt.Errorf("unknown embedded NFlow include %q", target)
		}
	}
	// Flat legacy sources are resolved only beside the including source;
	// arbitrary nested paths must not alias an embedded basename.
	if filepath.Dir(target) != "." {
		baseDir := filepath.Dir(from)
		if !filepath.IsAbs(baseDir) {
			var err error
			baseDir, err = filepath.Abs(baseDir)
			if err != nil {
				return nil, "", err
			}
		}
		if filepath.Clean(filepath.Dir(target)) != filepath.Clean(baseDir) {
			return nil, "", fmt.Errorf("unknown embedded NFlow include %q", target)
		}
	}
	name = filepath.Base(target)
	data, err := fs.ReadFile(FS, name)
	if err != nil {
		return nil, "", err
	}
	return data, name, nil
}
