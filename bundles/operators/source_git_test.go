package operators_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/bundles/operators"
)

func TestGitSourceChangedPathsMatrix(t *testing.T) {
	repo := newGitSourceFixture(t)
	lineBreakPath := gitSourceFilename("line\nbreak.go")
	untrackedNamePath := gitSourceFilename("untracked\nname.go")
	writeFile(t, repo, "modified.go", "package x\n// modified\n")
	writeFile(t, repo, "staged.go", "package x\n// staged\n")
	gitSourceCommand(t, repo, "add", "staged.go")
	if err := os.Remove(filepath.Join(repo, "deleted.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, "ignored-tracked.go", "package x\n// ignored changed\n")
	writeFile(t, repo, "srcpack-ignored.go", "package x\n// srcpack ignored changed\n")
	writeFile(t, repo, "cli-excluded.go", "package x\n// cli excluded changed\n")
	writeFile(t, repo, "target-excluded.go", "package x\n// target excluded changed\n")
	writeFile(t, repo, lineBreakPath, "package x\n// special filename changed\n")
	writeFile(t, repo, "untracked.go", "package x\n")
	writeFile(t, repo, untrackedNamePath, "package x\n")
	writeFile(t, repo, "ignored-untracked.go", "package x\n")

	t.Chdir(repo)
	cases := []struct {
		name   string
		filter operators.GitFilter
		want   []string
	}{
		{
			name:   "modified includes staged unstaged and untracked but not deleted",
			filter: operators.GitFilter{Modified: true},
			want:   []string{lineBreakPath, "modified.go", "staged.go", untrackedNamePath, "untracked.go"},
		},
		{
			name:   "staged only",
			filter: operators.GitFilter{Staged: true},
			want:   []string{"staged.go"},
		},
		{
			name:   "modified and staged union",
			filter: operators.GitFilter{Modified: true, Staged: true},
			want:   []string{lineBreakPath, "modified.go", "staged.go", untrackedNamePath, "untracked.go"},
		},
		{
			name:   "since takes precedence over modified while staged remains additive",
			filter: operators.GitFilter{Since: "HEAD^", Modified: true, Staged: true},
			want:   []string{lineBreakPath, "modified.go", "since-only.go", "staged.go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := collectSource(t, operators.PackConfig{
				Targets:        []string{"."},
				Since:          tc.filter.Since,
				Modified:       tc.filter.Modified,
				Staged:         tc.filter.Staged,
				Exclude:        "cli-excluded.go",
				TargetExcludes: map[string]string{repo: "target-excluded.go"},
			})
			got := includedRelPaths(files)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("included paths = %#v, want %#v", got, tc.want)
			}
			for _, file := range files {
				if file.Omitted {
					t.Fatalf("unexpected omission marker in Git mode: %#v", file)
				}
			}
		})
	}
}

