package operators

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"iter"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nexssp/kernel/action"
)

var defaultExampleFolders = []string{
	"examples", "example", "samples", "sample", "demo", "demos",
}

var treeVisibleReasons = map[string]bool{
	"test":      true,
	"examples":  true,
	"generated": true,
	"ext":       true,
}

var binaryExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".bin": true, ".obj": true, ".o": true, ".a": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".ico": true, ".webp": true, ".pdf": true,
	".zip": true, ".tar": true, ".gz": true, ".7z": true,
	".db": true, ".sqlite": true, ".sqlite3": true,
	".pyc": true, ".pyo": true, ".wasm": true,
}

var lockFiles = map[string]bool{
	"Cargo.lock":        true,
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"composer.lock":     true,
	"poetry.lock":       true,
	"Gemfile.lock":      true,
	"go.sum":            true,
}

//nolint:gocyclo // single walk closure with per-target state; extraction needs a six-parameter helper.
func SourceOperator() action.AnyStreamAction {
	return action.NewStream("srcpack.source", func(ctx context.Context, cfg PackConfig) (iter.Seq2[ParsedFile, error], error) {
		root, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("srcpack.source: get working directory: %w", err)
		}
		repositoryFiles := newRepositoryFileIndex()
		matcher := buildIgnoreMatcher(root, cfg.Exclude)
		extensions := parseExtensions(cfg.Ext)
		guardrails := NewGuardrails(cfg.MaxFileSize, cfg.MaxFilesScan)
		examples := cfg.Examples
		if len(examples) == 0 {
			examples = defaultExampleFolders
		}

		filter, err := buildChangeFilter(ctx, root, cfg)
		if err != nil {
			return nil, err
		}

		targets := append(append([]string(nil), cfg.Targets...), cfg.Files...)
		if len(targets) == 0 {
			targets = []string{"."}
		}

		return func(yield func(ParsedFile, error) bool) {
			seen := make(map[string]bool, len(targets))
			for _, target := range targets {
				if err := ctx.Err(); err != nil {
					var zero ParsedFile
					yield(zero, err)
					return
				}
				path, startLine, endLine := ParseLineRange(target)
				materialized := cfg.TargetRoots[target] != ""
				if materialized {
					// A materialized target is a literal filename, not another selector.
					path, startLine, endLine = target, 0, 0
				}
				rawPath := path
				expandTree := false
				if !materialized {
					path, expandTree = stripTreeExpansion(path)
				}
				absSelectorPath, err := filepath.Abs(rawPath)
				if err != nil {
					var zero ParsedFile
					yield(zero, fmt.Errorf("resolve target %q: %w", target, err))
					return
				}
				absSelectorPath = filepath.Clean(absSelectorPath)
				absPath := absSelectorPath
				if expandTree {
					absPath, err = filepath.Abs(path)
					if err != nil {
						var zero ParsedFile
						yield(zero, fmt.Errorf("resolve target %q: %w", target, err))
						return
					}
					absPath = filepath.Clean(absPath)
				}
				profile := targetOption(cfg.TargetProfiles, absPath, absSelectorPath)
				prompt := targetOption(cfg.TargetPrompts, absPath, absSelectorPath)
				remoteRoot := cfg.TargetRoots[absPath]
				targetYield := yield
				if expandTree {
					previous := targetYield
					targetYield = func(file ParsedFile, err error) bool {
						if err == nil {
							file.ExpandTree = true
						}
						return previous(file, err)
					}
				}
				if label := cfg.TargetLabels[absPath]; label != "" {
					previous := targetYield
					targetYield = func(file ParsedFile, err error) bool {
						if err == nil {
							rel, relErr := filepath.Rel(remoteRoot, file.Path)
							if relErr != nil {
								return previous(ParsedFile{}, relErr)
							}
							file.RelPath = label + "/" + filepath.ToSlash(rel)
							file.SourceURL = remoteSourceURL(label, filepath.ToSlash(rel), file.SourceStartLine, file.SourceEndLine)
						}
						return previous(file, err)
					}
				}
				targetConfig := cfg
				if profile == "with-tests" && (cfg.NoTestsOverride == nil || !*cfg.NoTestsOverride) {
					targetConfig.NoTests = false
				}
				targetMatcher := matcher
				if targetExclude := targetOption(cfg.TargetExcludes, absPath, absSelectorPath); targetExclude != "" {
					excludes := cfg.Exclude
					if excludes != "" {
						excludes += ","
					}
					targetMatcher = buildIgnoreMatcher(root, excludes+targetExclude)
				}
				key := fmt.Sprintf("%s:%d-%d:%s:expand=%t", absPath, startLine, endLine, profile, expandTree)
				if seen[key] {
					continue
				}
				seen[key] = true

				info, err := os.Stat(absPath)
				if err != nil {
					var zero ParsedFile
					yield(zero, fmt.Errorf("target %q: %w", target, err))
					return
				}
				if info.IsDir() {
					// `.` keeps contents at the tree root. Any other target
					// — subfolder, sibling, or absolute — becomes the top-
					// level folder, matching `tree ../flow` → `flow/`.
					walkRoot := root
					if path != "." {
						walkRoot = filepath.Dir(absPath)
					}
					if remoteRoot != "" {
						walkRoot = remoteRoot
					}
					walkMatcher := targetMatcher
					if walkRoot != root {
						excludes := targetConfig.Exclude
						if extra := targetOption(cfg.TargetExcludes, absPath, absSelectorPath); extra != "" {
							if excludes != "" {
								excludes += ","
							}
							excludes += extra
						}
						walkMatcher = buildIgnoreMatcher(walkRoot, excludes)
					}

					walkFilters := sourceFileFilter{matcher: walkMatcher, extensions: extensions, examples: examples, guardrails: guardrails, changes: filter}
					repoRoot, repoFiles, inRepository, listErr := repositoryFiles.list(ctx, absPath)
					if listErr != nil {
						var zero ParsedFile
						yield(zero, listErr)
						return
					}
					if inRepository {
						readFile := func(path string) ([]byte, error) {
							return readRepositorySourceFile(ctx, repoRoot, path, guardrails.MaxFileSize)
						}
						for _, file := range repoFiles {
							p := filepath.Clean(filepath.Join(repoRoot, filepath.FromSlash(file.Path)))
							if !pathWithinRoot(absPath, p) {
								continue
							}
							entryInfo, statErr := os.Lstat(p)
							if statErr != nil {
								if os.IsNotExist(statErr) {
									continue
								}
								var zero ParsedFile
								yield(zero, fmt.Errorf("walk target %q: stat %q: %w", target, p, statErr))
								return
							}
							rel, relErr := filepath.Rel(walkRoot, p)
							if relErr != nil {
								var zero ParsedFile
								yield(zero, fmt.Errorf("walk target %q: %w", target, relErr))
								return
							}
							if !visitFile(ctx, p, dirEntryFromFileInfo(p, entryInfo), filepath.ToSlash(rel), targetConfig, walkFilters, profile, prompt, 0, 0, readFile, targetYield) {
								return
							}
						}
					} else {
						readFile := func(path string) ([]byte, error) {
							return readSourceFileBounded(path, guardrails.MaxFileSize)
						}
						var scanErr error
						walkErr := filepath.WalkDir(absPath, func(p string, d os.DirEntry, walkErr error) error {
							if ctx.Err() != nil {
								return ctx.Err()
							}
							if walkErr != nil {
								scanErr = walkErr
								return filepath.SkipAll
							}
							if d.IsDir() {
								return nil
							}
							rel, relErr := filepath.Rel(walkRoot, p)
							if relErr != nil {
								scanErr = relErr
								return filepath.SkipAll
							}
							if !visitFile(ctx, p, d, filepath.ToSlash(rel), targetConfig, walkFilters, profile, prompt, 0, 0, readFile, targetYield) {
								return filepath.SkipAll
							}
							return nil
						})
						if scanErr == nil {
							scanErr = walkErr
						}
						if scanErr != nil {
							var zero ParsedFile
							yield(zero, fmt.Errorf("walk target %q: %w", target, scanErr))
							return
						}
					}
					continue
				}

				fileRoot := root
				if path != "." {
					fileRoot = filepath.Dir(absPath)
				}
				if remoteRoot != "" {
					fileRoot = remoteRoot
				}
				rel, relErr := filepath.Rel(fileRoot, absPath)
				if relErr != nil {
					var zero ParsedFile
					yield(zero, relErr)
					return
				}
				fileMatcher := targetMatcher
				if fileRoot != root {
					excludes := targetConfig.Exclude
					if extra := targetOption(cfg.TargetExcludes, absPath, absSelectorPath); extra != "" {
						if excludes != "" {
							excludes += ","
						}
						excludes += extra
					}
					fileMatcher = buildIgnoreMatcher(fileRoot, excludes)
				}
				fileFilters := sourceFileFilter{matcher: fileMatcher, extensions: extensions, examples: examples, guardrails: guardrails, changes: filter}
				repoRoot, _, inRepository, listErr := repositoryFiles.list(ctx, filepath.Dir(absPath))
				if listErr != nil {
					var zero ParsedFile
					yield(zero, listErr)
					return
				}
				readFile := func(path string) ([]byte, error) {
					if inRepository {
						return readRepositorySourceFile(ctx, repoRoot, path, guardrails.MaxFileSize)
					}
					return readSourceFileBounded(path, guardrails.MaxFileSize)
				}
				if !visitFile(ctx, absPath, dirEntryFromFileInfo(absPath, info), filepath.ToSlash(rel), targetConfig, fileFilters, profile, prompt, startLine, endLine, readFile, targetYield) {
					return
				}
			}
		}, nil
	})
}

