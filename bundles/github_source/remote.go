// Package remote parses and materializes GitHub source specifications.
package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Spec identifies one GitHub repository, an optional repository-relative path,
// and an optional branch, tag, or commit reference.
type Spec struct {
	Repo string // owner/repository
	Path string // optional file or directory within the repository
	Ref  string // optional branch, tag, or commit
}

// Result describes a materialized source. Call Cleanup when the source is no
// longer needed; it removes the complete private work area, including .git.
type Result struct {
	Path       string // absolute path to the selected file, directory, or repository root
	Root       string // absolute path to the repository checkout, including its private .git directory
	Commit     string // full commit object ID
	Repository string // canonical https://github.com/owner/repository.git URL

	workDir string
}

// Cleanup removes the private working copy. It is safe to call more than once.
func (r *Result) Cleanup() error {
	if r == nil || r.workDir == "" {
		return nil
	}
	err := os.RemoveAll(r.workDir)
	if err == nil {
		r.workDir = ""
	}
	return err
}

// Close is an alias for Cleanup and allows Result to be used as an owned resource.
func (r *Result) Close() error { return r.Cleanup() }

var (
	repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	fullOID  = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
)

// IsRemote reports whether input uses the github: source scheme.
func IsRemote(input string) bool { return strings.HasPrefix(input, "github:") }

// Parse validates github:owner/repo[//sub/path][?ref=url-encoded-ref].
func Parse(input string) (Spec, error) {
	if !IsRemote(input) {
		return Spec{}, errors.New("remote source must start with the github: prefix")
	}
	rest := strings.TrimPrefix(input, "github:")
	if strings.ContainsAny(rest, "#~") {
		return Spec{}, errors.New("remote source cannot contain # or ~; use the ref query parameter")
	}
	if strings.Contains(rest, "\\") {
		return Spec{}, errors.New("remote source cannot contain backslashes")
	}
	base, query, hasQuery := strings.Cut(rest, "?")
	spec := Spec{}
	if hasQuery {
		ref, err := parseRemoteRef(query)
		if err != nil {
			return Spec{}, err
		}
		spec.Ref = ref
	}
	repo, path, hasPath := strings.Cut(base, "//")
	if err := parseRepository(&spec, repo, path, hasPath); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func parseRemoteRef(query string) (string, error) {
	if strings.Contains(query, "?") {
		return "", errors.New("remote source has more than one query delimiter")
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return "", fmt.Errorf("invalid remote query: %w", err)
	}
	if len(values) != 1 || len(values["ref"]) != 1 {
		return "", errors.New("remote query must contain exactly one ref parameter")
	}
	ref := values.Get("ref")
	if err := validateRef(ref); err != nil {
		return "", err
	}
	return ref, nil
}

func parseRepository(spec *Spec, repo, path string, hasPath bool) error {
	if strings.Contains(repo, "@") {
		return errors.New("remote source must name only a GitHub owner and repository")
	}
	if hasPath && strings.Contains(path, "//") {
		return errors.New("remote source path contains an empty component")
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || !validRepoPart(parts[0]) || !validRepoPart(parts[1]) || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		return errors.New("remote repository must be github:owner/repo")
	}
	spec.Repo = parts[0] + "/" + parts[1]
	if !hasPath {
		return nil
	}
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return fmt.Errorf("invalid encoded remote subpath: %w", err)
	}
	if err := validatePath(decoded); err != nil {
		return err
	}
	spec.Path = decoded
	return nil
}

func validRepoPart(s string) bool { return s != "" && repoPart.MatchString(s) }

func validatePath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
		return errors.New("remote subpath must be a non-empty relative path")
	}
	if strings.ContainsAny(p, "\\\x00") {
		return errors.New("remote subpath contains an invalid character")
	}
	for component := range strings.SplitSeq(p, "/") {
		if component == "" || component == "." || component == ".." || strings.EqualFold(component, ".git") {
			return errors.New("remote subpath contains an empty or traversal component")
		}
		for _, r := range component {
			if unicode.IsControl(r) {
				return errors.New("remote subpath contains a control character")
			}
		}
	}
	return nil
}

