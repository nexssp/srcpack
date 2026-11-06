package config

import (
	"bytes"
	_ "embed"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/nexssp/kernel/xerr"
	"gopkg.in/yaml.v3"
)

//go:embed defaults/default_config.yml
var embeddedDefaultConfig []byte

//go:embed defaults/nexss.srcpack.ignore
var embeddedIgnore []byte

// EmbeddedIgnore returns the shipped content of nexss.srcpack.ignore.
// Used by `srcpack init` to seed the file when one is not present.
func EmbeddedIgnore() []byte { return embeddedIgnore }

// ConfigFileName is the name srcpack looks for when walking up from a
// project root.
const ConfigFileName = "nexss.srcpack.yml"

// ConfigFileNames is the ordered list of names srcpack recognizes as a
// configuration file when looking inside a specific directory. The
// first entry is the canonical name written by `srcpack init`; the
// rest are accepted for compatibility.
var ConfigFileNames = []string{
	"nexss.srcpack.yml",
	"nexss.srcpack.yaml",
	"_nexss.yml",
	"_nexss.yaml",
}

// FindRoot returns the project root for startDir, plus the path of the
// config file found there (empty when the root was determined by .git/
// only).
//
// The project root is the first ancestor of startDir that contains
// either a recognized config file or a .git/ directory. When neither is
// found before the filesystem root, startDir is returned with an empty
// configPath.
//
// This is the single source of truth for "where is the project". Both
// config discovery and default output placement use it, so a run from a
// subdirectory never produces a stray _srcpack/ under the subdirectory.
func FindRoot(startDir string) (root, configPath string) {
	if startDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ".", ""
		}
		startDir = wd
	}

	dir := filepath.Clean(startDir)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for {
		if path := FindInDir(dir); path != "" {
			return dir, path
		}
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir, ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir, ""
		}
		dir = parent
	}
}

// FindInDir returns the config file present in dir, or "" when none is
// found. Unlike FindRoot, this does not walk upward — it only checks
// the given directory.
//
// Used by `srcpack init` to detect "a config already exists HERE",
// which is what decides whether --force is required. The distinction
// matters: init creates a config in the current directory, so a
// walk-up would find the parent project's config and overwrite it.
func FindInDir(dir string) string {
	for _, name := range ConfigFileNames {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// Load returns the effective Config for a run. When no user file is
// found under rootDir the embedded defaults plus built-in profiles are
// returned. When a user file is present it is overlaid on the
// defaults; built-in profile fields are preserved unless the user file
// explicitly overrides them.
//
// explicitPath, when non-empty, bypasses discovery and loads that file
// instead. A missing explicit path is an error, unlike a missing
// discovered path.
func Load(rootDir, explicitPath string) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(embeddedDefaultConfig, &cfg); err != nil {
		return Config{}, xerr.Internal("parse embedded defaults", err)
	}
	initMaps(&cfg)
	registerBuiltinProfiles(&cfg)

	path := explicitPath
	if path == "" {
		_, path = FindRoot(rootDir)
	}
	if path == "" {
		finalizeProfiles(&cfg)
		return cfg, nil
	}

	var user Config
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, xerr.NotFound("read config "+path, err)
		}
		return Config{}, xerr.Internal("read config "+path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&user); err != nil {
		return Config{}, xerr.Validation("parse config "+path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple YAML documents are not supported")
		}
		return Config{}, xerr.Validation("parse config "+path, err)
	}
	mergeUserConfig(&cfg, &user)
	finalizeProfiles(&cfg)
	return cfg, nil
}

// LoadOptional behaves like Load but treats a missing explicit path as
// "no file present" rather than an error. Used by callers that want the
// defaults-plus-discovery behavior only.
func LoadOptional(rootDir string) (Config, error) {
	return Load(rootDir, "")
}

func initMaps(cfg *Config) {
	if cfg.Prompts == nil {
		cfg.Prompts = make(map[string]string)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}
	if cfg.Editors == nil {
		cfg.Editors = make(map[string]EditorFormat)
	}
	if cfg.TargetsMap == nil {
		cfg.TargetsMap = make(map[string]TargetGroup)
	}
	if cfg.Views == nil {
		cfg.Views = make(map[string][]string)
	}
}

// registerBuiltinProfiles seeds every built-in profile into cfg.Profiles
// before any user file is read, so user overlays merge onto the same
// option defaults the profiles ship with.
func registerBuiltinProfiles(cfg *Config) {
	for i := range BuiltinProfiles {
		meta := &BuiltinProfiles[i]
		cfg.Profiles[meta.Name] = Profile{
			Name:        meta.Name,
			Description: meta.Description,
			Prompt:      FlexibleText{Value: meta.PromptKey, Set: meta.PromptKey != ""},
			Options:     meta.Options,
		}
	}
}

// mergeUserConfig overlays a decoded user file onto cfg.
//
// Profiles are merged field-by-field rather than replaced, so an entry
// that only overrides `options.no_tests: false` keeps the rest of the
// built-in profile's defaults. This mirrors the semantics legacy
// shipped with and is why we do not rely on yaml.v3's in-place struct
// merge for map values.
func mergeUserConfig(dst, src *Config) {
	MergeProfile(&dst.Profile, src.Profile)

	maps.Copy(dst.Prompts, src.Prompts)
	maps.Copy(dst.Editors, src.Editors)
	maps.Copy(dst.TargetsMap, src.TargetsMap)
	maps.Copy(dst.Views, src.Views)
	//nolint:gocritic // rangeValCopy: map values are not addressable; Profile is config-time only.
	for key, value := range src.Profiles {
		value.Name = key
		if existing, ok := dst.Profiles[key]; ok {
			MergeProfile(&existing, value)
			dst.Profiles[key] = existing
		} else {
			dst.Profiles[key] = value
		}
	}
	if src.Editor != "" {
		dst.Editor = src.Editor
	}
}

// finalizeProfiles stamps the map key onto each profile's Name field,
// so callers can rely on Profile.Name being populated after Load.
func finalizeProfiles(cfg *Config) {
	for key := range cfg.Profiles {
		p := cfg.Profiles[key]
		p.Name = key
		cfg.Profiles[key] = p
	}
	cfg.Name = profileDefault
}
