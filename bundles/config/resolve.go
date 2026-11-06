package config

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/nexssp/kernel/xerr"
)

// ResolveProfileChain resolves ref to a list of profiles whose option
// overlays are applied in order — earliest first, latest last — to
// produce the effective options for a run. The last profile in the
// chain is the target profile; profiles before it are its transitive
// includes.
//
// ref may be a bare profile name, "path:profile", "path#profile", or
// "path/to/dir/profile" when the directory contains a config file. When
// ref names an external config, that file is loaded and its profiles
// are used; otherwise cfg is used.
//
// Cycles in the include graph are reported as xerr.Conflict. Missing
// profiles are reported as xerr.NotFound with the available names.
func ResolveProfileChain(ref string, cfg Config, currentDir string) ([]Profile, error) {
	if strings.TrimSpace(ref) == "" {
		ref = profileDefault
	}
	return resolveChain(ref, cfg, currentDir, nil)
}

func resolveChain(ref string, currentConfig Config, currentDir string, stack []string) ([]Profile, error) {
	extPath, profileName := ParseProfileRef(ref, currentDir)

	targetConfig := currentConfig
	targetDir := currentDir

	if extPath != "" {
		loaded, err := Load(filepath.Dir(extPath), extPath)
		if err != nil {
			return nil, err
		}
		targetConfig = loaded
		targetDir = filepath.Dir(extPath)
	}

	profile, canonical, ok := findProfileInConfig(targetConfig, profileName)
	if !ok {
		names := SortedProfileNames(targetConfig)
		return nil, xerr.NotFound(
			"profile " + profileName + " not found (available: " + strings.Join(names, ", ") + ")")
	}

	key := filepath.Join(targetDir, canonical)
	if slices.Contains(stack, key) {
		cycle := append(append([]string{}, stack...), key)
		return nil, xerr.Conflict("cyclic profile dependency: " + strings.Join(cycle, " -> "))
	}
	nextStack := append(slices.Clone(stack), key)

	var chain []Profile
	var usedIncludes []string
	for _, includeRef := range profile.GetIncludeProfiles() {
		sub, err := resolveChain(includeRef, targetConfig, targetDir, nextStack)
		if err != nil {
			return nil, err
		}
		chain = append(chain, sub...)
		usedIncludes = append(usedIncludes, includeRef)
	}

	profile.Name = canonical
	profile.BaseDir = targetDir
	profile.IncludedProfiles = usedIncludes
	return append(chain, profile), nil
}

// ParseProfileRef splits a selector into an optional external config
// file and a profile name. When ref does not name a config file, the
// returned path is empty and the whole ref is the profile name.
//
// Supported forms:
//
//	"path/to/file.yml:profile"
//	"path/to/file.yml#profile"
//	"path/to/dir:profile"        (dir must contain a config file)
//	"path/to/dir#profile"
//	"path/to/dir/profile"        (dir must contain a config file)
//	"profile"                    (bare profile name)
func ParseProfileRef(ref, currentDir string) (configPath, profileName string) {
	ref = strings.TrimSpace(ref)

	for _, delim := range []string{":", "#"} {
		idx := strings.LastIndex(ref, delim)
		if idx <= 0 {
			continue
		}
		// Windows drive letter: C:\... or C:/... — not a profile delimiter.
		if delim == ":" && idx == 1 && len(ref) > 2 &&
			(ref[2] == '\\' || ref[2] == '/') {
			continue
		}
		path := resolveConfigFile(ref[:idx], currentDir)
		if path == "" {
			continue
		}
		return path, ref[idx+1:]
	}

	if strings.ContainsAny(ref, `/\`) {
		dir := filepath.Dir(ref)
		if path := resolveConfigFile(dir, currentDir); path != "" {
			return path, filepath.Base(ref)
		}
	}

	return "", ref
}

// resolveConfigFile returns the config file at target, or the first
// recognized config file inside target when target is a directory. An
// empty return means no config file was found.
func resolveConfigFile(target, currentDir string) string {
	if !filepath.IsAbs(target) && currentDir != "" {
		target = filepath.Join(currentDir, target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return ""
	}
	info, err := os.Stat(abs)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		return abs
	}
	return FindInDir(abs)
}

// SortedProfileNames returns every profile name in cfg.Profiles in
// stable order. Used for error messages and CLI listings.
func SortedProfileNames(cfg Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveOptions applies every profile's option overlay to base in
// chain order — earliest first, latest last — and returns the result.
// Callers pass cfg.Options.ToOptions() as base.
func ResolveOptions(base Options, chain []Profile) Options {
	for i := range chain {
		base.Apply(chain[i].Options.Mutator())
	}
	return base
}

// findProfileInConfig resolves a profile name against cfg. It tries the
// exact name first, then the canonical built-in alias, then the
// top-level embedded profile for "default".
func findProfileInConfig(cfg Config, name string) (Profile, string, bool) {
	if p, ok := cfg.Profiles[name]; ok {
		p.Name = name
		return p, name, true
	}
	if canonical := CanonicalProfileName(name); canonical != name {
		if p, ok := cfg.Profiles[canonical]; ok {
			p.Name = canonical
			return p, canonical, true
		}
	}
	if name == "default" {
		p := cfg.Profile
		p.Name = "default"
		return p, "default", true
	}
	return Profile{}, "", false
}
