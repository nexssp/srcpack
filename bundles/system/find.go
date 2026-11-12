package systemcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nexssp/srcpack/bundles/operators"
)

type findMatch struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

var nflowDeclaration = regexp.MustCompile(`(?m)^\s*@([A-Za-z][A-Za-z0-9_.-]*)\s+([A-Za-z][A-Za-z0-9_.-]*)`)

func find(ctx context.Context, in map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringValue(in["query"]))
	if query == "" {
		return printed("usage: srcpack find <name> [--root PATH] [--kind KIND] [--json]\n"), nil
	}
	root := stringValue(in["root"])
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("find root: %w", err)
	}
	if info, statErr := os.Stat(absRoot); statErr != nil {
		return nil, fmt.Errorf("find root %q: %w", root, statErr)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("find root %q is not a directory", root)
	}

	kind := strings.ToLower(strings.TrimSpace(stringValue(in["kind"])))
	matches, err := findDeclarations(ctx, absRoot, query, kind)
	if err != nil {
		return nil, err
	}
	if boolValue(in["json"]) {
		data, marshalErr := json.MarshalIndent(map[string]any{
			"query": query, "root": root, "matches": matches,
		}, "", "  ")
		if marshalErr != nil {
			return nil, marshalErr
		}
		return printed(string(data) + "\n"), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d match(es) for %q\n", len(matches), query)
	for _, match := range matches {
		fmt.Fprintf(&b, "  %-7s %-32s %s:%d-%d\n", match.Kind, match.Name, match.Path, match.StartLine, match.EndLine)
	}
	if len(matches) == 0 {
		b.WriteString("Try a broader name, --kind func|class|type|struct|enum|trait|pipeline|macro|heading|text, or --root PATH.\n")
	}
	return printed(b.String()), nil
}

func findDeclarations(ctx context.Context, root, query, kind string) ([]findMatch, error) {
	needle := strings.ToLower(query)
	policy := operators.NewSourceFilePolicy(root, operators.PackConfig{})
	guardrails := operators.NewGuardrails(0, 0)
	sourceRoot, rootErr := os.OpenRoot(root)
	if rootErr != nil {
		return nil, fmt.Errorf("open find root %q: %w", root, rootErr)
	}
	var matches []findMatch
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relPath = filepath.ToSlash(relPath)
		if !policy.Includes(path, relPath) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if err := guardrails.CheckFile(path, info); err != nil {
			return err
		}
		found, readErr := sourceRoot.ReadFile(filepath.FromSlash(relPath))
		if readErr != nil {
			return readErr
		}
		source := string(found)
		if !utf8.ValidString(source) || strings.IndexByte(source, 0) >= 0 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		matches = append(matches, findFileMatches(path, relPath, source, ext, needle, kind)...)
		return nil
	})
	closeErr := sourceRoot.Close()
	if walkErr != nil {
		return nil, fmt.Errorf("find declarations: %w", walkErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close find root: %w", closeErr)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].StartLine < matches[j].StartLine
	})
	return matches, nil
}

func findFileMatches(path, relPath, source, ext, needle, kind string) []findMatch {
	if kind == "text" {
		return findTextMatches(relPath, source, needle)
	}

	var matches []findMatch
	switch ext {
	case ".go":
		matches = findGoDeclarations(path, relPath, source, needle, kind)
	case ".nflow":
		matches = findNFlowDeclarations(relPath, source, needle, kind)
	default:
		if _, ok := findPatterns[ext]; ok {
			matches = findLanguageDeclarations(relPath, source, ext, needle, kind)
		}
	}
	if kind != "" {
		return matches
	}

	declarationLines := make(map[int]struct{}, len(matches))
	for _, match := range matches {
		declarationLines[match.StartLine] = struct{}{}
	}
	for _, match := range findTextMatches(relPath, source, needle) {
		if _, isDeclaration := declarationLines[match.StartLine]; !isDeclaration {
			matches = append(matches, match)
		}
	}
	return matches
}

func findTextMatches(relPath, source, needle string) []findMatch {
	var matches []findMatch
	for index, line := range strings.Split(source, "\n") {
		if !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		matches = append(matches, findMatch{
			Name:      strings.TrimSpace(line),
			Kind:      "text",
			Path:      relPath,
			StartLine: index + 1,
			EndLine:   index + 1,
		})
	}
	return matches
}

func findGoDeclarations(path, relPath, source, needle, kind string) []findMatch {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		return nil
	}
	var matches []findMatch
	add := func(name, itemKind string, start, end token.Pos) {
		if !kindMatches(kind, itemKind) {
			return
		}
		if !strings.Contains(strings.ToLower(name), needle) {
			return
		}
		matches = append(matches, findMatch{Name: name, Kind: itemKind, Path: filepath.ToSlash(relPath), StartLine: fset.Position(start).Line, EndLine: fset.Position(end).Line})
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			add(decl.Name.Name, "func", decl.Pos(), decl.End())
		case *ast.GenDecl:
			itemKind := strings.ToLower(decl.Tok.String())
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					add(spec.Name.Name, "type", spec.Pos(), spec.End())
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						add(name.Name, itemKind, spec.Pos(), spec.End())
					}
				}
			}
		}
	}
	return matches
}

