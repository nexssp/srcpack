package operators_test

import (
	"path/filepath"
	"testing"

	"github.com/nexssp/srcpack/bundles/operators"
)

func TestSourceRepositoryBackendPreservesSelectorAndLanguageBehavior(t *testing.T) {
	repo := newGitSourceFixture(t)
	writeFile(t, repo, "keep.go", "package x\n// selected line\n// excluded line\n")
	t.Chdir(repo)

	path := filepath.Join(repo, "keep.go")
	files := collectSource(t, operators.PackConfig{
		Targets:        []string{"keep.go:2"},
		TargetProfiles: map[string]string{path: "source"},
		TargetPrompts:  map[string]string{path: "review selected lines"},
	})
	if len(files) != 1 {
		t.Fatalf("source produced %d files, want one: %#v", len(files), files)
	}
	file := files[0]
	if file.RelPath != "keep.go" || file.Lang != "go" || file.Profile != "source" || file.Prompt != "review selected lines" {
		t.Fatalf("selector metadata changed: %#v", file)
	}
	if got, want := string(file.Content), "// selected line"; got != want {
		t.Fatalf("line selector content = %q, want %q", got, want)
	}
}
