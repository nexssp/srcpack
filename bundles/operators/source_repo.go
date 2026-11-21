package operators

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