func visitFile(
	ctx context.Context,
	path string,
	entry os.DirEntry,
	relPath string,
	cfg PackConfig,
	filters sourceFileFilter,
	profile, prompt string,
	startLine, endLine int,
	readFile func(string) ([]byte, error),
	yield func(ParsedFile, error) bool,
) bool {
	info, include, keepGoing := filters.Decide(ctx, path, entry, relPath, cfg, yield)
	if !keepGoing || !include {
		return keepGoing
	}

	content, readErr := readFile(path)
	if readErr != nil {
		var zero ParsedFile
		return yield(zero, fmt.Errorf("read %q: %w", path, readErr))
	}

	fullLineCount := sourceLineCount(content)
	sourceStartLine, sourceEndLine := 1, fullLineCount
	sliced := false
	if startLine > 0 {
		content, sourceStartLine, sourceEndLine = sliceLinesWithSpan(content, startLine, endLine)
		sliced = sourceStartLine != 1 || sourceEndLine != fullLineCount
	}

	hash := computeHash(content)
	item := ParsedFile{
		Path:            path,
		RelPath:         relPath,
		Size:            info.Size(),
		ModTime:         info.ModTime(),
		Lang:            detectLang(path),
		Content:         content,
		Hash:            hash,
		Lines:           bytes.Count(content, []byte{'\n'}) + 1,
		SourceStartLine: sourceStartLine,
		SourceEndLine:   sourceEndLine,
		Sliced:          sliced,
		Profile:         profile,
		Prompt:          prompt,
		ParseOptions:    effectiveParseOptions(cfg, profile),
		ParseOptionsSet: true,
	}
	return yield(item, nil)
}

