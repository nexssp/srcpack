package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		want  Spec
		bad   bool
	}{
		{"github:acme/widget", Spec{Repo: "acme/widget"}, false},
		{"github:acme/widget//cmd/tool?ref=feature%2Fnew-api", Spec{Repo: "acme/widget", Path: "cmd/tool", Ref: "feature/new-api"}, false},
		{"github:acme/widget//a%2Fb?ref=v1.2.0", Spec{Repo: "acme/widget", Path: "a/b", Ref: "v1.2.0"}, false},
		{"github:acme/widget//dir%23hash%2C%40%7E", Spec{Repo: "acme/widget", Path: "dir#hash,@~"}, false},
		{"github:acme/widget//../outside", Spec{}, true},
		{"github:acme/widget//%2e%2e/outside", Spec{}, true},
		{"github:acme/widget//src/.git/config", Spec{}, true},
		{"github:acme/widget//bad%2", Spec{}, true},
		{"github:acme/widget//a//b", Spec{}, true},
		{"github:acme/widget//a/./b", Spec{}, true},
		{"github:acme/widget?ref=--upload-pack=x", Spec{}, true},
		{"github:acme/widget?ref=feature%2F..%2Fmain", Spec{}, true},
		{"github:acme/widget#main", Spec{}, true},
		{"github:acme/widget@evil.example", Spec{}, true},
		{"https://github.com/acme/widget", Spec{}, true},
		{"github:acme/widget?ref=a&ref=b", Spec{}, true},
		{"github:acme/widget?ref=%2Fbranch", Spec{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.bad {
				t.Fatalf("Parse(%q) error = %v, want bad=%v", tt.input, err, tt.bad)
			}
			if err == nil && got != tt.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
	if !IsRemote("github:x/y") || IsRemote("git@github.com:x/y") {
		t.Fatal("IsRemote did not identify the github: scheme precisely")
	}
}

func TestMaterializeSparseFileAndCleanup(t *testing.T) {
	origin, commit, blobs := newOrigin(t)
	spec := Spec{Repo: "test/project", Path: "src/keep.txt", Ref: commit}
	result, err := materializeURL(context.Background(), spec, "file://"+filepath.ToSlash(origin), true)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()
	if result.Commit != commit {
		t.Fatalf("commit = %s, want %s", result.Commit, commit)
	}
	if result.Repository != "file://"+filepath.ToSlash(origin) {
		t.Fatalf("Repository = %q", result.Repository)
	}
	if !filepath.IsAbs(result.Path) || !filepath.IsAbs(result.Root) {
		t.Fatalf("expected absolute Path and Root, got %q and %q", result.Path, result.Root)
	}
	if got := strings.TrimSpace(git(t, result.Root, "config", "--get", "remote.origin.promisor")); got != "true" {
		t.Fatalf("origin promisor = %q", got)
	}
	if got := strings.TrimSpace(git(t, result.Root, "config", "--get", "remote.origin.partialclonefilter")); got != "blob:none" {
		t.Fatalf("origin partial clone filter = %q", got)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil || string(data) != "selected payload\n" {
		t.Fatalf("selected file = %q, error %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(result.Root, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("unselected top-level file unexpectedly checked out: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Root, "src", "other.txt")); !os.IsNotExist(err) {
		t.Fatalf("unselected sibling unexpectedly checked out: %v", err)
	}
	if objectSet := presentObjects(t, filepath.Join(result.Root, ".git")); strings.Contains(objectSet, blobs["README.md"]) {
		t.Fatal("unselected README blob is present in the partial clone")
	}
	root := result.Root
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("Close did not remove work area: %v", err)
	}
	if err := result.Close(); err != nil {
		t.Fatalf("Close is not idempotent: %v", err)
	}
}

func TestMaterializeDirectoryAndSymlinkRejection(t *testing.T) {
	origin, commit, _ := newOrigin(t)
	result, err := materializeURL(context.Background(), Spec{Repo: "test/project", Path: "src", Ref: commit}, "file://"+filepath.ToSlash(origin), true)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()
	for _, name := range []string{"keep.txt", "other.txt"} {
		if _, err := os.Stat(filepath.Join(result.Path, name)); err != nil {
			t.Fatalf("expected selected directory member %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(result.Root, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("unrelated top-level file unexpectedly checked out: %v", err)
	}

	// A second repository has a symlink beneath its selected directory.
	linkRepo := filepath.Join(t.TempDir(), "linked")
	must(t, os.MkdirAll(filepath.Join(linkRepo, "pkg"), 0o755))
	outside := filepath.Join(t.TempDir(), "secret.txt")
	must(t, os.WriteFile(outside, []byte("secret"), 0o600))
	must(t, os.WriteFile(filepath.Join(linkRepo, "pkg", "ordinary.txt"), []byte("ok"), 0o600))
	must(t, os.Symlink(outside, filepath.Join(linkRepo, "pkg", "escape")))
	git(t, linkRepo, "init", "--quiet")
	git(t, linkRepo, "config", "user.name", "Test")
	git(t, linkRepo, "config", "user.email", "test@example.invalid")
	git(t, linkRepo, "add", ".")
	git(t, linkRepo, "commit", "--quiet", "-m", "link")
	git(t, linkRepo, "config", "uploadpack.allowFilter", "true")
	for _, spec := range []Spec{
		{Repo: "test/linked", Path: "pkg"},
		{Repo: "test/linked", Path: "pkg/escape"},
		{Repo: "test/linked"},
	} {
		_, err = materializeURL(context.Background(), spec, "file://"+filepath.ToSlash(linkRepo), true)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("materialize(%#v) error = %v, want symlink rejection", spec, err)
		}
	}
}

func TestMaterializeCancellationAndMissingPath(t *testing.T) {
	origin, commit, _ := newOrigin(t)
	_, err := materializeURL(context.Background(), Spec{Repo: "test/project", Path: "missing", Ref: commit}, "file://"+filepath.ToSlash(origin), true)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing path error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = materializeURL(ctx, Spec{Repo: "test/project"}, "file://"+filepath.ToSlash(origin), true)
	if err == nil {
		t.Fatal("expected canceled context error")
	}
}

func newOrigin(t *testing.T) (string, string, map[string]string) {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin")
	must(t, os.Mkdir(origin, 0o755))
	git(t, origin, "init", "--quiet")
	git(t, origin, "config", "user.name", "Test")
	git(t, origin, "config", "user.email", "test@example.invalid")
	files := map[string]string{
		"README.md":       "large unrelated readme payload\n",
		"src/keep.txt":    "selected payload\n",
		"src/other.txt":   "unselected sibling payload\n",
		"docs/guide.txt":  "another unrelated payload\n",
		"top-level.extra": "another top-level file\n",
	}
	for name, contents := range files {
		path := filepath.Join(origin, filepath.FromSlash(name))
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(contents), 0o600))
	}
	git(t, origin, "add", ".")
	git(t, origin, "commit", "--quiet", "-m", "fixture")
	git(t, origin, "config", "uploadpack.allowFilter", "true")
	commit := strings.TrimSpace(git(t, origin, "rev-parse", "HEAD"))
	blobs := make(map[string]string, len(files))
	for name := range files {
		blobs[name] = strings.TrimSpace(git(t, origin, "rev-parse", "HEAD:"+name))
	}
	return origin, commit, blobs
}

func presentObjects(t *testing.T, gitDir string) string {
	t.Helper()
	// --batch-all-objects enumerates only objects already stored locally. A
	// direct lookup of a missing promisor object would silently lazy-fetch it.
	cmd := exec.CommandContext(t.Context(), "git", "--git-dir", gitDir, "cat-file", "--batch-all-objects", "--batch-check")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cat-file: %v: %s", err, out)
	}
	return string(out)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, out)
	}
	return string(out)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(fmt.Errorf("fixture setup: %w", err))
	}
}

func TestMaterializeDefaultHeadAndRootGitlinkRejection(t *testing.T) {
	origin, commit, _ := newOrigin(t)
	result, err := materializeURL(context.Background(), Spec{Repo: "test/project"}, "file://"+filepath.ToSlash(origin), true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Commit != commit {
		t.Fatalf("default HEAD commit = %s, want %s", result.Commit, commit)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "README.md")); err != nil {
		t.Fatalf("whole-repository default selection is missing README.md: %v", err)
	}
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "update-index", "--add", "--cacheinfo", "160000,"+commit+",external-module")
	git(t, origin, "commit", "--quiet", "-m", "add gitlink")
	_, err = materializeURL(context.Background(), Spec{Repo: "test/project"}, "file://"+filepath.ToSlash(origin), true)
	if err == nil || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("expected whole-repository gitlink rejection, got %v", err)
	}
}