func validateRef(ref string) error {
	if ref == "" {
		return errors.New("ref must not be empty")
	}
	if fullOID.MatchString(ref) {
		return nil
	}
	if strings.HasPrefix(ref, "-") || strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".") || strings.Contains(ref, "..") || strings.Contains(ref, "@{") || strings.Contains(ref, "//") {
		return errors.New("invalid Git ref")
	}
	for _, r := range ref {
		if unicode.IsControl(r) || unicode.IsSpace(r) || strings.ContainsRune("~^:?*[\\", r) {
			return errors.New("invalid Git ref")
		}
	}
	for component := range strings.SplitSeq(ref, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return errors.New("invalid Git ref")
		}
	}
	return nil
}

// Materialize resolves a GitHub source to a fresh sparse worktree. It always
// contacts the fixed github.com HTTPS endpoint, fetches only depth one with
// blob filtering, performs no checkout hooks or submodule initialization, and
// does not run downloaded code. The caller owns the result and must call
// Result.Cleanup after use. An omitted subpath intentionally selects the
// complete repository; specifying a subpath limits checkout to that path.
// Every call creates a fresh temporary work area; no persistent cache is used.
func Materialize(ctx context.Context, raw string) (Result, error) {
	spec, err := Parse(raw)
	if err != nil {
		return Result{}, err
	}
	return materializeURL(ctx, spec, "https://github.com/"+spec.Repo+".git", false)
}

// materializeURL is injectable only within this package so tests can use a
// deterministic local Git repository; production entry points never accept a URL.
//
//nolint:contextcheck // A nil context has no parent; preserve the background fallback.
func materializeURL(ctx context.Context, spec Spec, remoteURL string, allowFile bool) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workDir, err := os.MkdirTemp("", "srcpack-remote-")
	if err != nil {
		return Result{}, fmt.Errorf("create remote work area: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(workDir)
		}
	}()
	gitDir, runner, err := initializeGit(ctx, workDir, remoteURL, allowFile)
	if err != nil {
		return Result{}, err
	}
	commit, err := fetchCommit(ctx, runner, spec)
	if err != nil {
		return Result{}, err
	}
	pattern, err := sparsePattern(ctx, runner, spec, commit)
	if err != nil {
		return Result{}, err
	}
	if _, runErr := runner.run(ctx, "sparse-checkout", "init", "--no-cone"); runErr != nil {
		return Result{}, fmt.Errorf("enable literal sparse checkout: %w", runErr)
	}
	if _, runErr := runner.run(ctx, "sparse-checkout", "set", "--no-cone", "--", pattern); runErr != nil {
		return Result{}, fmt.Errorf("select sparse checkout path: %w", runErr)
	}
	if _, runErr := runner.run(ctx, "checkout", "--quiet", "--detach", commit); runErr != nil {
		return Result{}, fmt.Errorf("checkout selected source: %w", runErr)
	}

	root, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve materialized repository: %w", err)
	}
	target := root
	if spec.Path != "" {
		target = filepath.Join(root, filepath.FromSlash(spec.Path))
	}
	if _, statErr := os.Lstat(target); statErr != nil {
		return Result{}, fmt.Errorf("materialized path %q is unavailable: %w", spec.Path, statErr)
	}
	if symlinkErr := checkSymlinks(root, target); symlinkErr != nil {
		return Result{}, symlinkErr
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return Result{}, fmt.Errorf("resolve result path: %w", err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve repository root path: %w", err)
	}
	keep = true
	return Result{Path: abs, Root: rootAbs, Commit: commit, Repository: remoteURL, workDir: workDir}, nil
}

