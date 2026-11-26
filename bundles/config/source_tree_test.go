package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nexssp/srcpack/bundles/config"
)

func TestBuildSourceTreeProfile_ResolvesAliasesAndExtensions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSourceTreeFile(t, root, "main.go", "package main\n")
	writeSourceTreeFile(t, root, "web/app.TSX", "export {}\n")
	writeSourceTreeFile(t, root, "web/bundle.mjs", "export {}\n")
	writeSourceTreeFile(t, root, "web/readme.md", "notes\n")
	writeSourceTreeFile(t, root, "vendor/ignored.tsx", "ignored\n")
	writeSourceTreeFile(t, root, ".private/secret.go", "ignored\n")

	got, err := config.BuildSourceTreeProfile(config.SourceTreeProfileOptions{
		Root:            root,
		ProfileName:     "service",
		Types:           []string{" TypeScript, .GO ", "go"},
		TypeDefinitions: map[string][]string{"TypeScript": {"ts", "tsx"}},
		Extensions:      ".mjs, TSX, .CUSTOM",
	})
	if err != nil {
		t.Fatalf("BuildSourceTreeProfile() error = %v", err)
	}
	profile, ok := got.Profiles["service"]
	if !ok {
		t.Fatal("profile key service missing")
	}
	if profile.Name != "service" {
		t.Errorf("profile name = %q, want service", profile.Name)
	}
	if profile.BaseDir != root {
		t.Errorf("profile base dir = %q, want %q", profile.BaseDir, root)
	}
	if profile.Options.ExtFilter == nil || *profile.Options.ExtFilter != "custom,go,mjs,ts,tsx" {
		t.Errorf("ext_filter = %v, want custom,go,mjs,ts,tsx", profile.Options.ExtFilter)
	}
	if profile.Options.RemoveBodyFunc == nil || *profile.Options.RemoveBodyFunc {
		t.Errorf("remove_body_func = %v, want explicit false", profile.Options.RemoveBodyFunc)
	}
	if profile.Options.ExportedOnly == nil || *profile.Options.ExportedOnly {
		t.Errorf("exported_only = %v, want explicit false", profile.Options.ExportedOnly)
	}
	if gotDirs := sourceTreeDirPaths(profile.Dir); !reflect.DeepEqual(gotDirs, []string{".", "web"}) {
		t.Errorf("profile dirs = %v, want [., web]", gotDirs)
	}
}

func TestBuildSourceTreeProfile_ExcludesAndCollapsesDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, path := range []string{
		"main.go",
		"cmd/main.go",
		"cmd/server/deep/server.go",
		"cmd/tools/tool.go",
		"pkg/main.go",
		"pkg/nested/file.go",
		"private/file.go",
		"cmd/private/secret.go",
		"vendor/dependency.go",
		"node_modules/pkg/index.js",
		"dist/output.js",
		".git/config",
	} {
		writeSourceTreeFile(t, root, path, "source\n")
	}

	got, err := config.BuildSourceTreeProfile(config.SourceTreeProfileOptions{
		Root:        root,
		ProfileName: "app",
		ExcludeDirs: []string{"private", "cmd/private"},
	})
	if err != nil {
		t.Fatalf("BuildSourceTreeProfile() error = %v", err)
	}
	profile := got.Profiles["app"]
	if gotDirs := sourceTreeDirPaths(profile.Dir); !reflect.DeepEqual(gotDirs, []string{".", "cmd", "pkg"}) {
		t.Errorf("profile dirs = %v, want [., cmd, pkg]", gotDirs)
	}
	if profile.Options.ExtFilter != nil {
		t.Errorf("all-types profile ext_filter = %q, want nil", *profile.Options.ExtFilter)
	}
}

func TestBuildSourceTreeProfile_ReturnsSrcpackConfigModel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSourceTreeFile(t, root, "src/main.rs", "fn main() {}\n")
	options := config.SourceTreeProfileOptions{
		Root:            root,
		ProfileName:     "rust-service",
		Types:           []string{"rust"},
		TypeDefinitions: map[string][]string{"rust": {"rs"}},
	}

	first, err := config.BuildSourceTreeProfile(options)
	if err != nil {
		t.Fatalf("first BuildSourceTreeProfile() error = %v", err)
	}
	second, err := config.BuildSourceTreeProfile(options)
	if err != nil {
		t.Fatalf("second BuildSourceTreeProfile() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated builds differ:\nfirst: %#v\nsecond: %#v", first, second)
	}
	firstYAML, err := yaml.Marshal(first)
	if err != nil {
		t.Fatalf("marshal Config: %v", err)
	}
	secondYAML, err := yaml.Marshal(second)
	if err != nil {
		t.Fatalf("marshal Config: %v", err)
	}
	if string(firstYAML) != string(secondYAML) {
		t.Fatalf("repeated YAML differs:\n%s\n---\n%s", firstYAML, secondYAML)
	}

	var decoded config.Config
	if err := yaml.Unmarshal(firstYAML, &decoded); err != nil {
		t.Fatalf("unmarshal into srcpack Config: %v", err)
	}
	profile, ok := decoded.Profiles["rust-service"]
	if !ok {
		t.Fatal("decoded srcpack Config lacks generated profile")
	}
	if len(profile.Dir) != 1 || profile.Dir[0].Path != "src" {
		t.Errorf("decoded Profile.Dir = %#v, want src", profile.Dir)
	}
	if profile.Options.ExtFilter == nil || *profile.Options.ExtFilter != "rs" {
		t.Errorf("decoded Profile.Options.ExtFilter = %v, want rs", profile.Options.ExtFilter)
	}
	if profile.Options.RemoveBodyFunc == nil || *profile.Options.RemoveBodyFunc {
		t.Errorf("decoded remove_body_func = %v, want explicit false", profile.Options.RemoveBodyFunc)
	}
	if profile.Options.ExportedOnly == nil || *profile.Options.ExportedOnly {
		t.Errorf("decoded exported_only = %v, want explicit false", profile.Options.ExportedOnly)
	}
}

func TestBuildSourceTreeProfile_RejectsMissingOrNonDirectoryRoot(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file.go")
	if err := os.WriteFile(file, []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{filepath.Join(t.TempDir(), "missing"), file} {
		if _, err := config.BuildSourceTreeProfile(config.SourceTreeProfileOptions{Root: root}); err == nil {
			t.Errorf("BuildSourceTreeProfile(root=%q) error = nil, want error", root)
		}
	}
}

func TestBuildSourceTreeProfile_RejectsEscapingExclusion(t *testing.T) {
	t.Parallel()
	_, err := config.BuildSourceTreeProfile(config.SourceTreeProfileOptions{
		Root:        t.TempDir(),
		ExcludeDirs: []string{"../outside"},
	})
	if err == nil {
		t.Fatal("BuildSourceTreeProfile() error = nil, want escaping exclusion error")
	}
}

func writeSourceTreeFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sourceTreeDirPaths(entries []config.DirEntry) []string {
	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
	}
	return paths
}
