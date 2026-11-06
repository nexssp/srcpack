package operators_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/bundles/operators"
)

func TestSourceRemoteLabelsAndExclusions(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "compiler/main.go", "package compiler\nfunc Main() {}\n")
	writeFile(t, repo, "compiler/main_test.go", "package compiler\n")
	writeFile(t, repo, "compiler/omit.go", "package compiler\n")
	writeFile(t, repo, "other.go", "package other\n")
	t.Chdir(t.TempDir())
	target := filepath.Join(repo, "compiler")
	label := "github/nexssp/flow@" + strings.Repeat("b", 40)
	files := collectSource(t, operators.PackConfig{
		Targets:        []string{target},
		TargetRoots:    map[string]string{target: repo},
		TargetLabels:   map[string]string{target: label},
		TargetProfiles: map[string]string{target: "source"},
		TargetPrompts:  map[string]string{target: "explain"},
		TargetExcludes: map[string]string{target: "compiler/omit.go"},
	})
	if len(files) != 2 {
		t.Fatalf("got %d files: %#v", len(files), files)
	}
	for _, file := range files {
		if !strings.HasPrefix(file.RelPath, label+"/compiler/") || file.Profile != "source" || file.Prompt != "explain" {
			t.Fatalf("metadata lost: %#v", file)
		}
		if strings.Contains(file.RelPath, "omit.go") || strings.Contains(file.RelPath, repo) {
			t.Fatalf("excluded or unstable path: %s", file.RelPath)
		}
	}
}

func TestSourceRemoteFileExclusion(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "compiler/main.go", "package compiler\n")
	t.Chdir(t.TempDir())
	target := filepath.Join(repo, "compiler", "main.go")
	files := collectSource(t, operators.PackConfig{Targets: []string{target}, TargetRoots: map[string]string{target: repo}, TargetLabels: map[string]string{target: "github/nexssp/flow@commit"}, TargetExcludes: map[string]string{target: "compiler/main.go"}})
	if len(files) != 0 {
		t.Fatalf("explicit remote file exclusion ignored: %#v", files)
	}
}

func TestSourceRemoteLiteralColonFilename(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("colon filenames are not valid on Windows")
	}
	repo := t.TempDir()
	writeFile(t, repo, "literal:10", "literal content\n")
	t.Chdir(t.TempDir())
	target := filepath.Join(repo, "literal:10")
	files := collectSource(t, operators.PackConfig{Targets: []string{target}, TargetRoots: map[string]string{target: repo}, TargetLabels: map[string]string{target: "github/owner/repo@commit"}})
	if len(files) != 1 || !strings.HasSuffix(files[0].RelPath, "/literal:10") {
		t.Fatalf("remote filename reinterpreted as line range: %#v", files)
	}
}