func initializeGit(ctx context.Context, workDir, remoteURL string, allowFile bool) (string, gitRunner, error) {
	home := filepath.Join(workDir, "home")
	template := filepath.Join(workDir, "empty-template")
	if mkdirErr := os.MkdirAll(home, 0o700); mkdirErr != nil {
		return "", gitRunner{}, fmt.Errorf("create isolated Git home: %w", mkdirErr)
	}
	if mkdirErr := os.Mkdir(template, 0o700); mkdirErr != nil {
		return "", gitRunner{}, fmt.Errorf("create empty Git template: %w", mkdirErr)
	}
	gitDir := filepath.Join(workDir, "repo")
	if mkdirErr := os.Mkdir(gitDir, 0o700); mkdirErr != nil {
		return "", gitRunner{}, fmt.Errorf("create Git worktree: %w", mkdirErr)
	}
	runner := gitRunner{dir: gitDir, home: home, template: template, allowFile: allowFile}
	if _, runErr := runner.run(ctx, "init", "--quiet", "--template="+template); runErr != nil {
		return "", gitRunner{}, fmt.Errorf("initialize isolated repository: %w", runErr)
	}
	if _, runErr := runner.run(ctx, "remote", "add", "origin", remoteURL); runErr != nil {
		return "", gitRunner{}, fmt.Errorf("configure remote: %w", runErr)
	}
	if _, runErr := runner.run(ctx, "config", "remote.origin.promisor", "true"); runErr != nil {
		return "", gitRunner{}, fmt.Errorf("configure partial-clone promisor: %w", runErr)
	}
	if _, runErr := runner.run(ctx, "config", "remote.origin.partialclonefilter", "blob:none"); runErr != nil {
		return "", gitRunner{}, fmt.Errorf("configure partial-clone filter: %w", runErr)
	}
	return gitDir, runner, nil
}

func fetchCommit(ctx context.Context, runner gitRunner, spec Spec) (string, error) {
	ref := spec.Ref
	if ref == "" {
		ref = "HEAD"
	}
	fetchOut, err := runner.run(ctx, "fetch", "--depth=1", "--filter=blob:none", "--no-tags", "origin", ref)
	if err != nil {
		return "", fmt.Errorf("fetch remote source: %w", err)
	}
	if strings.Contains(strings.ToLower(fetchOut), "filtering not recognized") || strings.Contains(strings.ToLower(fetchOut), "filtering not supported") {
		return "", errors.New("git server does not support partial-clone filtering; aborting after fetch (the server may already have transferred full content)")
	}
	commitOut, err := runner.run(ctx, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve fetched commit: %w", err)
	}
	commit := strings.TrimSpace(commitOut)
	if !fullOID.MatchString(commit) {
		return "", errors.New("git returned an invalid commit object ID")
	}

	return commit, nil
}

func sparsePattern(ctx context.Context, runner gitRunner, spec Spec, commit string) (string, error) {
	if spec.Path != "" {
		return selectedPathPattern(ctx, runner, spec, commit)
	}
	if err := repositoryHasNoSubmodules(ctx, runner, commit); err != nil {
		return "", err
	}
	return "/*", nil
}

func selectedPathPattern(ctx context.Context, runner gitRunner, spec Spec, commit string) (string, error) {
	entriesOut, inspectErr := runner.run(ctx, "ls-tree", "-r", "-z", "--full-tree", commit, "--", ":(literal)"+spec.Path)
	if inspectErr != nil {
		return "", fmt.Errorf("inspect selected repository path: %w", inspectErr)
	}
	entries := bytes.Split([]byte(entriesOut), []byte{0})
	found := false
	isDirectory := false
	for _, entry := range entries {
		if len(entry) == 0 {
			continue
		}
		meta, name, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			return "", errors.New("git returned malformed tree metadata")
		}
		fields := strings.Fields(string(meta))
		if len(fields) != 3 {
			return "", errors.New("git returned malformed tree entry")
		}
		entryPath := string(name)
		if fields[0] == "120000" {
			return "", fmt.Errorf("selected path contains unsupported Git symlink %q", entryPath)
		}
		if entryPath == spec.Path {
			found = true
			isDirectory = fields[1] == "tree"
		}
		if fields[0] == "160000" || fields[1] == "commit" {
			return "", fmt.Errorf("selected path contains unsupported Git submodule %q", entryPath)
		}
	}
	if !found {
		// ls-tree -r omits the tree node for a directory; a descendant is enough
		// to establish that it exists. Empty directories are not represented in Git.
		for _, entry := range entries {
			if len(entry) == 0 {
				continue
			}
			_, name, ok := bytes.Cut(entry, []byte{'\t'})
			if ok && strings.HasPrefix(string(name), spec.Path+"/") {
				found = true
				isDirectory = true
				break
			}
		}
	}
	if !found {
		return "", fmt.Errorf("selected path %q does not exist in commit %s", spec.Path, commit)
	}
	pattern := "/" + escapeSparsePattern(spec.Path)
	if isDirectory {
		pattern += "/"
	}
	return pattern, nil
}

