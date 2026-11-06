package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xerr"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/config"
)

// ── ParseProfileRef ──────────────────────────────────────────────────

func TestParseProfileRef_BareName(t *testing.T) {
	t.Parallel()

	path, name := config.ParseProfileRef("arch", "/tmp")
	ktest.RequireEqual(t, path, "")
	ktest.RequireEqual(t, name, "arch")
}

func TestParseProfileRef_ColonDelimitedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "a.yml")
	writeYAML(t, dir, "a.yml", "profiles: {}\n")

	path, name := config.ParseProfileRef(cfgPath+":arch", dir)
	ktest.RequireEqual(t, path, cfgPath)
	ktest.RequireEqual(t, name, "arch")
}

func TestParseProfileRef_HashDelimitedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "a.yml")
	writeYAML(t, dir, "a.yml", "profiles: {}\n")

	path, name := config.ParseProfileRef(cfgPath+"#arch", dir)
	ktest.RequireEqual(t, path, cfgPath)
	ktest.RequireEqual(t, name, "arch")
}

func TestParseProfileRef_DirectoryWithConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, "profiles: {}\n")

	path, name := config.ParseProfileRef(dir+":arch", "")
	ktest.RequireEqual(t, path, filepath.Join(dir, config.ConfigFileName))
	ktest.RequireEqual(t, name, "arch")
}

func TestParseProfileRef_PathSuffixProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, "profiles: {}\n")

	path, name := config.ParseProfileRef(filepath.Join(dir, "arch"), "")
	ktest.RequireEqual(t, path, filepath.Join(dir, config.ConfigFileName))
	ktest.RequireEqual(t, name, "arch")
}

func TestParseProfileRef_SkipsWindowsDriveLetter(t *testing.T) {
	t.Parallel()

	// On any OS, "C:\\foo" must not be split on the drive-letter colon.
	path, name := config.ParseProfileRef(`C:\foo\arch`, "")
	ktest.RequireEqual(t, path, "")
	ktest.RequireEqual(t, name, `C:\foo\arch`)
}

func TestParseProfileRef_UnresolvablePrefixFallsThrough(t *testing.T) {
	t.Parallel()

	// "no-such-dir" does not resolve; the whole ref becomes the profile
	// name so the caller can report "profile not found" cleanly instead
	// of silently using a built-in profile.
	path, name := config.ParseProfileRef("no-such-dir:arch", t.TempDir())
	ktest.RequireEqual(t, path, "")
	ktest.RequireEqual(t, name, "no-such-dir:arch")
}

// ── ResolveProfileChain ──────────────────────────────────────────────

func TestResolveChain_SingleProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, "profiles: {}\n")

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	chain, err := config.ResolveProfileChain("arch", cfg, dir)
	ktest.RequireNoError(t, err)
	if len(chain) != 1 {
		t.Fatalf("chain length = %d, want 1", len(chain))
	}
	ktest.RequireEqual(t, chain[0].Name, "arch")
	ktest.RequireEqual(t, chain[0].BaseDir, dir)
}

func TestResolveChain_AliasResolves(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, "profiles: {}\n")

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	chain, err := config.ResolveProfileChain("skeleton", cfg, dir)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, chain[0].Name, "arch")
}

func TestResolveChain_IncludesRunFirst(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, `
profiles:
  base:
    options:
      no_tests: true
  layered:
    include_profile: base
    options:
      no_tests: false
`)

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	chain, err := config.ResolveProfileChain("layered", cfg, dir)
	ktest.RequireNoError(t, err)
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2", len(chain))
	}
	ktest.RequireEqual(t, chain[0].Name, "base")
	ktest.RequireEqual(t, chain[1].Name, "layered")

	opts := config.ResolveOptions(config.Options{}, chain)
	ktest.RequireEqual(t, opts.NoTests, false)
}

func TestResolveChain_DiamondIncludes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, `
profiles:
  common:
    options: { strip_comments: true }
  left:
    include_profile: common
  right:
    include_profile: common
  top:
    include_profiles: [left, right]
`)

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	chain, err := config.ResolveProfileChain("top", cfg, dir)
	ktest.RequireNoError(t, err)

	names := make([]string, len(chain))
	for i, p := range chain {
		names[i] = p.Name
	}
	want := []string{"common", "left", "common", "right", "top"}
	ktest.RequireEqual(t, names, want)
}

func TestResolveChain_DetectsCycle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, `
profiles:
  a:
    include_profile: b
  b:
    include_profile: a
`)

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	_, err = config.ResolveProfileChain("a", cfg, dir)
	ktest.RequireErrorKind(t, err, xerr.KindConflict)
	ktest.RequireStringContains(t, err.Error(), "cyclic profile dependency")
}

