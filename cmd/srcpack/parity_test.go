package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

// TestPackParity compares the direct normalized runPack API against the
// CLI normalization host for the same inputs. Both now execute the root NFlow. Both paths write to
// out.md; compare all bytes except the generated current-time header, since
// sequential runs can cross a second boundary (particularly under -race).
//
// The paths are exercised directly (not through execute) because
// execute routes both through the same root; this guards normalization parity,
// not an independent implementation of the pre-migration pipeline.
func TestPackParity(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	workspace := map[string]string{
		"main.go":      "package main\n\n// Public says hi.\nfunc Public() string { return \"hi\" }\n\nfunc private() {}\n",
		"main_test.go": "package main\n\nfunc TestPublic(t *testing.T) {}\n",
		"lib.go":       "package main\n\nfunc Lib() {}\n",
		"config.yml":   "key: value\n",
		"unicode.txt":  "café — 東京\n",
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "target only-sig",
			args: []string{"pack", "--target", ".", "--only-sig", "--output", "out.md"},
		},
		{
			name: "no-tests",
			args: []string{"pack", "--target", ".", "--no-tests", "--output", "out.md"},
		},
		{
			name: "ext filter",
			args: []string{"pack", "--target", ".", "--ext", "go", "--output", "out.md"},
		},
		{
			name: "UTF-8 text file",
			args: []string{"pack", "--target", ".", "--ext", "txt", "--output", "out.md"},
		},
		{
			name: "dense signatures",
			args: []string{"pack", "--target", ".", "--only-sig", "--dense", "--output", "out.md"},
		},
		{
			name: "mask",
			args: []string{"pack", "--target", ".", "--mask", "--output", "out.md"},
		},
		{
			name: "exclude pattern",
			args: []string{"pack", "--target", ".", "--exclude", "*.yml", "--output", "out.md"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacyOutput := captureLegacyPack(t, workspace, test.args)
			entrypointOutput := captureEntrypointPack(t, workspace, test.args)
			ktest.RequireEqual(t, stage5StableDocument([]byte(entrypointOutput)), stage5StableDocument([]byte(legacyOutput)))
		})
	}
}

func TestPackParity_EmptyWorkspace(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	args := []string{"pack", "--target", ".", "--output", "out.md"}
	legacyOutput := captureLegacyPack(t, map[string]string{}, args)
	entrypointOutput := captureEntrypointPack(t, map[string]string{}, args)
	ktest.RequireEqual(t, stage5StableDocument([]byte(entrypointOutput)), stage5StableDocument([]byte(legacyOutput)))
}

// TestPackParity_OutputPathDivergence documents the one difference the
// new path has today: an empty --output leaves the legacy default in
// place (writes to _srcpack/context.md) while the nflow path treats an
// empty path as pass-through. Fixing the divergence is a separate
// decision; this test locks the current behavior.
func TestPackParity_OutputPathDivergence(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// Clipboard is unrelated to output-path behavior; keep this test headless-safe.
	args := []string{"pack", "--target", ".", "--no-copy"}
	workspace := map[string]string{
		"main.go": "package main\n",
	}

	legacyRoot := materializeWorkspace(t, workspace)
	t.Chdir(legacyRoot)
	legacyArgs, err := parseCLI(args)
	ktest.RequireNoError(t, err)
	ktest.RequireNoError(t, runPack(context.Background(), legacyArgs))
	_, legacyErr := os.Stat(filepath.Join(legacyRoot, "_srcpack", "context.md"))
	ktest.RequireNoError(t, legacyErr)

	entryRoot := materializeWorkspace(t, workspace)
	t.Chdir(entryRoot)
	handled, err := runEntrypoint(context.Background(), args)
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, handled, "runEntrypoint did not handle pack")

	entries, err := os.ReadDir(entryRoot)
	ktest.RequireNoError(t, err)
	for _, e := range entries {
		ktest.RequireCondition(t, e.Name() != "_srcpack",
			"entrypoint unexpectedly created _srcpack/ without --output")
	}
}

func captureLegacyPack(tb testing.TB, workspace map[string]string, args []string) string {
	tb.Helper()

	root := materializeWorkspace(tb, workspace)
	chdirForTest(tb, root)

	cli, err := parseCLI(args)
	ktest.RequireNoError(tb, err)
	ktest.RequireNoError(tb, runPack(context.Background(), cli))

	return readOutputFile(tb)
}

func captureEntrypointPack(tb testing.TB, workspace map[string]string, args []string) string {
	tb.Helper()

	root := materializeWorkspace(tb, workspace)
	chdirForTest(tb, root)

	handled, err := runEntrypoint(context.Background(), args)
	ktest.RequireNoError(tb, err)
	ktest.RequireCondition(tb, handled, "runEntrypoint did not handle %v", args)

	return readOutputFile(tb)
}

func materializeWorkspace(tb testing.TB, files map[string]string) string {
	tb.Helper()

	root := tb.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		ktest.RequireNoError(tb, os.MkdirAll(filepath.Dir(path), 0o755))
		ktest.RequireNoError(tb, os.WriteFile(path, []byte(body), 0o600))
	}
	return root
}

func chdirForTest(tb testing.TB, dir string) {
	tb.Helper()
	type chdirer interface {
		Chdir(string)
	}
	c, ok := tb.(chdirer)
	ktest.RequireCondition(tb, ok, "testing.TB does not support Chdir")
	c.Chdir(dir)
}

func readOutputFile(tb testing.TB) string {
	tb.Helper()
	data, err := os.ReadFile("out.md")
	ktest.RequireNoError(tb, err)
	return string(data)
}