func repositoryHasNoSubmodules(ctx context.Context, runner gitRunner, commit string) error {
	// Trees and gitlinks are metadata; inspecting them does not fetch blobs.
	// Do not silently return an incomplete tree for whole-repository requests.
	entriesOut, inspectErr := runner.run(ctx, "ls-tree", "-r", "-z", "--full-tree", commit)
	if inspectErr != nil {
		return fmt.Errorf("inspect repository tree: %w", inspectErr)
	}
	for entry := range bytes.SplitSeq([]byte(entriesOut), []byte{0}) {
		meta, name, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			continue
		}
		fields := strings.Fields(string(meta))
		if len(fields) == 3 {
			if fields[0] == "120000" {
				return fmt.Errorf("repository contains unsupported Git symlink %q", string(name))
			}
			if fields[0] == "160000" || fields[1] == "commit" {
				return fmt.Errorf("repository contains unsupported Git submodule %q", string(name))
			}
		}
	}
	return nil
}

func escapeSparsePattern(path string) string {
	var b strings.Builder
	for _, r := range path {
		if strings.ContainsRune(`\\*?[`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func checkSymlinks(root, target string) error {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("resolve selected path safely: %w", err)
	}
	if !within(rootReal, resolved) {
		return errors.New("selected path resolves outside the materialized repository")
	}
	relTarget, err := filepath.Rel(root, target)
	if err != nil || relTarget == ".." || strings.HasPrefix(relTarget, ".."+string(filepath.Separator)) {
		return errors.New("selected path is outside the materialized repository")
	}
	current := root
	for component := range strings.SplitSeq(relTarget, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		entryInfo, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("selected source path contains symlink %q", current)
		}
	}
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		err = filepath.WalkDir(target, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("selected source contains unsupported symlink %q", path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.max - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buf.Write(p)
	}
	if n > remaining {
		b.truncated = true
	}
	return n, nil
}

type gitRunner struct {
	dir       string
	home      string
	template  string
	allowFile bool
}

func (g gitRunner) run(parent context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	gitArgs := make([]string, 0, 10+len(args))
	gitArgs = append(gitArgs, "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=")
	if g.allowFile {
		gitArgs = append(gitArgs, "-c", "protocol.file.allow=always")
	}
	gitArgs = append(gitArgs, "-c", "fetch.fsckObjects=true", "-c", "http.followRedirects=false")
	gitArgs = append(gitArgs, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = g.dir
	cmd.Env = gitEnvironment(g.home, g.template, g.allowFile)
	var stdout, stderr cappedBuffer
	stdout.max = 1 << 20
	stderr.max = 8 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.buf.String()
	if stdout.truncated || stderr.truncated {
		return "", fmt.Errorf("git %s output exceeded the capture limit (stdout 1 MiB, stderr 8 KiB)", strings.Join(args, " "))
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("git %s timed out", strings.Join(args, " "))
		}
		if parent.Err() != nil {
			return "", parent.Err()
		}
		msg := strings.TrimSpace(stderr.buf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	if stderr.buf.Len() > 0 {
		out += stderr.buf.String()
	}
	return out, nil
}

func gitEnvironment(home, template string, allowFile bool) []string {
	base := make([]string, 0, len(os.Environ())+8)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "GIT_") || strings.HasPrefix(upper, "SSH_") || strings.HasPrefix(upper, "GIT_CONFIG") {
			continue
		}
		base = append(base, item)
	}
	protocol := "https"
	if allowFile {
		protocol = "https:file"
	}
	return append(base,
		"HOME="+home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_LFS_SKIP_SMUDGE=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_ALLOW_PROTOCOL="+protocol,
		"GIT_TEMPLATE_DIR="+template,
		"LC_ALL=C",
	)
}

var _ io.Writer = (*cappedBuffer)(nil)