func TestResolveChain_MissingProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, "profiles: {}\n")

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	_, err = config.ResolveProfileChain("does-not-exist", cfg, dir)
	ktest.RequireErrorKind(t, err, xerr.KindNotFound)
	ktest.RequireStringContains(t, err.Error(), "does-not-exist")
}

func TestResolveChain_DefaultFallsBackToTopLevel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, `
editor: zed
options:
  no_tests: true
`)

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	chain, err := config.ResolveProfileChain("default", cfg, dir)
	ktest.RequireNoError(t, err)
	if len(chain) != 1 {
		t.Fatalf("chain length = %d, want 1", len(chain))
	}
	ktest.RequireEqual(t, chain[0].Name, "default")
	ktest.RequireEqual(t, chain[0].Editor, "zed")
	if chain[0].Options.NoTests == nil || !*chain[0].Options.NoTests {
		t.Error("top-level options.no_tests not surfaced through default profile")
	}
}

func TestResolveChain_ExternalConfigViaColon(t *testing.T) {
	t.Parallel()

	primaryDir := t.TempDir()
	externalDir := t.TempDir()

	writeYAML(t, primaryDir, config.ConfigFileName, "profiles: {}\n")
	writeYAML(t, externalDir, config.ConfigFileName, `
profiles:
  backend:
    options: { ext_filter: "go" }
`)

	cfg, err := config.Load(primaryDir, "")
	ktest.RequireNoError(t, err)

	ref := externalDir + ":backend"
	chain, err := config.ResolveProfileChain(ref, cfg, primaryDir)
	ktest.RequireNoError(t, err)
	if len(chain) != 1 {
		t.Fatalf("chain length = %d, want 1", len(chain))
	}
	ktest.RequireEqual(t, chain[0].Name, "backend")
	ktest.RequireEqual(t, chain[0].BaseDir, externalDir)
	if chain[0].Options.ExtFilter == nil || *chain[0].Options.ExtFilter != "go" {
		t.Error("external profile options not loaded")
	}
}

func TestResolveChain_ExternalIncludesStayInExternalConfig(t *testing.T) {
	t.Parallel()

	primaryDir := t.TempDir()
	externalDir := t.TempDir()

	writeYAML(t, primaryDir, config.ConfigFileName, "profiles: {}\n")
	writeYAML(t, externalDir, config.ConfigFileName, `
profiles:
  base:
    options: { strip_comments: true }
  layered:
    include_profile: base
    options: { no_tests: true }
`)

	cfg, err := config.Load(primaryDir, "")
	ktest.RequireNoError(t, err)

	chain, err := config.ResolveProfileChain(externalDir+":layered", cfg, primaryDir)
	ktest.RequireNoError(t, err)
	if len(chain) != 2 {
		t.Fatalf("chain length = %d, want 2", len(chain))
	}
	ktest.RequireEqual(t, chain[0].Name, "base")
	ktest.RequireEqual(t, chain[1].Name, "layered")
	ktest.RequireEqual(t, chain[0].BaseDir, externalDir)
	ktest.RequireEqual(t, chain[1].BaseDir, externalDir)
}

// ── ResolveOptions ───────────────────────────────────────────────────

func TestResolveOptions_AppliesInOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, `
profiles:
  a: { options: { strip_comments: true,  no_tests: true  } }
  b: { options: { strip_comments: false, no_deps:  true  } }
  c: { options: { no_tests: false                      } }
`)

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	chain, err := config.ResolveProfileChain("a", cfg, dir)
	ktest.RequireNoError(t, err)
	_ = chain

	// Build a synthetic chain to check ordering semantics directly.
	chainAB := []config.Profile{
		cfg.Profiles["a"],
		cfg.Profiles["b"],
		cfg.Profiles["c"],
	}
	opts := config.ResolveOptions(config.Options{}, chainAB)

	ktest.RequireEqual(t, opts.StripComments, false) // b overrode a
	ktest.RequireEqual(t, opts.NoTests, false)       // c overrode a
	ktest.RequireEqual(t, opts.NoDeps, true)         // only b set it
}

// ── SortedProfileNames ───────────────────────────────────────────────

func TestSortedProfileNames_StableOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeYAML(t, dir, config.ConfigFileName, "profiles: {}\n")

	cfg, err := config.Load(dir, "")
	ktest.RequireNoError(t, err)

	names := config.SortedProfileNames(cfg)
	if len(names) < 7 {
		t.Fatalf("expected at least 7 built-in profiles, got %d", len(names))
	}
	if !sortedStrings(names) {
		t.Fatalf("names not sorted: %v", names)
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func sortedStrings(values []string) bool {
	for i := 1; i < len(values); i++ {
		if strings.Compare(values[i-1], values[i]) > 0 {
			return false
		}
	}
	return true
}