// sliceLinesWithSpan returns lines [start, end] (1-indexed, inclusive), plus
// their original line range. When end is 0 it runs to EOF.
func sliceLinesWithSpan(data []byte, start, end int) ([]byte, int, int) {
	lines := bytes.Split(data, []byte("\n"))
	lineCount := sourceLineCount(data)
	if start < 1 || start > lineCount {
		return data, 1, lineCount
	}
	from := start - 1
	to := end
	if to == 0 || to > len(lines) {
		to = len(lines)
	}
	if to < from {
		return data, 1, lineCount
	}
	return bytes.Join(lines[from:to], []byte("\n")), start, min(to, lineCount)
}

func skipReason(path, relPath string, cfg PackConfig, extensions map[string]bool, examples []string) string {
	base := strings.ToLower(filepath.Base(path))

	if isSensitiveSourceFile(relPath) {
		return "sensitive"
	}
	if binaryExtensions[strings.ToLower(filepath.Ext(path))] {
		return "binary"
	}
	if lockFiles[filepath.Base(path)] {
		return "binary"
	}
	if cfg.NoTests && isTestName(base) {
		return "test"
	}
	if cfg.NoGenerated && isGeneratedName(base) {
		return "generated"
	}
	if cfg.NoDeps && isDependencyPath(relPath) {
		return "deps"
	}
	if cfg.NoExamples && isExamplePath(relPath, examples) {
		return "examples"
	}
	if len(extensions) > 0 && !extensions[strings.ToLower(filepath.Ext(path))] {
		return "ext"
	}
	return ""
}

