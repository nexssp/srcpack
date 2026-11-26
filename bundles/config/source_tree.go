package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SourceTreeProfileOptions describes a source tree to turn into a srcpack
// profile. Types are comma-separated type names or extensions; TypeDefinitions
// maps type names to extensions. Unknown type names are treated as extensions.
// Extensions adds comma-separated raw extensions. ExcludeDirs adds directory
// names or root-relative directory paths to the built-in exclusions.
type SourceTreeProfileOptions struct {
	Root            string
	ProfileName     string
	Types           []string
	TypeDefinitions map[string][]string
	Extensions      string
	ExcludeDirs     []string
}

var defaultSourceTreeExcludedDirs = map[string]struct{}{
	"vendor":       {},
	"node_modules": {},
	".svelte-kit":  {},
	"dist":         {},
	"build":        {},
	"workspaces":   {},
}

// BuildSourceTreeProfile discovers matching files under Root and returns a
// Config containing a typed profile. Directory selectors are relative to
// Root, sorted, and collapsed so a selected parent replaces its descendants.
// Hidden directories and common generated/dependency directories are skipped.
// No YAML encoding is performed; callers can compose or marshal the Config.
func BuildSourceTreeProfile(options SourceTreeProfileOptions) (Config, error) {
	root := options.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Config{}, fmt.Errorf("resolve source tree root %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Config{}, fmt.Errorf("stat source tree root %q: %w", absRoot, err)
	}
	if !info.IsDir() {
		return Config{}, fmt.Errorf("source tree root %q is not a directory", absRoot)
	}

	profileName := strings.TrimSpace(options.ProfileName)
	if profileName == "" {
		profileName = strings.ReplaceAll(filepath.Base(absRoot), "-", "_")
	}
	if profileName == "" || profileName == "." || profileName == string(filepath.Separator) {
		return Config{}, errors.New("source tree profile name is empty")
	}

	exclusions, err := normalizeSourceTreeExclusions(options.ExcludeDirs)
	if err != nil {
		return Config{}, err
	}
	extensions := resolveSourceTreeExtensions(options.Types, options.TypeDefinitions, options.Extensions)
	selectedDirs, err := discoverSourceTreeDirs(absRoot, extensions, exclusions)
	if err != nil {
		return Config{}, err
	}

	dirs := make([]string, 0, len(selectedDirs))
	for dir := range selectedDirs {
		if dir != "." {
			dirs = append(dirs, dir)
		}
	}
	dirs = collapseSourceTreeDirs(dirs)
	if _, ok := selectedDirs["."]; ok {
		dirs = append([]string{"."}, dirs...)
	}

	projectName := filepath.Base(absRoot)
	profile := Profile{
		Name:        profileName,
		BaseDir:     absRoot,
		Description: "Auto-generated context profile for " + projectName,
		Options: ConfigOptions{
			RemoveBodyFunc: new(false),
			ExportedOnly:   new(false),
		},
		Dir: make([]DirEntry, 0, len(dirs)),
	}
	for _, dir := range dirs {
		profile.Dir = append(profile.Dir, DirEntry{Path: dir})
	}
	if len(extensions) > 0 {
		sortedExtensions := make([]string, 0, len(extensions))
		for ext := range extensions {
			sortedExtensions = append(sortedExtensions, ext)
		}
		sort.Strings(sortedExtensions)
		profile.Options.ExtFilter = new(strings.Join(sortedExtensions, ","))
	}

	return Config{Profiles: map[string]Profile{profileName: profile}}, nil
}

func discoverSourceTreeDirs(root string, extensions, exclusions map[string]struct{}) (map[string]struct{}, error) {
	selectedDirs := make(map[string]struct{})
	allExtensions := len(extensions) == 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("relative path for %q: %w", path, relErr)
		}
		if entry.IsDir() {
			if relPath != "." && excludedSourceTreeDir(entry.Name(), filepath.ToSlash(relPath), exclusions) {
				return filepath.SkipDir
			}
			return nil
		}

		if !allExtensions {
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(entry.Name())), ".")
			if _, ok := extensions[ext]; !ok {
				return nil
			}
		}
		dir := filepath.ToSlash(filepath.Dir(relPath))
		if dir == "" {
			dir = "."
		}
		selectedDirs[dir] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk source tree %q: %w", root, err)
	}
	return selectedDirs, nil
}

func resolveSourceTreeExtensions(types []string, definitions map[string][]string, explicit string) map[string]struct{} {
	resolved := make(map[string]struct{})
	add := func(raw string) {
		ext := strings.ToLower(strings.TrimSpace(raw))
		ext = strings.TrimPrefix(ext, ".")
		if ext != "" {
			resolved[ext] = struct{}{}
		}
	}
	normalizedDefinitions := make(map[string][]string, len(definitions))
	for alias, extensions := range definitions {
		name := strings.ToLower(strings.TrimSpace(alias))
		if name != "" {
			normalizedDefinitions[name] = append(normalizedDefinitions[name], extensions...)
		}
	}
	for _, rawType := range types {
		for token := range strings.SplitSeq(rawType, ",") {
			token = strings.ToLower(strings.TrimSpace(token))
			if token == "" {
				continue
			}
			if exts, ok := normalizedDefinitions[token]; ok {
				for _, ext := range exts {
					add(ext)
				}
				continue
			}
			add(token)
		}
	}
	for ext := range strings.SplitSeq(explicit, ",") {
		add(ext)
	}
	return resolved
}

func normalizeSourceTreeExclusions(exclusions []string) (map[string]struct{}, error) {
	normalized := make(map[string]struct{}, len(exclusions))
	for _, raw := range exclusions {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		path := filepath.Clean(filepath.FromSlash(raw))
		if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("source tree exclusion %q must be relative to the root", raw)
		}
		if path == "." {
			return nil, fmt.Errorf("source tree exclusion %q cannot exclude the root", raw)
		}
		normalized[filepath.ToSlash(path)] = struct{}{}
	}
	return normalized, nil
}

func excludedSourceTreeDir(name, relPath string, exclusions map[string]struct{}) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if _, ok := defaultSourceTreeExcludedDirs[name]; ok {
		return true
	}
	if _, ok := exclusions[name]; ok {
		return true
	}
	_, ok := exclusions[relPath]
	return ok
}

func collapseSourceTreeDirs(dirs []string) []string {
	set := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		dir = filepath.ToSlash(filepath.Clean(dir))
		if dir != "" && dir != "." {
			set[dir] = struct{}{}
		}
	}
	cleaned := make([]string, 0, len(set))
	for dir := range set {
		cleaned = append(cleaned, dir)
	}
	sort.Strings(cleaned)

	collapsed := make([]string, 0, len(cleaned))
	for _, dir := range cleaned {
		subdir := false
		for _, parent := range collapsed {
			if strings.HasPrefix(dir, parent+"/") {
				subdir = true
				break
			}
		}
		if !subdir {
			collapsed = append(collapsed, dir)
		}
	}
	return collapsed
}
