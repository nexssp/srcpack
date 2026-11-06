package operators

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	repository "github.com/nexssp/repo"
)

func TestReadRepositorySourceFilePreservesBytes(t *testing.T) {
	root := newRepositoryTestGitRoot(t)
	cases := map[string][]byte{
		"text.go":             []byte("package source\nfunc Main() {}\n"),
		"invalid-utf8.txt":    {0xff, 0xfe, 'x', '\n'},
		"binary-with-nul.txt": {'a', 0, 'b', 0xff},
		"space name.txt":      []byte("space\n"),
	}
	for path, content := range cases {
		writeRepositoryTestFile(t, root, path, content)
	}
	if err := os.Symlink("text.go", filepath.Join(root, "link.go")); err == nil {
		runRepositoryTestGit(t, root, "add", "-A")
		cases["link.go"] = cases["text.go"]
	} else {
		t.Logf("symlink parity skipped: %v", err)
		runRepositoryTestGit(t, root, "add", "-A")
	}
	runRepositoryTestGit(t, root, "commit", "-qm", "fixture")

	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			got, err := readRepositorySourceFile(context.Background(), root, filepath.Join(root, path), 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("shared read changed bytes: want %x, got %x", want, got)
			}
		})
	}
}

func TestReadRepositorySourceFileRejectsOutsideRootAndAppliesLimit(t *testing.T) {
	root := newRepositoryTestGitRoot(t)
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "outside.txt")
	writeRepositoryTestFile(t, outside, "outside.txt", []byte("outside"))
	if _, err := readRepositorySourceFile(context.Background(), root, outsidePath, 1<<20); err == nil || !strings.Contains(err.Error(), "outside repository root") {
		t.Fatalf("outside-repository read error = %v, want path-boundary error", err)
	}

	writeRepositoryTestFile(t, root, "too-large.txt", []byte("larger than limit"))
	if _, err := readRepositorySourceFile(context.Background(), root, filepath.Join(root, "too-large.txt"), 4); err == nil || !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Fatalf("oversized read error = %v, want file-size error", err)
	}
}

func TestPathWithinRootIsCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows paths are case-insensitive")
	}
	root := t.TempDir()
	if sourcePathKey(strings.ToUpper(root)) != sourcePathKey(strings.ToLower(root)) {
		t.Fatal("case-only Windows path differences should share a changed-path key")
	}
	if !pathWithinRoot(strings.ToUpper(root), strings.ToLower(filepath.Join(root, "child.go"))) {
		t.Fatal("case-only Windows path differences should remain inside the repository")
	}
	if pathWithinRoot(strings.ToUpper(root), strings.ToLower(root+"-outside")) {
		t.Fatal("a path outside the repository must not match by prefix")
	}
}

func TestRepositoryDiffPathsDecodeGitQuotedNames(t *testing.T) {
	root := newRepositoryTestGitRoot(t)
	path := "space name.go"
	writeRepositoryTestFile(t, root, path, []byte("package x\n"))
	runRepositoryTestGit(t, root, "add", "-A")
	runRepositoryTestGit(t, root, "commit", "-qm", "base")
	writeRepositoryTestFile(t, root, path, []byte("package x\n// changed\n"))

	diff, err := repository.New().Diff(context.Background(), repository.DiffRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if diff.Truncated {
		t.Fatal("small test diff was unexpectedly truncated")
	}
	got, ok := changedPathsFromRepositoryDiff(diff.Root, diff.Patch)
	if !ok {
		t.Fatalf("could not decode provider patch:\n%s", diff.Patch)
	}
	if want := []string{filepath.Join(root, path)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed paths = %#v, want %#v", got, want)
	}

	// Windows forbids control characters and double quotes in filesystem names,
	// but the parser must still decode Git's C-quoted patch headers on every OS.
	quotedPatch := "diff --git \"a/line\\nbreak.go\" \"b/line\\nbreak.go\"\n" +
		"diff --git \"a/quote\\\"name.go\" \"b/quote\\\"name.go\"\n"
	got, ok = changedPathsFromRepositoryDiff(root, quotedPatch)
	if !ok {
		t.Fatalf("could not decode quoted Git patch:\n%s", quotedPatch)
	}
	want := []string{filepath.Join(root, "line\nbreak.go"), filepath.Join(root, "quote\"name.go")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded quoted paths = %#v, want %#v", got, want)
	}
}

func TestGitUnifiedDiffParityWithLegacyForSupportedModes(t *testing.T) {
	root := newRepositoryTestGitRoot(t)
	writeRepositoryTestFile(t, root, "modified.go", []byte("package x\n"))
	writeRepositoryTestFile(t, root, "staged.go", []byte("package x\n"))
	runRepositoryTestGit(t, root, "add", "-A")
	runRepositoryTestGit(t, root, "commit", "-qm", "base")
	writeRepositoryTestFile(t, root, "modified.go", []byte("package x\n// modified\n"))
	writeRepositoryTestFile(t, root, "staged.go", []byte("package x\n// staged\n"))
	runRepositoryTestGit(t, root, "add", "staged.go")

	for _, filter := range []GitFilter{{Modified: true}, {Staged: true}} {
		got, err := GitUnifiedDiff(context.Background(), root, filter)
		if err != nil {
			t.Fatal(err)
		}
		want, err := legacyGitUnifiedDiff(context.Background(), root, filter)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("diff output differs for %#v\nwant:\n%s\ngot:\n%s", filter, want, got)
		}
	}

	// Since-ref semantics remain on the established implementation because the
	// shared provider does not yet accept an arbitrary comparison ref.
	filter := GitFilter{Since: "HEAD", Modified: true}
	got, err := GitUnifiedDiff(context.Background(), root, filter)
	if err != nil {
		t.Fatal(err)
	}
	want, err := legacyGitUnifiedDiff(context.Background(), root, filter)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || !strings.Contains(got, "modified.go") {
		t.Fatalf("since-ref diff parity failed: got=%q want=%q", got, want)
	}
}

func newRepositoryTestGitRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runRepositoryTestGit(t, root, "init", "-q")
	runRepositoryTestGit(t, root, "config", "user.email", "repo-parity@example.invalid")
	runRepositoryTestGit(t, root, "config", "user.name", "Repo Parity")
	return root
}

func writeRepositoryTestFile(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runRepositoryTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
