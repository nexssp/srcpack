// Package document supplies a copyable NFlow document formatter. It depends
// only on Flow and kernel; no source-packaging domain types or I/O effects.
package document

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/placeholder"
)

const ID = "document"

//go:embed document.nflow
var FS embed.FS

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/document/nexssflow", Bundle)
}

// Bundle mounts the generic interpolation primitive, not the NFlow source.
// Include document.nflow separately with Loader or a filesystem source loader.
func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: ID, Libraries: []action.Library{{
		Name: ID,
		Actions: []action.AnyAction{
			action.New("document.interpolate", interpolate).
				Description("Expand literal text parts once and concatenate them into blocks").Build(),
		},
	}}}
}

// Loader resolves the single source file of this standalone bundle. The parent
// application may compose this with its own namespaced source loader.
func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

func (sourceLoader) Load(_, target string) ([]byte, string, error) {
	name := filepath.Base(target)
	data, err := fs.ReadFile(FS, name)
	return data, "document.nflow", err
}

// Keep strings out of JSON coercion: invalid UTF-8 and embedded NUL are valid
// source payloads and must survive the same as any other bytes.
func interpolate(ctx context.Context, input map[string]any) (map[string]any, error) {
	blocks, ok := input["blocks"].([]any)
	if !ok {
		return nil, errors.New("document.interpolate: blocks must be an array")
	}
	out := make([]string, 0, len(blocks))
	for i, rawBlock := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		block, ok := rawBlock.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("document.interpolate: block %d must be an object", i)
		}
		parts, ok := block["parts"].([]any)
		if !ok {
			return nil, fmt.Errorf("document.interpolate: block %d parts must be an array", i)
		}
		var joined strings.Builder
		for j, rawPart := range parts {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			part, ok := rawPart.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("document.interpolate: block %d part %d must be an object", i, j)
			}
			text, ok := part["text"].(string)
			if !ok {
				return nil, fmt.Errorf("document.interpolate: block %d part %d text must be a string", i, j)
			}
			expand, err := optionalBool(part, "expand")
			if err != nil {
				return nil, err
			}
			newline, err := optionalBool(part, "newline")
			if err != nil {
				return nil, err
			}
			if expand {
				values, err := stringMap(part["values"])
				if err != nil {
					return nil, err
				}
				text = expandOnce(text, values)
			}
			joined.WriteString(text)
			if newline && text != "" && !strings.HasSuffix(text, "\n") {
				joined.WriteByte('\n')
			}
		}
		out = append(out, joined.String())
	}
	return map[string]any{"blocks": out}, nil
}

func optionalBool(part map[string]any, name string) (bool, error) {
	value, exists := part[name]
	if !exists {
		return false, nil
	}
	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("document.interpolate: %s must be a bool", name)
	}
	return flag, nil
}

func stringMap(raw any) (map[string]string, error) {
	switch values := raw.(type) {
	case nil:
		return nil, nil
	case map[string]string:
		return values, nil
	case map[string]any:
		out := make(map[string]string, len(values))
		for key, rawValue := range values {
			value, ok := rawValue.(string)
			if !ok {
				return nil, fmt.Errorf("document.interpolate: value %q must be a string", key)
			}
			out[key] = value
		}
		return out, nil
	default:
		return nil, errors.New("document.interpolate: values must be an object of strings")
	}
}

// expandOnce preserves unknown and malformed tokens and never parses inserted
// values as templates. In particular source text cannot become executable DSL.
func expandOnce(source string, values map[string]string) string {
	return placeholder.ExpandOnce(source, func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}
