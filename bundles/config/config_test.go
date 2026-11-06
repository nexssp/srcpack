package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/kernel/xerr"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/config"
)

// ── Load ─────────────────────────────────────────────────────────────

func TestLoad_DefaultsOnly(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadOptional(t.TempDir())
	ktest.RequireNoError(t, err)

	if cfg.Editor != "markdown" {
		t.Errorf("default editor = %q, want markdown", cfg.Editor)
	}
	if cfg.Prompts["default"] == "" {
		t.Error("default prompt missing")
	}
	for _, name := range []string{"arch", "api", "impl", "source", "with-tests", "compact", "dense"} {
		if _, ok := cfg.Profiles[name]; !ok {
			t.Errorf("built-in profile %q missing", name)
		}
	}
}

func TestLoad_UserOverlay(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeYAML(t, root, config.ConfigFileName, `
editor: zed
prompts:
  custom: "custom prompt text"
`)

	cfg, err := config.Load(root, "")
	ktest.RequireNoError(t, err)

	ktest.RequireEqual(t, cfg.Editor, "zed")
	ktest.RequireEqual(t, cfg.Prompts["custom"], "custom prompt text")
	if cfg.Prompts["default"] == "" {
		t.Error("embedded default prompt was replaced instead of merged")
	}
}

func TestLoad_UserOverridesBuiltinProfileField(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeYAML(t, root, config.ConfigFileName, `
profiles:
  arch:
    options:
      no_tests: false
`)

	cfg, err := config.Load(root, "")
	ktest.RequireNoError(t, err)

	arch := cfg.Profiles["arch"]
	if arch.Options.NoTests == nil {
		t.Fatal("arch.options.no_tests = nil, want &false")
	}
	ktest.RequireEqual(t, *arch.Options.NoTests, false)

	// Other built-in fields preserved.
	if arch.Options.ExportedOnly == nil || !*arch.Options.ExportedOnly {
		t.Error("arch.options.exported_only was lost by overlay")
	}
	if arch.Options.RemoveBodyFunc == nil || !*arch.Options.RemoveBodyFunc {
		t.Error("arch.options.remove_body_func was lost by overlay")
	}
}

func TestLoad_UserAddsNewProfile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeYAML(t, root, config.ConfigFileName, `
profiles:
  docs-only:
    description: "Only docs"
    options:
      ext_filter: "md"
`)

	cfg, err := config.Load(root, "")
	ktest.RequireNoError(t, err)

	p, ok := cfg.Profiles["docs-only"]
	if !ok {
		t.Fatal("user profile not registered")
	}
	ktest.RequireEqual(t, p.Name, "docs-only")
	ktest.RequireEqual(t, p.Description, "Only docs")
	if p.Options.ExtFilter == nil || *p.Options.ExtFilter != "md" {
		t.Error("ext_filter not applied")
	}
}

func TestLoad_RejectsUnknownField(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeYAML(t, root, config.ConfigFileName, "typo_field: value\n")

	_, err := config.Load(root, "")
	ktest.RequireErrorKind(t, err, xerr.KindValidation)
}

func TestLoad_ExplicitPathMissing(t *testing.T) {
	t.Parallel()

	_, err := config.Load("", filepath.Join(t.TempDir(), "absent.yml"))
	ktest.RequireErrorKind(t, err, xerr.KindNotFound)
}

func TestFindInDir_OrderedFilesSkipDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range config.ConfigFileNames[:2] {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	want := filepath.Join(dir, config.ConfigFileNames[2])
	writeYAML(t, dir, config.ConfigFileNames[2], "profiles: {}\n")
	writeYAML(t, dir, config.ConfigFileNames[3], "profiles: {}\n")

	ktest.RequireEqual(t, config.FindInDir(dir), want)
}

func TestFindRoot_PrefersNearestConfigBeforeGit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(nested, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeYAML(t, root, config.ConfigFileName, "profiles: {}\n")
	writeYAML(t, nested, config.ConfigFileNames[1], "profiles: {}\n")

	gotRoot, gotPath := config.FindRoot(nested)
	ktest.RequireEqual(t, gotRoot, nested)
	ktest.RequireEqual(t, gotPath, filepath.Join(nested, config.ConfigFileNames[1]))
}

// ── CanonicalProfileName ─────────────────────────────────────────────

func TestCanonicalProfileName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"arch":       "arch",
		"ARCH":       "arch",
		"skeleton":   "arch",
		"api-map":    "arch",
		"docs":       "api",
		"tests":      "with-tests",
		"full":       "source",
		"custom-one": "custom-one",
	}
	for input, want := range cases {
		if got := config.CanonicalProfileName(input); got != want {
			t.Errorf("CanonicalProfileName(%q) = %q, want %q", input, got, want)
		}
	}
}

// ── MergeProfile ─────────────────────────────────────────────────────

func TestMergeProfile_PreservesUnsetFields(t *testing.T) {
	t.Parallel()

	dst := config.Profile{
		Description: "old",
		Editor:      "markdown",
		Output:      "out.md",
		Options: config.ConfigOptions{
			StripComments: new(true),
			ExportedOnly:  new(true),
		},
	}
	src := config.Profile{
		Editor: "zed",
		Options: config.ConfigOptions{
			StripComments: new(false),
		},
	}

	config.MergeProfile(&dst, src)

	ktest.RequireEqual(t, dst.Description, "old")
	ktest.RequireEqual(t, dst.Editor, "zed")
	ktest.RequireEqual(t, dst.Output, "out.md")
	ktest.RequireEqual(t, *dst.Options.StripComments, false)
	ktest.RequireEqual(t, *dst.Options.ExportedOnly, true)
}

// ── DirEntry ─────────────────────────────────────────────────────────

func TestDirEntry_BareStringForm(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeYAML(t, root, config.ConfigFileName, `
targets:
  backend:
    dir:
      - kernel
      - path: transport
        omit:
          - _test.go
`)

	cfg, err := config.Load(root, "")
	ktest.RequireNoError(t, err)

	backend, ok := cfg.TargetsMap["backend"]
	if !ok {
		t.Fatal("target backend missing")
	}
	if len(backend.Dir) != 2 {
		t.Fatalf("dir entries = %d, want 2", len(backend.Dir))
	}
	ktest.RequireEqual(t, backend.Dir[0].Path, "kernel")
	ktest.RequireEqual(t, backend.Dir[1].Path, "transport")
	ktest.RequireEqual(t, backend.Dir[1].Omit, []string{"_test.go"})
}

// ── FlexibleText ─────────────────────────────────────────────────────

func TestFlexibleText_ExplicitFalse(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeYAML(t, root, config.ConfigFileName, `
prompt: false
`)

	cfg, err := config.Load(root, "")
	ktest.RequireNoError(t, err)

	if !cfg.Prompt.Set {
		t.Error("prompt.Set = false, want true")
	}
	if !cfg.Prompt.Disabled {
		t.Error("prompt.Disabled = false, want true")
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func writeYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
