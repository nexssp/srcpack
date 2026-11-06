package operators

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nexssp/kernel/action"
	repository "github.com/nexssp/repo"
)

// GitFilter selects which Git change sets to include. Since takes precedence
// over Modified; Staged is additive to either selection.
type GitFilter struct {
	Since    string `json:"since"`
	Modified bool   `json:"modified"`
	Staged   bool   `json:"staged"`
}

func (f GitFilter) Active() bool {
	return f.Since != "" || f.Modified || f.Staged
}

// GitLSFilesRequest asks Git for untracked, non-ignored working-tree paths.
type GitLSFilesRequest struct {
	Root string `json:"root"`
}

// GitChangedPathsRequest selects changed paths using a Git change filter.
type GitChangedPathsRequest struct {
	Root   string    `json:"root"`
	Filter GitFilter `json:"filter"`
}

// GitLSFilesAction exposes Git's reusable untracked-file query.
func GitLSFilesAction() action.AnyAction {
	return action.New("git.ls_files", func(ctx context.Context, req GitLSFilesRequest) ([]string, error) {
		out, err := runGit(ctx, req.Root, "ls-files", "--others", "--exclude-standard", "-z")
		if err != nil {
			return nil, fmt.Errorf("git ls-files: %w", err)
		}
		return gitPaths(req.Root, out), nil
	}).Description("List untracked non-ignored Git paths").Build()
}

// GitChangedPathsAction exposes the changed-path set used by incremental source scans.
func GitChangedPathsAction() action.AnyAction {
	return action.New("git.changed_paths", func(ctx context.Context, req GitChangedPathsRequest) ([]string, error) {
		paths := make(map[string]bool)
		add := func(values []string) {
			for _, path := range values {
				paths[filepath.Clean(path)] = true
			}
		}

		switch {
		case req.Filter.Since != "":
			out, err := runGit(ctx, req.Root, "diff", "--name-only", "-z", req.Filter.Since)
			if err != nil {
				return nil, fmt.Errorf("git diff %s: %w", req.Filter.Since, err)
			}
			add(gitPaths(req.Root, out))
		case req.Filter.Modified:
			out, err := runGit(ctx, req.Root, "diff", "--name-only", "-z", "HEAD")
			if err != nil {
				out, err = runGit(ctx, req.Root, "diff", "--name-only", "-z")
				if err != nil {
					return nil, fmt.Errorf("git diff: %w", err)
				}
			}
			add(gitPaths(req.Root, out))
		}

		if req.Filter.Staged {
			out, err := runGit(ctx, req.Root, "diff", "--name-only", "-z", "--cached")
			if err != nil {
				return nil, fmt.Errorf("git diff --cached: %w", err)
			}
			add(gitPaths(req.Root, out))
		}

		if req.Filter.Since == "" && req.Filter.Modified {
			// Preserve the historical behavior of tolerating a failed untracked
			// listing, but never hide cancellation from the caller.
			untracked, err := GitLSFilesAction().DoAny(ctx, GitLSFilesRequest{Root: req.Root})
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			} else if values, ok := untracked.([]string); ok {
				add(values)
			}
		}

		result := make([]string, 0, len(paths))
		for path := range paths {
			result = append(result, path)
		}
		slices.Sort(result)
		return result, nil
	}).Description("List paths selected by a Git since, modified, and staged filter").Build()
}

// GitActions returns the Git actions used by source scans and available to NFlow.
func GitActions() []action.AnyAction {
	return []action.AnyAction{GitLSFilesAction(), GitChangedPathsAction()}
}

func gitPaths(root string, data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		root = filepath.Clean(root)
	}
	var paths []string
	for path := range strings.SplitSeq(string(data), "\x00") {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		paths = append(paths, filepath.Clean(path))
	}
	return paths
}

// GitUnifiedDiff returns the raw patch for the filter's active mode.
func GitUnifiedDiff(ctx context.Context, root string, filter GitFilter) (string, error) {
	if filter.Staged || (filter.Since == "" && filter.Modified) {
		diff, err := repository.New().Diff(ctx, repository.DiffRequest{Root: root, Staged: filter.Staged})
		if err == nil && !diff.Truncated {
			return diff.Patch, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Keep the previous complete-patch behavior if the provider's bounded
		// result was truncated or the target is not a usable Git worktree.
	}
	return legacyGitUnifiedDiff(ctx, root, filter)
}

func legacyGitUnifiedDiff(ctx context.Context, root string, filter GitFilter) (string, error) {
	switch {
	case filter.Staged:
		out, err := runGit(ctx, root, "diff", "--cached")
		if err != nil {
			return "", fmt.Errorf("git diff --cached: %w", err)
		}
		return string(out), nil
	case filter.Since != "":
		out, err := runGit(ctx, root, "diff", filter.Since)
		if err != nil {
			return "", fmt.Errorf("git diff %s: %w", filter.Since, err)
		}
		return string(out), nil
	case filter.Modified:
		out, err := runGit(ctx, root, "diff", "HEAD")
		if err != nil {
			out, err = runGit(ctx, root, "diff")
			if err != nil {
				return "", fmt.Errorf("git diff: %w", err)
			}
		}
		return string(out), nil
	}
	return "", nil
}

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return output, err
}