func isTestName(base string) bool {
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasSuffix(base, ".test.ts"), strings.HasSuffix(base, ".spec.ts"):
		return true
	case strings.HasSuffix(base, ".test.tsx"), strings.HasSuffix(base, ".spec.tsx"):
		return true
	case strings.HasSuffix(base, ".test.js"), strings.HasSuffix(base, ".spec.js"):
		return true
	case strings.HasSuffix(base, ".test.jsx"), strings.HasSuffix(base, ".spec.jsx"):
		return true
	case strings.HasSuffix(base, "_test.py"), strings.HasPrefix(base, "test_"):
		return true
	case strings.HasSuffix(base, "_test.rs"):
		return true
	}
	return false
}

func isGeneratedName(base string) bool {
	switch {
	case strings.HasSuffix(base, "_generated.go"),
		strings.HasSuffix(base, ".gen.go"),
		strings.HasSuffix(base, ".pb.go"),
		strings.HasSuffix(base, ".pb.ts"),
		strings.HasSuffix(base, "_generated.ts"):
		return true
	}
	return false
}

func isDependencyPath(relPath string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(relPath), "/") {
		switch strings.ToLower(part) {
		case "vendor", "node_modules", ".pnpm":
			return true
		}
	}
	return false
}

func isExamplePath(relPath string, examples []string) bool {
	clean := filepath.ToSlash(relPath)
	for part := range strings.SplitSeq(clean, "/") {
		for _, ex := range examples {
			if strings.EqualFold(part, ex) {
				return true
			}
		}
	}
	return false
}

func dirEntryFromFileInfo(path string, info os.FileInfo) os.DirEntry {
	return &fileInfoDirEntry{name: filepath.Base(path), info: info}
}

type fileInfoDirEntry struct {
	name string
	info os.FileInfo
}

func (e *fileInfoDirEntry) Name() string               { return e.name }
func (e *fileInfoDirEntry) IsDir() bool                { return e.info.IsDir() }
func (e *fileInfoDirEntry) Type() os.FileMode          { return e.info.Mode().Type() }
func (e *fileInfoDirEntry) Info() (os.FileInfo, error) { return e.info, nil }

