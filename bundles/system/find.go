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
		b.WriteString("Try a broader name, --kind func|class|type|struct|enum|trait|pipeline|macro|heading, or --root PATH.\n")
	}
	return printed(b.String()), nil
}

func findDeclarations(ctx context.Context, root, query, kind string) ([]findMatch, error) {
	needle := strings.ToLower(query)
	var matches []findMatch
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", "_srcpack":
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !supportedFindExtension(ext) {
			return nil
		}
		if ext == ".go" {
			matches = append(matches, findGoDeclarations(path, root, needle, kind)...)
			return nil
		}
		found, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if ext == ".nflow" {
			matches = append(matches, findNFlowDeclarations(path, root, string(found), needle, kind)...)
		} else {
			matches = append(matches, findLanguageDeclarations(path, root, string(found), ext, needle, kind)...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("find declarations: %w", err)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].StartLine < matches[j].StartLine
	})
	return matches, nil
}

func findGoDeclarations(path, root, needle, kind string) []findMatch {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil
	}
	rel, _ := filepath.Rel(root, path)
	var matches []findMatch
	add := func(name, itemKind string, start, end token.Pos) {
		if kind != "" && kind != itemKind && !(kind == "function" && itemKind == "func") {
			return
		}
		if !strings.Contains(strings.ToLower(name), needle) {
			return
		}
		matches = append(matches, findMatch{Name: name, Kind: itemKind, Path: filepath.ToSlash(rel), StartLine: fset.Position(start).Line, EndLine: fset.Position(end).Line})
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

func findNFlowDeclarations(path, root, source, needle, kind string) []findMatch {
	rel, _ := filepath.Rel(root, path)
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
		matches = append(matches, findMatch{Name: name, Kind: declKind, Path: filepath.ToSlash(rel), StartLine: start, EndLine: end})
	}
	return matches
}

type findPattern struct {
	kind string
	re   *regexp.Regexp
}

var findPatterns = map[string][]findPattern{
	".ts": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+|async\s+)*function\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "class", re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "type", re: regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*(?:export\s+)?enum\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		{kind: "const", re: regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
	},
	".tsx": {}, ".js": {}, ".jsx": {}, ".mjs": {}, ".cjs": {},
	".py": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "class", re: regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".pyi": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "class", re: regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".rs": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "struct", re: regexp.MustCompile(`^\s*(?:pub\s+)?struct\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*(?:pub\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "trait", re: regexp.MustCompile(`^\s*(?:pub\s+)?trait\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "impl", re: regexp.MustCompile(`^\s*impl(?:<[^>]+>)?\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "const", re: regexp.MustCompile(`^\s*(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".php": {
		{kind: "func", re: regexp.MustCompile(`^\s*(?:public\s+|protected\s+|private\s+|static\s+|final\s+|abstract\s+)*function\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "class", re: regexp.MustCompile(`^\s*(?:abstract\s+|final\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "interface", re: regexp.MustCompile(`^\s*interface\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "trait", re: regexp.MustCompile(`^\s*trait\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*enum\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	},
	".zig": {
		{kind: "func", re: regexp.MustCompile(`^\s*pub\s+fn\s+([A-Za-z_][A-Za-z0-9_]*)|^\s*fn\s+([A-Za-z_][A-Za-z0-9_]*)`)},
		{kind: "struct", re: regexp.MustCompile(`^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*struct`)},
		{kind: "enum", re: regexp.MustCompile(`^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*enum`)},
		{kind: "const", re: regexp.MustCompile(`^\s*pub\s+const\s+([A-Za-z_][A-Za-z0-9_]*)|^\s*const\s+([A-Za-z_][A-Za-z0-9_]*)`)},
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

func supportedFindExtension(ext string) bool {
	if ext == ".go" || ext == ".nflow" || ext == ".md" || ext == ".mdx" || ext == ".markdown" {
		return true
	}
	_, ok := findPatterns[ext]
	return ok
}

func findLanguageDeclarations(path, root, source, ext, needle, kind string) []findMatch {
	patterns := findPatterns[ext]
	if ext == ".tsx" || ext == ".svelte" || ext == ".vue" {
		patterns = findPatterns[".ts"]
	}
	if ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs" {
		patterns = []findPattern{
			{kind: "func", re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+|async\s+)*function\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
			{kind: "class", re: regexp.MustCompile(`^\s*(?:export\s+default\s+|export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
			{kind: "const", re: regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)`)},
		}
	}
	lines := strings.Split(source, "\n")
	rel, _ := filepath.Rel(root, path)
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
			matches = append(matches, findMatch{Name: name, Kind: pattern.kind, Path: filepath.ToSlash(rel), StartLine: start, EndLine: declarationEnd(lines, start, ext)})
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
