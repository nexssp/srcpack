package operators

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	repository "github.com/nexssp/repo"
	ignore "github.com/sabhiram/go-gitignore"
)

// changeFilter selects a candidate path by Git membership or incremental state.
// Git is authoritative when active; state is consulted only otherwise.
type changeFilter struct {
	gitPaths map[string]bool
	state    *StateCache
}

func (f *changeFilter) Include(path, relPath string) bool {
	if f == nil {
		return true
	}
	if f.gitPaths != nil {
		return f.gitPaths[sourcePathKey(path)]
	}
	if f.state != nil {
		info, err := os.Stat(path)
		if err != nil {
			return true
		}
		changed, err := f.state.IsChanged(relPath, path, info.ModTime())
		if err != nil {
			return true
		}
		return changed
	}
	return true
}

// sourceFileFilter is the common policy pipeline for filesystem-walk and
// repository-index candidates. It preserves the established order:
// ignore/exclude, omission markers, guardrails, then changed-path selection.
type sourceFileFilter struct {
	matcher    *ignore.GitIgnore
	extensions map[string]bool
	examples   []string
	guardrails *Guardrails
	changes    *changeFilter
}

// SourceFilePolicy applies the packer's ignore and file-exclusion rules for one root.
type SourceFilePolicy struct {
	matcher    *ignore.GitIgnore
	cfg        PackConfig
	extensions map[string]bool
	examples   []string
}

// NewSourceFilePolicy builds the packer's path policy for a root and pack configuration.
func NewSourceFilePolicy(root string, cfg PackConfig) SourceFilePolicy {
	examples := cfg.Examples
	if len(examples) == 0 {
		examples = defaultExampleFolders
	}
	return SourceFilePolicy{
		matcher:    buildIgnoreMatcher(root, cfg.Exclude),
		cfg:        cfg,
		extensions: parseExtensions(cfg.Ext),
		examples:   examples,
	}
}

// Includes reports whether a candidate passes the packer's ignore and file exclusions.
func (p SourceFilePolicy) Includes(path, relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	return !p.matcher.MatchesPath(relPath) && skipReason(path, relPath, p.cfg, p.extensions, p.examples) == ""
}

// Decide applies the exact same filters to any source candidate. keepGoing is
// false only when an emitted error or omission marker asks the stream to stop.
func (f sourceFileFilter) Decide(
	ctx context.Context,
	path string,
	entry os.DirEntry,
	relPath string,
	cfg PackConfig,
	yield func(ParsedFile, error) bool,
) (info os.FileInfo, included, keepGoing bool) {
	if err := ctx.Err(); err != nil {
		var zero ParsedFile
		return nil, false, yield(zero, err)
	}
	if f.matcher != nil && f.matcher.MatchesPath(relPath) {
		return nil, false, true
	}
	info, err := entry.Info()
	if err != nil {
		return nil, false, true
	}

	if reason := skipReason(path, relPath, cfg, f.extensions, f.examples); reason != "" {
		if !treeVisibleReasons[reason] {
			return info, false, true
		}
		return info, false, yield(ParsedFile{
			Path:       path,
			RelPath:    relPath,
			Size:       info.Size(),
			Lang:       detectLang(path),
			Omitted:    true,
			OmitReason: reason,
		}, nil)
	}

	if err := f.guardrails.CheckFile(path, info); err != nil {
		var zero ParsedFile
		return info, false, yield(zero, err)
	}
	if !f.changes.Include(path, relPath) {
		return info, false, true
	}
	return info, true, true
}

// buildChangeFilter consumes the Repo-selected handoff from NFlow. Direct Go
// callers use the same Repo provider; state-cache filtering remains separate.
func buildChangeFilter(ctx context.Context, root string, cfg PackConfig) (*changeFilter, error) {
	gitFilter := GitFilter{Since: cfg.Since, Modified: cfg.Modified, Staged: cfg.Staged}
	if gitFilter.Active() {
		paths := cfg.ChangedPaths
		if !cfg.ChangedPathsProvided {
			result, err := repository.New().ChangedPaths(ctx, repository.ChangedPathsRequest{
				Root: root, Since: gitFilter.Since, Modified: gitFilter.Modified, Staged: gitFilter.Staged,
			})
			if err != nil {
				return nil, fmt.Errorf("srcpack.source: %w", err)
			}
			paths = result.Paths
		}
		set := make(map[string]bool, len(paths))
		for _, path := range paths {
			set[sourcePathKey(path)] = true
		}
		return &changeFilter{gitPaths: set}, nil
	}
	if cfg.Changed {
		return &changeFilter{state: LoadState(StateFileName)}, nil
	}
	return nil, nil
}

func buildIgnoreMatcher(root, excludes string) *ignore.GitIgnore {
	var lines []string
	for _, name := range []string{".gitignore", "nexss.srcpack.ignore"} {
		if data, err := os.ReadFile(filepath.Join(root, name)); err == nil {
			lines = append(lines, strings.Split(string(data), "\n")...)
		}
	}
	lines = append(lines, ".git", "node_modules", "vendor", "dist", "build", "target", "_srcpack", ".nexss.srcpack.state")
	for pattern := range strings.SplitSeq(excludes, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" {
			lines = append(lines, pattern)
		}
	}
	return ignore.CompileIgnoreLines(lines...)
}
