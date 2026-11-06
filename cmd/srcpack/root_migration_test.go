package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestRootMigrationEntrySyntax(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	tests := []struct {
		name string
		args []string
	}{
		{"explicit pack", []string{"pack", "--target", ".", "--output", "out.md"}},
		{"short pack", []string{"p", "--target", ".", "--output", "out.md"}},
		{"flag first", []string{"--target", ".", "--output", "out.md"}},
		{"bare profile", []string{"arch", "--target", ".", "--output", "out.md"}},
		{"selector", []string{"main.go#arch", "--output", "out.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := materializeWorkspace(t, map[string]string{"main.go": "package main\n\nfunc Public() string { return \"hello\" }\n"})
			t.Chdir(root)
			if err := execute(context.Background(), tc.args); err != nil {
				t.Fatalf("execute(%v): %v", tc.args, err)
			}
			data, err := os.ReadFile("out.md")
			ktest.RequireNoError(t, err)
			if !strings.Contains(string(data), "Public") {
				t.Fatalf("output missing selected source: %s", data)
			}
		})
	}
}

func TestRootMigrationTree(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\n\nfunc Public() string { return \"implementation marker\" }\n"})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"tree", "--tree-files", "--output", "tree.md"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("tree.md")
	ktest.RequireNoError(t, err)
	if !strings.Contains(string(data), "main.go") {
		t.Fatalf("tree missing source path: %s", data)
	}
	if strings.Contains(string(data), "implementation marker") || strings.Contains(string(data), "func Public()") {
		t.Fatalf("tree unexpectedly contains function implementation: %s", data)
	}
}

func TestRootMigrationCwd(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc CwdSource() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(parent)
	if err := execute(context.Background(), []string{"--cwd", root, "--target", ".", "--output", "out.md"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "out.md"))
	ktest.RequireNoError(t, err)
	if !strings.Contains(string(data), "CwdSource") {
		t.Fatalf("cwd output missing source: %s", data)
	}
}

func TestRootMigrationCheckPreservesTargetAndCleansTemp(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := materializeWorkspace(t, map[string]string{
		"main.go": "package main\n\nfunc Fresh() {}\n",
		"out.md":  "keep existing target\n",
	})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"pack", "--target", ".", "--output", "out.md", "--check"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("out.md")
	ktest.RequireNoError(t, err)
	if string(data) != "keep existing target\n" {
		t.Fatalf("check modified target: %q", data)
	}
	entries, err := os.ReadDir(root)
	ktest.RequireNoError(t, err)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".srcpack-check-") {
			t.Fatalf("check temp file remains: %s", entry.Name())
		}
	}
}

func TestRootMigrationSaveView(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\n"})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"pack", "main.go", "--save-view", "smoke"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "nexss.srcpack.yml"))
	ktest.RequireNoError(t, err)
	if !strings.Contains(string(data), "smoke") {
		t.Fatalf("saved view missing name: %s", data)
	}
}

func TestRootMigrationStdoutOnly(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc StdoutOnly() {}\n"})
	t.Chdir(root)
	reader, writer, err := os.Pipe()
	ktest.RequireNoError(t, err)
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original; _ = reader.Close(); _ = writer.Close() }()
	err = execute(context.Background(), []string{"p", "-t", ".", "-o", "-", "--no-header", "-q"})
	os.Stdout = original
	_ = writer.Close()
	ktest.RequireNoError(t, err)
	data, err := io.ReadAll(reader)
	ktest.RequireNoError(t, err)
	if strings.Count(string(data), "func StdoutOnly()") != 1 {
		t.Fatalf("expected exactly one stdout document, got %q", data)
	}
	if _, err := os.Stat("-"); !os.IsNotExist(err) {
		t.Fatalf("stdout created a file named -: %v", err)
	}
	if _, err := os.Stat("_srcpack"); !os.IsNotExist(err) {
		t.Fatalf("stdout created default output directory: %v", err)
	}
}
