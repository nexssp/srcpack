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

//go:embed cli_invocation.nflow common.nflow output.nflow pack_fold.nflow render.nflow srcpack.nflow
var FS embed.FS

type embeddedSource struct {
	files fs.FS
	file  string
}

// embeddedSources is indexed once so resolving each include does not repeat fs.Glob.
var embeddedSources = indexEmbeddedSources()

func indexEmbeddedSources() map[string]embeddedSource {
	sources := make(map[string]embeddedSource)
	for _, module := range []struct {
		prefix  string
		files   fs.FS
		pattern string
	}{
		{prefix: "nflows", files: FS, pattern: "*.nflow"},
		{prefix: "bundles", files: bundles.FS, pattern: "*/*.nflow"},
	} {
		entries, err := fs.Glob(module.files, module.pattern)
		if err != nil {
			// These fixed patterns are valid for embed.FS; failure indicates a programming error.
			panic(fmt.Errorf("index embedded NFlow sources for %s: %w", module.prefix, err))
		}
		for _, file := range entries {
			identity := module.prefix + "/" + file
			sources[identity] = embeddedSource{files: module.files, file: file}
		}
	}
	return sources
}

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
	for _, prefix := range []string{"nflows", "bundles"} {
		marker := prefix + "/"
		index := strings.LastIndex(name, marker)
		if index < 0 {
			continue
		}
		identity := name[index:]
		if source, ok := embeddedSources[identity]; ok {
			data, err := fs.ReadFile(source.files, source.file)
			return data, identity, err
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
