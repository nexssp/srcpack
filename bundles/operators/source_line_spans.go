package operators

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// sourceLineSpans maps emitted body lines to original file coordinates. When
// the transformation preserves line positions, every nonblank line has an
// exact one-line origin. Otherwise, only content-preserving matches are mapped.
func sourceLineSpans(source, output []byte, sourceStartLine int, preserveLinePositions bool) []SourceLineSpan {
	if sourceStartLine < 1 {
		sourceStartLine = 1
	}
	sourceLines := physicalLines(source)
	outputLines := physicalLines(output)
	if preserveLinePositions && len(sourceLines) == len(outputLines) {
		spans := make([]SourceLineSpan, 0, len(outputLines))
		for i, line := range outputLines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			spans = append(spans, SourceLineSpan{
				OutputLine: i + 1, SourceStartLine: sourceStartLine + i, SourceEndLine: sourceStartLine + i,
			})
		}
		return spans
	}

	positions := make(map[string][]int, len(sourceLines))
	for index, line := range sourceLines {
		key := strings.TrimSpace(line)
		if key != "" {
			positions[key] = append(positions[key], index)
		}
	}

	var spans []SourceLineSpan
	lastSourceIndex := -1
	for outputIndex, line := range outputLines {
		want := strings.TrimSpace(line)
		if want == "" || isSyntheticSourceLine(want) {
			continue
		}
		matches := positions[want]
		if len(matches) != 1 || matches[0] <= lastSourceIndex {
			continue
		}
		sourceIndex := matches[0]
		lineNumber := sourceStartLine + sourceIndex
		spans = append(spans, SourceLineSpan{
			OutputLine: outputIndex + 1, SourceStartLine: lineNumber, SourceEndLine: lineNumber,
		})
		lastSourceIndex = sourceIndex
	}
	return spans
}

// sourceLineSpansForSnippets maps signature body lines to their enclosing Go
// declarations. A compact line can represent a multi-line declaration, so its
// origin is the declaration's original span rather than a guessed single line.
func sourceLineSpansForSnippets(snippets []string, symbols []SymbolSpan, lang string, dense bool) []SourceLineSpan {
	if len(snippets) == 0 || len(symbols) == 0 {
		return nil
	}

	var spans []SourceLineSpan
	outputLine := 1
	symbolIndex := 0
	written := false
	previousMultiline := false
	for _, raw := range snippets {
		sourceSnippet := strings.TrimSpace(raw)
		snippet := sourceSnippet
		if dense {
			snippet = strings.TrimSpace(DensifySnippets(lang, []string{snippet}))
		}
		if snippet == "" {
			continue
		}
		lines := physicalLines([]byte(snippet))
		if len(lines) == 0 {
			continue
		}
		if written {
			if dense || (!previousMultiline && len(lines) == 1) {
				outputLine++
			} else {
				outputLine += 2
			}
		}

		if symbol, nextIndex, ok := nextSymbolSpan(sourceSnippet, symbols, symbolIndex); ok {
			symbolIndex = nextIndex
			for i, line := range lines {
				if strings.TrimSpace(line) == "" || symbol.StartLine < 1 || symbol.EndLine < symbol.StartLine {
					continue
				}
				spans = append(spans, SourceLineSpan{
					OutputLine: outputLine + i, SourceStartLine: symbol.StartLine, SourceEndLine: symbol.EndLine,
				})
			}
		}

		outputLine += len(lines)
		written = true
		previousMultiline = len(lines) > 1
	}
	return spans
}

func nextSymbolSpan(snippet string, symbols []SymbolSpan, start int) (SymbolSpan, int, bool) {
	names := goSnippetNames(snippet)
	if len(names) == 0 {
		return SymbolSpan{}, start, false
	}
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	first, last := -1, -1
	var result SymbolSpan
	for i := start; i < len(symbols); i++ {
		if !wanted[symbols[i].Name] {
			continue
		}
		span := symbols[i]
		if first < 0 {
			first = i
			result = span
		} else {
			if span.StartLine < result.StartLine {
				result.StartLine = span.StartLine
			}
			if span.EndLine > result.EndLine {
				result.EndLine = span.EndLine
			}
		}
		last = i
		delete(wanted, span.Name)
	}
	if first < 0 {
		return SymbolSpan{}, start, false
	}
	return result, last + 1, true
}

func goSnippetNames(snippet string) []string {
	file, err := parser.ParseFile(token.NewFileSet(), "snippet.go", "package srcpack\n"+snippet, parser.ParseComments)
	if err != nil {
		return nil
	}
	var names []string
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			names = append(names, declaration.Name.Name)
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, spec.Name.Name)
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						names = append(names, name.Name)
					}
				}
			}
		}
	}
	return names
}

func physicalLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	lines := bytes.Split(content, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = string(line)
	}
	return result
}

func sourceLineCount(content []byte) int {
	if lines := physicalLines(content); len(lines) > 0 {
		return len(lines)
	}
	return 1
}

func isSyntheticSourceLine(line string) bool {
	switch line {
	case "// --- SIGNATURES ---", "// --- TRUNCATED MIDDLE ---":
		return true
	default:
		return false
	}
}