func TestGitSourceKeepsOmittedMarkersAndWalkGuardrails(t *testing.T) {
	repo := newGitSourceFixture(t)
	writeFile(t, repo, "omitted_test.go", "package x\n")
	gitSourceCommand(t, repo, "add", "omitted_test.go")
	gitSourceCommand(t, repo, "commit", "-qm", "test fixture")
	writeFile(t, repo, "staged.go", "package x\n// staged\n")
	gitSourceCommand(t, repo, "add", "staged.go")
	t.Chdir(repo)

	files := collectSource(t, operators.PackConfig{Targets: []string{"."}, Staged: true, NoTests: true})
	var testMarker *operators.ParsedFile
	stagedIndex, markerIndex := -1, -1
	for i := range files {
		if files[i].RelPath == "omitted_test.go" {
			fileCopy := files[i]
			testMarker = &fileCopy
			markerIndex = i
		}
		if files[i].RelPath == "staged.go" {
			stagedIndex = i
		}
	}
	if testMarker == nil || !testMarker.Omitted || testMarker.OmitReason != "test" {
		t.Fatalf("unmodified test omission marker lost in Git mode: %#v", testMarker)
	}
	if markerIndex < 0 || stagedIndex < 0 || markerIndex >= stagedIndex {
		t.Fatalf("walk order not preserved: marker index=%d staged index=%d", markerIndex, stagedIndex)
	}

	// The generic walk still accounts for files before checking Git membership;
	// pruning the walk to only staged paths would incorrectly pass this limit.
	stream, err := operators.SourceOperator().DoStreamAny(context.Background(), operators.PackConfig{
		Targets: []string{"."}, Staged: true, MaxFilesScan: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var streamErr error
	stream(func(_ any, itemErr error) bool {
		if itemErr != nil {
			streamErr = itemErr
			return false
		}
		return true
	})
	if streamErr == nil || !strings.Contains(streamErr.Error(), "scan runaway prevented") {
		t.Fatalf("Git candidate preselection bypassed walk guardrail: %v", streamErr)
	}
}

func TestGitActionsAreRegisteredAndReusable(t *testing.T) {
	repo := newGitSourceFixture(t)
	untrackedNamePath := gitSourceFilename("untracked\nname.go")
	writeFile(t, repo, untrackedNamePath, "package x\n")
	ctx := context.Background()
	listed, err := operators.GitLSFilesAction().DoAny(ctx, operators.GitLSFilesRequest{Root: repo})
	if err != nil {
		t.Fatal(err)
	}
	paths, ok := listed.([]string)
	if !ok {
		t.Fatalf("git.ls_files returned %T", listed)
	}
	if !reflect.DeepEqual(paths, []string{filepath.Join(repo, untrackedNamePath)}) {
		t.Fatalf("git.ls_files paths = %#v", paths)
	}

	registered := map[string]bool{}
	for _, action := range operators.SourceLibrary().Actions {
		registered[action.Describe().Name] = true
	}
	for _, action := range operators.Library().Actions {
		registered[action.Describe().Name] = true
	}
	for _, name := range []string{"git.ls_files", "git.changed_paths"} {
		if !registered[name] {
			t.Errorf("source or aggregate library does not register %s", name)
		}
	}
}

func TestGitActionsErrorsAndCancellation(t *testing.T) {
	nonRepo := t.TempDir()
	_, err := operators.GitChangedPathsAction().DoAny(context.Background(), operators.GitChangedPathsRequest{
		Root: nonRepo, Filter: operators.GitFilter{Modified: true},
	})
	if err == nil || !strings.Contains(err.Error(), "git diff") {
		t.Fatalf("non-repository error = %v, want git diff failure", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = operators.GitChangedPathsAction().DoAny(ctx, operators.GitChangedPathsRequest{
		Root: nonRepo, Filter: operators.GitFilter{Since: "HEAD"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled action error = %v, want context.Canceled", err)
	}
}

func newGitSourceFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitSourceCommand(t, repo, "init", "-q")
	gitSourceCommand(t, repo, "config", "user.email", "source-test@example.invalid")
	gitSourceCommand(t, repo, "config", "user.name", "Source Test")
	writeFile(t, repo, ".gitignore", "ignored-tracked.go\nignored-untracked.go\n")
	writeFile(t, repo, "nexss.srcpack.ignore", "srcpack-ignored.go\n")
	for _, path := range []string{
		"keep.go", "modified.go", "staged.go", "deleted.go", "ignored-tracked.go",
		"srcpack-ignored.go", "cli-excluded.go", "target-excluded.go", gitSourceFilename("line\nbreak.go"),
	} {
		writeFile(t, repo, path, "package x\n")
	}
	gitSourceCommand(t, repo, "add", "-A")
	gitSourceCommand(t, repo, "add", "-f", "ignored-tracked.go")
	gitSourceCommand(t, repo, "commit", "-qm", "base")
	writeFile(t, repo, "since-only.go", "package x\n")
	gitSourceCommand(t, repo, "add", "since-only.go")
	gitSourceCommand(t, repo, "commit", "-qm", "since boundary")
	return repo
}

func gitSourceFilename(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(name, "\n", "-")
	}
	return name
}

func gitSourceCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func includedRelPaths(files []operators.ParsedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if !file.Omitted {
			paths = append(paths, filepath.ToSlash(file.RelPath))
		}
	}
	return paths
}
