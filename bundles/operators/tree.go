package operators

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nexssp/srcpack/bundles/config"
	"github.com/nexssp/srcpack/bundles/terminal"
)

// treeNode is one directory or file in the rendered tree. Only leaves
// carry per-file metadata; directories roll up their descendants.
type treeNode struct {
	name       string
	isDir      bool
	funcs      int
	tokens     int64
	lang       string
	lines      int
	role       string
	ageDays    int
	paired     string
	isExcluded bool
	isTarget   bool
	children   map[string]*treeNode
}

func newTreeNode(name string, isDir bool) *treeNode {
	return &treeNode{
		name:     name,
		isDir:    isDir,
		ageDays:  -1,
		children: map[string]*treeNode{},
	}
}

func (n *treeNode) rollup() (files int, tokens int64, funcs int, langs map[string]int) {
	langs = map[string]int{}
	for _, child := range n.children {
		if child.isDir {
			f, t, fn, l := child.rollup()
			files += f
			tokens += t
			funcs += fn
			for k, v := range l {
				langs[k] += v
			}
		} else {
			files++
			tokens += child.tokens
			funcs += child.funcs
			if child.lang != "" {
				langs[child.lang]++
			}
		}
	}
	return
}

func (n *treeNode) isFullyExcluded() bool {
	if !n.isDir {
		return n.isExcluded
	}
	if len(n.children) == 0 {
		return false
	}
	for _, child := range n.children {
		if child.isDir {
			if !child.isFullyExcluded() {
				return false
			}
		} else if !child.isExcluded {
			return false
		}
	}
	return true
}

// ── classification ───────────────────────────────────────────────────

var (
	testNamePrefixes = []string{"test_"}
	testSuffixes     = []string{
		"_test.go",
		".test.ts", ".spec.ts",
		".test.tsx", ".spec.tsx",
		".test.js", ".spec.js",
		".test.jsx", ".spec.jsx",
		"_test.py",
	}
	configFilenames = map[string]bool{
		"dockerfile":     true,
		"containerfile":  true,
		"makefile":       true,
		"cmakelists.txt": true,
		".gitignore":     true,
		".gitattributes": true,
	}
	configSuffixes = []string{".yml", ".yaml", ".toml", ".ini"}
	docFilenames   = map[string]bool{"license": true}
	docSuffixes    = []string{".md", ".mdx", ".rst"}
	genSuffixes    = []string{".pb.go", "_generated.go", "_generated.ts"}
	genPathParts   = []string{"/gen/", "/generated/"}
	entryNames     = map[string]bool{
		"main.go":  true,
		"index.ts": true,
		"index.js": true,
		"main.rs":  true,
		"main.py":  true,
	}
)

func classifyFile(name, relPath string) string {
	lower := strings.ToLower(name)
	relSlash := filepath.ToSlash(relPath)

	switch {
	case isTestFile(lower):
		return "test"
	case configFilenames[lower] || hasAnySuffix(lower, configSuffixes):
		return "config"
	case docFilenames[lower] || hasAnySuffix(lower, docSuffixes):
		return "doc"
	case hasAnySuffix(lower, genSuffixes) || containsAny(relSlash, genPathParts):
		return "gen"
	case entryNames[lower]:
		return "entry"
	}
	return ""
}

func isTestFile(lower string) bool {
	return hasAnyPrefix(lower, testNamePrefixes) || hasAnySuffix(lower, testSuffixes)
}

