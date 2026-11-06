package operators

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	repository "github.com/nexssp/repo"
)

// repositoryFileIndex reuses the shared provider's tracked and non-ignored
// untracked file discovery. Results are cached per Git root for one source scan.
type repositoryFileIndex struct {
	provider repository.Provider
	byRoot   map[string][]repository.File
}

func newRepositoryFileIndex() *repositoryFileIndex {
	return &repositoryFileIndex{
		provider: repository.New(),
		byRoot:   make(map[string][]repository.File),
	}
}

func (i *repositoryFileIndex) list(ctx context.Context, target string) (root string, files []repository.File, ok bool, err error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", nil, false, err
	}
	absTarget = filepath.Clean(absTarget)
	for knownRoot, knownFiles := range i.byRoot {
		if pathWithinRoot(knownRoot, absTarget) {
			return knownRoot, knownFiles, true, nil
		}
	}

	listed, err := i.provider.Files(ctx, repository.FilesRequest{Root: absTarget})
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, false, ctx.Err()
		}
		// Source packing has always supported ordinary directories as well as
		// Git worktrees. A non-repository target therefore keeps the legacy walk.
		return "", nil, false, nil
	}
	root = filepath.Clean(listed.Root)
	i.byRoot[root] = listed.Files
	return root, listed.Files, true, nil
}

func pathWithinRoot(root, path string) bool {
	root = sourcePathKey(root)
	path = sourcePathKey(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func sourcePathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func sourceRepoPath(repoRoot, path string) (string, error) {
	rel, err := filepath.Rel(sourcePathKey(repoRoot), sourcePathKey(path))
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("source path %q is outside repository root %q", path, repoRoot)
	}
	return filepath.ToSlash(rel), nil
}

// readRepositorySourceFile reads a selected working-tree file without reopening
// repository discovery. The caller has already enumerated the repository paths;
// calling provider.Pack here would repeat Git root discovery and file listing
// for every source file.
func readRepositorySourceFile(ctx context.Context, repoRoot, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := sourceRepoPath(repoRoot, path); err != nil {
		return nil, err
	}
	return readSourceFileBounded(path, maxBytes)
}

func readSourceFileBounded(path string, maxBytes int64) ([]byte, error) {
	limit := int64(sourceReadLimit(maxBytes))
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("file %s exceeds maximum allowed size", filepath.Base(path))
	}
	return content, nil
}

func sourceReadLimit(maxBytes int64) int {
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileSize
	}
	maxInt := int64(^uint(0) >> 1)
	if maxBytes > maxInt {
		return int(maxInt)
	}
	return int(maxBytes)
}

// changedPathsFromRepositoryDiff decodes Git's quoted diff headers and returns
// absolute changed paths. false means the bounded patch was incomplete or a
// header could not be decoded; callers should use srcpack's legacy path query.
func changedPathsFromRepositoryDiff(root, patch string) ([]string, bool) {
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(patch))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		_, right, ok := parseGitDiffHeader(line)
		if !ok || !strings.HasPrefix(right, "b/") {
			return nil, false
		}
		rel := filepath.FromSlash(strings.TrimPrefix(right, "b/"))
		seen[filepath.Clean(filepath.Join(root, rel))] = true
	}
	if scanner.Err() != nil {
		return nil, false
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, true
}

func parseGitDiffHeader(line string) (left, right string, ok bool) {
	body := strings.TrimPrefix(line, "diff --git ")
	if !strings.HasPrefix(body, `"`) {
		// Git may leave spaces unquoted in diff headers. Because the provider
		// disables rename detection, both sides carry the same relative path;
		// use that equality to identify the single " b/" separator safely.
		for offset := 0; ; {
			index := strings.Index(body[offset:], " b/")
			if index < 0 {
				break
			}
			index += offset
			candidateLeft, candidateRight := body[:index], body[index+1:]
			if strings.HasPrefix(candidateLeft, "a/") && strings.HasPrefix(candidateRight, "b/") && strings.TrimPrefix(candidateLeft, "a/") == strings.TrimPrefix(candidateRight, "b/") {
				return candidateLeft, candidateRight, true
			}
			offset = index + 1
		}
	}
	left, body, ok = parseGitDiffPath(body)
	if !ok {
		return "", "", false
	}
	right, body, ok = parseGitDiffPath(body)
	if !ok || strings.TrimSpace(body) != "" {
		return "", "", false
	}
	return left, right, true
}

func parseGitDiffPath(input string) (value, rest string, ok bool) {
	input = strings.TrimLeft(input, " ")
	if input == "" {
		return "", "", false
	}
	if input[0] != '"' {
		if space := strings.IndexByte(input, ' '); space >= 0 {
			return input[:space], strings.TrimLeft(input[space:], " "), true
		}
		return input, "", true
	}
	for index := 1; index < len(input); index++ {
		switch input[index] {
		case '\\':
			index++
		case '"':
			decoded, err := strconv.Unquote(input[:index+1])
			if err != nil {
				return "", "", false
			}
			return decoded, strings.TrimLeft(input[index+1:], " "), true
		}
	}
	return "", "", false
}
