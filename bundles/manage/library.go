// Package managecmd provides data access and atomic storage primitives for
// Flow-driven srcpack administrative commands. Routing and user-facing output
// are intentionally defined in manage.nflow.
package managecmd

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/config"
	"github.com/nexssp/srcpack/bundles/config_views"
)

const ID = "srcpack_manage"

//go:embed *.nflow
var FS embed.FS

func init() { compiler.Register(ID, Bundle) }

// Bundle exposes only storage/data actions; the NFlow pipelines own all
// command selection and operation semantics.
func Bundle(_ map[string]string) compiler.Bundle {
	libraries := []action.Library{{Name: ID, Actions: []action.AnyAction{
		action.New("manage.load", load).Description("Load normalized workspace configuration data").Build(),
		action.New("manage.view_context", viewContext).Description("Locate raw view storage without decoding unrelated configuration").Build(),
		action.New("manage.target_add", addTarget).Build(),
		action.New("manage.target_delete", deleteTarget).Build(),
		action.New("manage.prompt_add", addPrompt).Build(),
		action.New("manage.prompt_delete", deletePrompt).Build(),
	}}}
	return compiler.Bundle{ID: ID, Libraries: libraries, Fixtures: FS}
}

// Loader serves the module's embedded NFlow source files.
func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

func (sourceLoader) Load(from, target string) ([]byte, string, error) {
	clean := filepath.ToSlash(filepath.Clean(target))
	if clean == "../config_views/config_views.nflow" || strings.HasSuffix(clean, "bundles/config_views/config_views.nflow") || filepath.Base(clean) == "crud.nflow" {
		return config_views.Loader().Load(from, target)
	}
	name := filepath.Base(target)
	if name != "manage.nflow" && name != "manage_reports.nflow" && name != "schema.nflow" {
		return nil, "", fmt.Errorf("srcpack manage: unknown source %q", target)
	}
	data, err := FS.ReadFile(name)
	return data, "srcpack_manage/" + name, err
}

// load returns the merged configuration as plain JSON-compatible data and the
// root config path used by legacy target/prompt/view storage.
func load(_ context.Context, in map[string]any) (map[string]any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cwd, "")
	if err != nil {
		return nil, err
	}
	for name := range cfg.Profiles {
		profile := cfg.Profiles[name]
		profile.Name = name
		cfg.Profiles[name] = profile
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode config data: %w", err)
	}
	var plain map[string]any
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, fmt.Errorf("decode config data: %w", err)
	}
	root, path := config.FindRoot(cwd)
	if path == "" {
		path = filepath.Join(root, config.ConfigFileName)
	}
	promptSnippets := make(map[string]string, len(cfg.Prompts))
	for name, prompt := range cfg.Prompts {
		promptSnippets[name] = config.FormatSnippet(prompt, 60)
	}
	// NFlow receives a stable shape, including empty collections. Preserve the
	// command envelope: loading data must not discard subcommands or arguments.
	for _, key := range []string{"targets", "prompts", "profiles", "editors", "views"} {
		if plain[key] == nil {
			plain[key] = map[string]any{}
		}
	}
	out := make(map[string]any, len(in)+4)
	maps.Copy(out, in)
	out["config"], out["path"] = plain, path
	out["prompt_snippets"] = promptSnippets
	return out, nil
}

// Saved views are a raw-YAML CRUD section. They must remain manageable even
// when an unrelated future setting is unknown to the typed config decoder.
func viewContext(_ context.Context, in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in)+1)
	maps.Copy(out, in)
	out["path"] = configPath()
	return out, nil
}

func addTarget(_ context.Context, in map[string]any) (map[string]any, error) {
	name, err := requiredString(in, "name")
	if err != nil {
		return nil, err
	}
	paths, err := stringSlice(in["paths"], "paths")
	if err != nil {
		return nil, err
	}
	_, err = config.NewConfigStore(configPath()).Mutate(false, func(cfg *config.UserConfig) error {
		if cfg.Targets == nil {
			cfg.Targets = make(map[string]config.TargetGroup)
		}
		if _, exists := cfg.Targets[name]; exists {
			return fmt.Errorf("target %q already exists", name)
		}
		cfg.Targets[name] = config.TargetGroup{Paths: paths}
		return nil
	})
	return in, err
}

func deleteTarget(_ context.Context, in map[string]any) (map[string]any, error) {
	name, err := requiredString(in, "name")
	if err != nil {
		return nil, err
	}
	_, err = config.NewConfigStore(configPath()).Mutate(false, func(cfg *config.UserConfig) error {
		if _, exists := cfg.Targets[name]; !exists {
			return fmt.Errorf("target %q not found", name)
		}
		delete(cfg.Targets, name)
		return nil
	})
	return in, err
}

func addPrompt(_ context.Context, in map[string]any) (map[string]any, error) {
	name, err := requiredString(in, "name")
	if err != nil {
		return nil, err
	}
	text, err := requiredString(in, "text")
	if err != nil {
		return nil, err
	}
	_, err = config.NewConfigStore(configPath()).Mutate(false, func(cfg *config.UserConfig) error {
		if cfg.Prompts == nil {
			cfg.Prompts = make(map[string]string)
		}
		if _, exists := cfg.Prompts[name]; exists {
			return fmt.Errorf("prompt %q already exists", name)
		}
		cfg.Prompts[name] = text
		return nil
	})
	return in, err
}

func deletePrompt(_ context.Context, in map[string]any) (map[string]any, error) {
	name, err := requiredString(in, "name")
	if err != nil {
		return nil, err
	}
	_, err = config.NewConfigStore(configPath()).Mutate(false, func(cfg *config.UserConfig) error {
		if _, exists := cfg.Prompts[name]; !exists {
			return fmt.Errorf("prompt %q not found", name)
		}
		delete(cfg.Prompts, name)
		return nil
	})
	return in, err
}

func configPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	root, path := config.FindRoot(cwd)
	if path != "" {
		return path
	}
	return filepath.Join(root, config.ConfigFileName)
}

func requiredString(in map[string]any, key string) (string, error) {
	value, ok := in[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("manage: %s must be a nonempty string", key)
	}
	return value, nil
}

func stringSlice(value any, key string) ([]string, error) {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...), nil
	case []any:
		result := make([]string, len(values))
		for i, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("manage: %s item %d must be a string", key, i)
			}
			result[i] = text
		}
		return result, nil
	default:
		return nil, fmt.Errorf("manage: %s must be an array", key)
	}
}