func findNFlowDeclarations(relPath, source, needle, kind string) []findMatch {
	lines := strings.Split(source, "\n")
	var matches []findMatch
	for _, match := range nflowDeclaration.FindAllStringSubmatchIndex(source, -1) {
		declKind := strings.ToLower(source[match[2]:match[3]])
		name := source[match[4]:match[5]]
		if kind != "" && kind != declKind {
			continue
		}
		if !strings.Contains(strings.ToLower(name), needle) {
			continue
		}
		start := 1 + strings.Count(source[:match[0]], "\n")
		end := start
		for i := start; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "@end") || (declKind == "require" && i == start) {
				end = i + 1
				break
			}
			end = i + 1
		}
		matches = append(matches, findMatch{Name: name, Kind: declKind, Path: filepath.ToSlash(relPath), StartLine: start, EndLine: end})
	}
	return matches
}

type findPattern struct {
	kind string
	re   *regexp.Regexp
}

const (
	findKindClass = "class"
	findKindConst = "const"
)

var findPatterns = map[string][]findPattern{
	".ts": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+|async\s+)*function\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: findKindClass, re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "type", re: regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*(?:export\s+)?enum\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: findKindConst, re: regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
	},
	".tsx": {}, ".js": {}, ".jsx": {}, ".mjs": {}, ".cjs": {},
	".py": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: findKindClass, re: regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".pyi": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: findKindClass, re: regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".rs": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "struct", re: regexp.MustCompile(`^\s*(?:pub\s+)?struct\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*(?:pub\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "trait", re: regexp.MustCompile(`^\s*(?:pub\s+)?trait\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "impl", re: regexp.MustCompile(`^\s*impl(?:<[^>]+>)?\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: findKindConst, re: regexp.MustCompile(`^\s*(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".php": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:public\s+|protected\s+|private\s+|static\s+|final\s+|abstract\s+)*function\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: findKindClass, re: regexp.MustCompile(`^\s*(?:abstract\s+|final\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*interface\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "trait", re: regexp.MustCompile(`^\s*trait\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*enum\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".zig": {
		{kind: "func", re: regexp.MustCompile(`^\s*pub\s+fn\s+([A-Za-z_][A-Za-z0-9_]*)|^\s*fn\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "struct", re: regexp.MustCompile(`^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*struct`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*enum`)},
		{kind: findKindConst, re: regexp.MustCompile(`^\s*pub\s+const\s+([A-Za-z_][A-Za-z0-9_]*)|^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "var", re: regexp.MustCompile(`^\s*pub\s+var\s+([A-Za-z_][A-Za-z0-9_]*)|^\s*var\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".md":       {{kind: "heading", re: regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)}},
	".mdx":      {{kind: "heading", re: regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)}},
	".markdown": {{kind: "heading", re: regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*$`)}},
	".svelte":   {}, ".vue": {},
	".html": {
		{kind: "element", re: regexp.MustCompile(`<([A-Z][A-Za-z0-9_.-]*)\b`)},
	},
	".htm": {
		{kind: "element", re: regexp.MustCompile(`<([A-Z][A-Za-z0-9_.-]*)\b`)},
	},
	".css": {
		{kind: "selector", re: regexp.MustCompile(`^\s*([.#]?[A-Za-z_][A-Za-z0-9_-]*)\s*\{`)},
		{kind: "variable", re: regexp.MustCompile(`^\s*(--[A-Za-z_][A-Za-z0-9_-]*)\s*:`)},
	},
	".scss": {
		{kind: "selector", re: regexp.MustCompile(`^\s*([.#]?[A-Za-z_][A-Za-z0-9_-]*)\s*\{`)},
		{kind: "variable", re: regexp.MustCompile(`^\s*(\$[A-Za-z_][A-Za-z0-9_-]*)\s*:`)},
	},
}

func findLanguageDeclarations(relPath, source, ext, needle, kind string) []findMatch {
	patterns := findPatterns[ext]
	if ext == ".tsx" || ext == ".svelte" || ext == ".vue" {
		patterns = findPatterns[".ts"]
	}
	if ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs" {
		patterns = []findPattern{
			{kind: "func", re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+|async\s+)*function\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
			{kind: findKindClass, re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
			{kind: findKindConst, re: regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		}
	}
	lines := strings.Split(source, "\n")
	var matches []findMatch
	for index, line := range lines {
		for _, pattern := range patterns {
			groups := pattern.re.FindStringSubmatch(line)
			if len(groups) < 2 {
				continue
			}
			name := groups[1]
			if name == "" && len(groups) > 2 {
				name = groups[2]
			}
			if !kindMatches(kind, pattern.kind) || !strings.Contains(strings.ToLower(name), needle) {
				continue
			}
			start := index + 1
			matches = append(matches, findMatch{Name: name, Kind: pattern.kind, Path: filepath.ToSlash(relPath), StartLine: start, EndLine: declarationEnd(lines, start, ext)})
			break
		}
	}
	return matches
}

func kindMatches(want, got string) bool {
	if want == "" || want == got {
		return true
	}
	return want == "function" && got == "func"
}

func declarationEnd(lines []string, start int, ext string) int {
	if start < 1 || start > len(lines) {
		return start
	}
	line := lines[start-1]
	if ext == ".py" {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		for i := start; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			currentIndent := len(lines[i]) - len(strings.TrimLeft(lines[i], " \t"))
			if trimmed != "" && currentIndent <= indent && !strings.HasPrefix(trimmed, "#") {
				return i
			}
		}
		return len(lines)
	}
	if !strings.Contains(line, "{") {
		return start
	}
	depth := 0
	for i := start - 1; i < len(lines); i++ {
		depth += strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
		if depth <= 0 {
			return i + 1
		}
	}
	return len(lines)
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func boolValue(value any) bool {
	flag, _ := value.(bool)
	return flag
}
