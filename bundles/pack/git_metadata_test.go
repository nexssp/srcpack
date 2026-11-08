package packcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nexssp/srcpack/bundles/config"
)

func TestFinishResolutionPopulatesGitMetadata(t *testing.T) {
	root := t.TempDir()
	gitTestCommand(t, root, "init", "-q")
	gitTestCommand(t, root, "config", "user.name", "srcpack test")
	gitTestCommand(t, root, "config", "user.email", "srcpack@example.invalid")
	gitTestCommand(t, root, "symbolic-ref", "HEAD", "refs/heads/reviewer-branch")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, root, "add", "tracked.txt")
	gitTestCommand(t, root, "commit", "-q", "-m", "initial")
	gitTestCommand(t, root, "tag", "v1.2.3")
	wantCommit := gitTestCommand(t, root, "rev-parse", "HEAD")

	resolved, err := finishResolution(
		context.Background(),
		cliOptions{},
		time.Now(),
		resolutionPlan{WorkspaceRoot: root, Chain: []config.Profile{{Name: "default"}}},
		map[string]any{"paths": []string{}, "files": []string{}, "options": map[string]any{}},
	)
	if err != nil {
		t.Fatalf("finishResolution() error = %v", err)
	}
	if got := resolved.Render.GitCommit; got != wantCommit {
		t.Errorf("GitCommit = %q, want %q", got, wantCommit)
	}
	if got := resolved.Render.GitTag; got != "v1.2.3" {
		t.Errorf("GitTag = %q, want %q", got, "v1.2.3")
	}
	if got := resolved.Render.GitBranch; got != "reviewer-branch" {
		t.Errorf("GitBranch = %q, want %q", got, "reviewer-branch")
	}
}

func TestResolveGitMetadataUsesOneGitCommand(t *testing.T) {
	const root = "/workspace"
	const commit = "0123456789abcdef0123456789abcdef01234567"
	var calls [][]string
	run := func(_ context.Context, gotRoot string, args ...string) ([]byte, error) {
		if gotRoot != root {
			t.Fatalf("Git root = %q, want %q", gotRoot, root)
		}
		calls = append(calls, append([]string(nil), args...))
		switch strings.Join(args, " ") {
		case "log -1 --format=" + gitMetadataFormat:
			return []byte(commit + "\x00HEAD -> \nreview-branch, tag: v2.3.4\x00v2.3.4\n"), nil
		default:
			t.Fatalf("unexpected Git command: %q", args)
			return nil, nil
		}
	}

	got := resolveGitMetadataWith(context.Background(), root, run)
	if want := (gitMetadata{Commit: commit, Tag: "v2.3.4", Branch: "review-branch"}); got != want {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
	if want := [][]string{{"log", "-1", "--format=" + gitMetadataFormat}}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("Git calls = %#v, want %#v", calls, want)
	}
}

func TestGitMetadataSkippedInPlanMode(t *testing.T) {
	calls := 0
	resolve := func(context.Context, string) gitMetadata {
		calls++
		return gitMetadata{Commit: "unexpected"}
	}

	got := gitMetadataForResolution(context.Background(), true, "/workspace", resolve)
	if calls != 0 || got != (gitMetadata{}) {
		t.Fatalf("plan metadata = %#v after %d resolver calls, want empty and no call", got, calls)
	}
}

func TestFinishResolutionLeavesGitMetadataEmptyOutsideRepository(t *testing.T) {
	root := t.TempDir()
	resolved, err := finishResolution(
		context.Background(),
		cliOptions{},
		time.Now(),
		resolutionPlan{WorkspaceRoot: root, Chain: []config.Profile{{Name: "default"}}},
		map[string]any{"paths": []string{}, "files": []string{}, "options": map[string]any{}},
	)
	if err != nil {
		t.Fatalf("finishResolution() error = %v", err)
	}
	if got := resolved.Render.GitCommit + resolved.Render.GitTag + resolved.Render.GitBranch; got != "" {
		t.Errorf("Git metadata outside repository = %q, want empty", got)
	}
}

func gitTestCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", root}, args...)
	output, err := exec.CommandContext(context.Background(), "git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