func effectiveParseOptions(cfg PackConfig, profile string) ParseOptions {
	options := ParseOptions{
		OnlySig:        cfg.OnlySig,
		Dense:          cfg.Dense,
		Comments:       cfg.Comments,
		NeedFuncCount:  cfg.NeedFuncCount,
		Mask:           cfg.Mask,
		IncludeConfig:  cfg.IncludeConfig,
		MaxLines:       cfg.MaxLines,
		MaxLinesHead:   cfg.MaxLinesHead,
		MaxLinesTail:   cfg.MaxLinesTail,
		ExportedOnly:   cfg.ExportedOnly,
		TrimIndent:     cfg.TrimIndent,
		TrimBlankLines: cfg.TrimBlankLines,
	}
	switch profile {
	case "arch", "api", "dense":
		options.OnlySig = true
		options.Dense = true
		options.Comments = false
		options.ExportedOnly = true
	case "compact":
		options.OnlySig = false
		options.Dense = true
		options.Comments = false
		options.ExportedOnly = true
	case "impl", "source", "with-tests":
		options.OnlySig = false
		options.Dense = false
		options.Comments = true
		options.ExportedOnly = false
	}
	if cfg.DenseOverride != nil {
		options.Dense = *cfg.DenseOverride
	}
	if cfg.OnlySigOverride != nil {
		options.OnlySig = *cfg.OnlySigOverride
	}
	if cfg.CommentsOverride != nil {
		options.Comments = *cfg.CommentsOverride
	}
	return options
}

func parseExtensions(value string) map[string]bool {
	result := make(map[string]bool)
	for ext := range strings.SplitSeq(value, ",") {
		ext = strings.TrimSpace(strings.ToLower(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		result[ext] = true
	}
	return result
}

// ParseLineRange separates a local file selector's optional inclusive line range.
// A zero end means through EOF; invalid suffixes remain part of the path.
func ParseLineRange(target string) (path string, start, end int) {
	idx := strings.LastIndex(target, ":")
	if idx == -1 || idx == 1 {
		return target, 0, 0
	}
	rangeStr := target[idx+1:]
	if from, to, ok := strings.Cut(rangeStr, "-"); ok {
		if strings.Contains(to, "-") {
			return target, 0, 0
		}
		switch {
		case from == "":
			lastLine, err := strconv.Atoi(to)
			if err != nil || lastLine < 1 {
				return target, 0, 0
			}
			return target[:idx], 1, lastLine
		case to == "":
			firstLine, err := strconv.Atoi(from)
			if err != nil || firstLine < 1 {
				return target, 0, 0
			}
			return target[:idx], firstLine, 0
		default:
			firstLine, err := strconv.Atoi(from)
			if err != nil || firstLine < 1 {
				return target, 0, 0
			}
			lastLine, err := strconv.Atoi(to)
			if err != nil || lastLine < firstLine {
				return target, 0, 0
			}
			return target[:idx], firstLine, lastLine
		}
	}
	line, err := strconv.Atoi(rangeStr)
	if err != nil || line < 1 {
		return target, 0, 0
	}
	return target[:idx], line, line
}

func stripTreeExpansion(path string) (string, bool) {
	for _, suffix := range []string{"/...", `\...`} {
		if trimmed, ok := strings.CutSuffix(path, suffix); ok {
			return trimmed, true
		}
	}
	return path, false
}

// remoteSourceURL turns the stable label assigned during GitHub materialization
// into a commit-pinned citation. It deliberately returns empty for local labels.
func remoteSourceURL(label, relPath string, startLine, endLine int) string {
	if startLine < 1 || endLine < startLine || !strings.HasPrefix(label, "github/") {
		return ""
	}
	rest := strings.TrimPrefix(label, "github/")
	repo, commit, ok := strings.Cut(rest, "@")
	if !ok || repo == "" || commit == "" || relPath == "" {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return fmt.Sprintf("https://github.com/%s/blob/%s/%s#L%d-L%d", repo, url.PathEscape(commit), strings.Join(parts, "/"), startLine, endLine)
}

func targetOption(options map[string]string, path, selectorPath string) string {
	if value, ok := options[path]; ok {
		return value
	}
	return options[selectorPath]
}

func computeHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func detectLang(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".php":
		return "php"
	case ".zig":
		return "zig"
	case ".svelte":
		return "svelte"
	case ".vue":
		return "vue"
	case ".md":
		return editorMarkdown
	case ".nflow":
		return "nflow"
	default:
		return "text"
	}
}
