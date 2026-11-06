package operators_test

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/operators"
)

func TestReusableGitActionsThroughNFlow(t *testing.T) {
	repo := newGitSourceFixture(t)
	newFile := gitSourceFilename("a\nnew.go")
	writeFile(t, repo, "ignored-tracked.go", "package x\n// tracked change\n")
	writeFile(t, repo, "z-untracked.go", "package x\n")
	writeFile(t, repo, newFile, "package x\n")
	// Mount the Git actions alone: no srcpack source, profiles or exclusions.
	bundles := append(native.Bundles(), compiler.Bundle{
		ID:        "git-actions-test",
		Libraries: []action.Library{{Name: "git", Actions: operators.GitActions()}},
	})
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"root": repo}
	for range 3 {
		result, err := runner.Execute(context.Background(), cfg,
			`{ root: .root, filter: { modified: true } } -> git.changed_paths`, "git-action.nflow", input)
		if err != nil {
			t.Fatal(err)
		}
		paths, ok := result.Output.([]string)
		want := []string{filepath.Join(repo, newFile), filepath.Join(repo, "ignored-tracked.go"), filepath.Join(repo, "z-untracked.go")}
		slices.Sort(want)
		if !ok || !reflect.DeepEqual(paths, want) {
			t.Fatalf("Git action must preserve NUL-delimited paths, stable ordering, and no domain filters: %#v, want %#v", result.Output, want)
		}
	}
	result, err := runner.Execute(context.Background(), cfg, `git.ls_files`, "git-ls-files.nflow", input)
	if err != nil {
		t.Fatal(err)
	}
	paths, ok := result.Output.([]string)
	if !ok || len(paths) != 2 {
		t.Fatalf("git.ls_files must return only the two untracked, non-ignored paths: %#v", result.Output)
	}
}

func TestGitModifiedInUnbornRepository(t *testing.T) {
	repo := t.TempDir()
	gitSourceCommand(t, repo, "init", "-q")
	writeFile(t, repo, "new.go", "package x\n")
	t.Chdir(repo)
	files := collectSource(t, operators.PackConfig{Targets: []string{"."}, Modified: true})
	if got := includedRelPaths(files); !reflect.DeepEqual(got, []string{"new.go"}) {
		t.Fatalf("unborn HEAD fallback = %#v", got)
	}
}