func hasAnySuffix(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func containsAny(s string, parts []string) bool {
	for _, part := range parts {
		if strings.Contains(s, part) {
			return true
		}
	}
	return false
}

func dominantLang(langs map[string]int) string {
	best, bestN := "", 0
	for lang, n := range langs {
		if n > bestN || (n == bestN && n > 0 && (best == "" || lang < best)) {
			best, bestN = lang, n
		}
	}
	return best
}

// ── population ───────────────────────────────────────────────────────

// treeVisibleReason matches the whitelist in source_op.go.
func treeVisibleReason(reason string) bool {
	switch reason {
	case "test", roleExamples, "generated", "ext":
		return true
	}
	return false
}

func populateLeaf(node *treeNode, file *ParsedFile) {
	node.funcs = file.FuncCount
	node.lang = file.Lang
	node.lines = file.Lines
	node.role = classifyFile(filepath.Base(file.RelPath), file.RelPath)
	node.isExcluded = false

	processedSize := int64(len(file.Content))
	if processedSize == 0 {
		for _, s := range file.Snippets {
			processedSize += int64(len(s))
		}
	}
	if processedSize == 0 && file.Size > 0 {
		processedSize = file.Size
	}
	node.tokens = int64(float64(processedSize) / 3.5)

	if !file.ModTime.IsZero() {
		node.ageDays = int(time.Since(file.ModTime).Hours() / 24)
	}
}

func insertOmitted(root *treeNode, file *ParsedFile) {
	if !treeVisibleReason(file.OmitReason) {
		return
	}
	parts := strings.Split(filepath.ToSlash(file.RelPath), "/")
	current := root
	for i, part := range parts {
		isLast := i == len(parts)-1
		child, ok := current.children[part]
		if !ok {
			child = newTreeNode(part, !isLast)
			current.children[part] = child
		}
		current = child
		if isLast && !current.isDir && current.tokens == 0 && !current.isExcluded {
			current.isExcluded = true
			current.lang = file.Lang
			current.role = classifyFile(part, file.RelPath)
		}
	}
}

// ── test pairing ─────────────────────────────────────────────────────

func stripTestSuffix(name string) string {
	switch {
	case strings.HasSuffix(name, "_test.go"):
		return strings.TrimSuffix(name, "_test.go") + ".go"
	case strings.HasSuffix(name, ".test.ts"):
		return strings.TrimSuffix(name, ".test.ts") + ".ts"
	case strings.HasSuffix(name, ".spec.ts"):
		return strings.TrimSuffix(name, ".spec.ts") + ".ts"
	case strings.HasSuffix(name, ".test.tsx"):
		return strings.TrimSuffix(name, ".test.tsx") + ".tsx"
	case strings.HasSuffix(name, ".spec.tsx"):
		return strings.TrimSuffix(name, ".spec.tsx") + ".tsx"
	case strings.HasSuffix(name, ".test.js"):
		return strings.TrimSuffix(name, ".test.js") + ".js"
	case strings.HasSuffix(name, ".spec.js"):
		return strings.TrimSuffix(name, ".spec.js") + ".js"
	case strings.HasSuffix(name, "_test.py"):
		return strings.TrimSuffix(name, "_test.py") + ".py"
	case strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py"):
		return strings.TrimPrefix(name, "test_")
	}
	return ""
}

func pairTests(root *treeNode) {
	byDir := map[string]map[string]*treeNode{}

	var walk func(n *treeNode, path string)
	walk = func(n *treeNode, path string) {
		if !n.isDir {
			if byDir[path] == nil {
				byDir[path] = map[string]*treeNode{}
			}
			byDir[path][n.name] = n
			return
		}
		var childPath string
		if path == "" {
			childPath = n.name
		} else {
			childPath = path + "/" + n.name
		}
		for _, child := range n.children {
			walk(child, childPath)
		}
	}
	walk(root, "")

	for _, files := range byDir {
		for name, node := range files {
			if node.role != "test" {
				continue
			}
			src := stripTestSuffix(name)
			if src == "" {
				continue
			}
			if srcNode, ok := files[src]; ok {
				node.paired = src
				srcNode.paired = name
			}
		}
	}
}

func sortChildren(children map[string]*treeNode) []*treeNode {
	entries := make([]*treeNode, 0, len(children))
	for _, node := range children {
		entries = append(entries, node)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.isDir != b.isDir {
			return a.isDir
		}
		if a.isDir {
			_, tokensA, _, _ := a.rollup()
			_, tokensB, _, _ := b.rollup()
			if tokensA != tokensB {
				return tokensA > tokensB
			}
		}
		return a.name < b.name
	})
	return entries
}

const (
	treePackedBytesField = "packed_bytes"
	treeRawBytesField    = "raw_bytes"
)

// generateContextTree collects tree facts for the terminal NFlow renderer.
func generateContextTree(files []ParsedFile, maxDepth int, color, showFiles bool) (string, error) {
	return generateContextTreeContext(context.Background(), files, maxDepth, color, showFiles)
}

//nolint:contextcheck // A nil parent has no inherited state; retain the nil-context fallback.
func generateContextTreeContext(ctx context.Context, files []ParsedFile, maxDepth int, color, showFiles bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	input := map[string]any{"empty": len(files) == 0, "full": false, "included": 0, "excluded": 0, "total_files": 0, "total_tokens": int64(0), treeRawBytesField: int64(0), treePackedBytesField: int64(0), "color": color, "languages": map[string]int{}, "rows": []map[string]any{}}
	if len(files) == 0 {
		return "", nil
	}
	maxDepth = config.ResolveTreeDepth(maxDepth)
	root, err := buildContextTree(ctx, files)
	if err != nil {
		return "", err
	}
	stats, err := collectContextTreeStats(ctx, files)
	if err != nil {
		return "", err
	}
	_, _, _, langs := root.rollup()
	input["included"], input["excluded"], input["total_files"] = stats.included, stats.excluded, stats.included+stats.excluded
	input["total_tokens"], input["raw_bytes"], input["packed_bytes"] = stats.totalTokens, stats.rawBytes, stats.packedBytes
	input["languages"] = langs
	rows := []map[string]any{}
	if err := appendTreeRows(ctx, root, "", 1, maxDepth, showFiles, &rows); err != nil {
		return "", err
	}
	input["rows"] = rows
	return terminal.Tree(ctx, input)
}

func buildContextTree(ctx context.Context, files []ParsedFile) (*treeNode, error) {
	root := newTreeNode(".", true)
	for i := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if files[i].Omitted {
			insertOmitted(root, &files[i])
		}
	}
	for i := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file := &files[i]
		if file.Omitted || (len(file.Content) == 0 && len(file.Snippets) == 0) {
			continue
		}
		parts := strings.Split(filepath.ToSlash(file.RelPath), "/")
		current := root
		for j, part := range parts {
			last := j == len(parts)-1
			if current.children[part] == nil {
				current.children[part] = newTreeNode(part, !last)
			}
			current = current.children[part]
			if last {
				populateLeaf(current, file)
			}
		}
	}
	pairTests(root)
	return root, nil
}

type contextTreeStats struct {
	included, excluded                 int
	totalTokens, rawBytes, packedBytes int64
}

func collectContextTreeStats(ctx context.Context, files []ParsedFile) (contextTreeStats, error) {
	var stats contextTreeStats
	for i := range files {
		if err := ctx.Err(); err != nil {
			return contextTreeStats{}, err
		}
		file := &files[i]
		if file.Omitted {
			stats.excluded++
			continue
		}
		if len(file.Content) == 0 && len(file.Snippets) == 0 {
			continue
		}
		stats.included++
		stats.rawBytes += file.Size
		packed := int64(len(file.Content))
		if packed == 0 {
			for _, snippet := range file.Snippets {
				packed += int64(len(snippet))
			}
		}
		stats.packedBytes += packed
		stats.totalTokens += int64(float64(packed) / 3.5)
	}
	return stats, nil
}

// GenerateContextTree is the compatibility string API.
func GenerateContextTree(files []ParsedFile, depth int, color, showFiles bool) string {
	result, err := generateContextTree(files, depth, color, showFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "srcpack: tree: %v\n", err)
		return ""
	}
	return result
}

func appendTreeRows(ctx context.Context, node *treeNode, prefix string, depth, maxDepth int, showFiles bool, rows *[]map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	children := sortChildren(node.children)
	visible := make([]*treeNode, 0, len(children))
	for _, c := range children {
		if c.isDir || showFiles {
			visible = append(visible, c)
		}
	}
	for i, c := range visible {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := i == len(visible)-1
		connector, subPrefix := "├── ", "│   "
		if last {
			connector, subPrefix = "└── ", "    "
		}
		row := map[string]any{"name": c.name, "dir": c.isDir, "prefix": prefix, "connector": connector, "files": 0, "tokens": int64(0), "funcs": 0, "lang": "", "fully_excluded": false, "role": "", "age": -1, "lines": 0, "paired": "", "excluded": false, "target": false}
		if c.isDir {
			f, t, fn, l := c.rollup()
			row["files"], row["tokens"], row["funcs"], row["lang"], row["fully_excluded"] = f, t, fn, dominantLang(l), c.isFullyExcluded()
		} else {
			age := c.ageDays
			row["tokens"], row["funcs"], row["lang"], row["role"], row["age"], row["lines"], row["paired"], row["excluded"], row["target"] = c.tokens, c.funcs, c.lang, c.role, age, c.lines, c.paired, c.isExcluded, c.isTarget
		}
		*rows = append(*rows, row)
		if c.isDir && depth < maxDepth {
			if err := appendTreeRows(ctx, c, prefix+subPrefix, depth+1, maxDepth, showFiles, rows); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateFullProjectTree(projectRoot string, files []ParsedFile, maxDepth int, color, showFiles bool) (string, error) {
	return generateFullProjectTreeContext(context.Background(), projectRoot, files, maxDepth, color, showFiles)
}

//nolint:contextcheck // A nil parent has no inherited state; retain the nil-context fallback.
func generateFullProjectTreeContext(ctx context.Context, projectRoot string, files []ParsedFile, maxDepth int, color, showFiles bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if projectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			// Preserve the legacy best-effort overview when no root is available.
			//nolint:nilerr // A missing working directory intentionally yields an empty overview.
			return "", nil
		}
		projectRoot = wd
	}
	maxDepth = config.ResolveTreeDepth(maxDepth)
	included := make(map[string]bool, len(files))
	for i := range files {
		if !files[i].Omitted {
			included[filepath.ToSlash(files[i].RelPath)] = true
		}
	}
	root, totalFiles, totalTokens, walkErr := collectFullProjectTree(ctx, projectRoot, included)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	pairTests(root)
	excluded := totalFiles - len(included)
	_, _, _, langs := root.rollup()
	rows := []map[string]any{}
	if err := appendTreeRows(ctx, root, "", 1, maxDepth, showFiles, &rows); err != nil {
		return "", err
	}
	input := map[string]any{"empty": false, "full": true, "included": len(included), "excluded": excluded, "total_files": totalFiles, "total_tokens": totalTokens, "raw_bytes": totalTokens, "packed_bytes": totalTokens, "color": color, "languages": langs, "rows": rows}
	result, err := terminal.Tree(ctx, input)
	if err != nil {
		return "", err
	}
	if walkErr != nil {
		fmt.Fprintf(os.Stderr, "srcpack: tree: %v\n", walkErr)
	}
	return result, nil
}

func collectFullProjectTree(ctx context.Context, projectRoot string, included map[string]bool) (*treeNode, int, int64, error) {
	root := newTreeNode(".", true)
	totalFiles := 0
	var totalTokens int64
	walkErr := filepath.WalkDir(projectRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			//nolint:nilerr // Keep the legacy best-effort tree by skipping unreadable entries.
			return nil
		}
		rel, err := filepath.Rel(projectRoot, path)
		if err != nil || rel == "." {
			//nolint:nilerr // Unrelatable paths are omitted from this best-effort overview.
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if entry.IsDir() {
			name := entry.Name()
			if strings.HasPrefix(name, ".") || isHeavyDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		current := root
		parts := strings.Split(relSlash, "/")
		for i, part := range parts {
			last := i == len(parts)-1
			if current.children[part] == nil {
				current.children[part] = newTreeNode(part, !last)
			}
			current = current.children[part]
			if last {
				totalFiles++
				current.lang = detectLang(relSlash)
				current.role = classifyFile(part, relSlash)
				current.isTarget = included[relSlash]
				current.isExcluded = !current.isTarget
				if info, infoErr := entry.Info(); infoErr == nil {
					current.tokens = int64(float64(info.Size()) / 3.5)
					current.ageDays = int(time.Since(info.ModTime()).Hours() / 24)
				}
				totalTokens += current.tokens
			}
		}
		return nil
	})
	return root, totalFiles, totalTokens, walkErr
}

func GenerateFullProjectTree(root string, files []ParsedFile, depth int, color, showFiles bool) string {
	result, err := generateFullProjectTree(root, files, depth, color, showFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "srcpack: tree: %v\n", err)
		return ""
	}
	return result
}

func isHeavyDir(name string) bool {
	switch name {
	case "node_modules", "vendor", ".pnpm",
		"dist", "build", "out", "target",
		".next", ".nuxt", ".output", ".turbo", ".cache", ".parcel-cache",
		"__pycache__", "_srcpack":
		return true
	}
	return false
}
