package nflows

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var sameNameFieldMapping = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)[\t\r\n ]*:[\t\r\n ]*\.([A-Za-z_][A-Za-z0-9_]*)`)

func TestNFlowFilesUseSameNameFieldShorthand(t *testing.T) {
	root, err := os.OpenRoot("..")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	err = filepath.WalkDir("..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".nflow" {
			return nil
		}

		relativePath, err := filepath.Rel("..", path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		source, err := root.ReadFile(relativePath)
		if err != nil {
			return err
		}
		code := maskNFlowStringsAndComments(source)
		for _, match := range sameNameFieldMapping.FindAllSubmatchIndex(code, -1) {
			key := string(code[match[2]:match[3]])
			field := string(code[match[4]:match[5]])
			if key != field || !mappingValueEndsHere(code, match[1]) {
				continue
			}
			mapping := string(source[match[0]:match[1]])
			line := strings.Count(string(source[:match[0]]), "\n") + 1
			t.Errorf("%s:%d: redundant %q mapping; use %q shorthand", relativePath, line, mapping, key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSameNameFieldShorthandCheckCoversProjectionAndActionArguments(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "projection",
			source: "@pipeline check\n  -> { field: .field }\n@end\n",
		},
		{
			name:   "action arguments",
			source: "@pipeline check\n  test.consume @{ field: .field }\n@end\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := countRedundantSameNameMappings([]byte(test.source)); got != 1 {
				t.Fatalf("explicit copy detected %d times; want 1", got)
			}
			shorthand := strings.Replace(test.source, "field: .field", "field", 1)
			if got := countRedundantSameNameMappings([]byte(shorthand)); got != 0 {
				t.Fatalf("shorthand reported %d times; want no redundant mappings", got)
			}
		})
	}
}

func countRedundantSameNameMappings(source []byte) int {
	code := maskNFlowStringsAndComments(source)
	count := 0
	for _, match := range sameNameFieldMapping.FindAllSubmatchIndex(code, -1) {
		key := string(code[match[2]:match[3]])
		field := string(code[match[4]:match[5]])
		if key == field && mappingValueEndsHere(code, match[1]) {
			count++
		}
	}
	return count
}

func mappingValueEndsHere(code []byte, end int) bool {
	for end < len(code) {
		switch code[end] {
		case ' ', '\t', '\r', '\n':
			end++
		default:
			return strings.ContainsRune(",})]", rune(code[end]))
		}
	}
	return true
}

// maskNFlowStringsAndComments preserves offsets and newlines while hiding text
// that cannot be a Flow map entry, so examples and comments do not trigger the
// style check.
func maskNFlowStringsAndComments(source []byte) []byte {
	code := append([]byte(nil), source...)
	var quote byte
	escaped := false
	lineComment := false
	blockComment := false
	linePrefixBlank := true

	for i := 0; i < len(source); i++ {
		c := source[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
				linePrefixBlank = true
			} else {
				code[i] = ' '
			}
			continue
		}
		if blockComment {
			if c == '*' && i+1 < len(source) && source[i+1] == '/' {
				code[i], code[i+1] = ' ', ' '
				blockComment = false
				i++
				continue
			}
			if c != '\n' {
				code[i] = ' '
			} else {
				linePrefixBlank = true
			}
			continue
		}
		if quote != 0 {
			if c != '\n' {
				code[i] = ' '
			}
			if quote == '`' {
				if c == quote {
					quote = 0
				}
			} else if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			if c == '\n' {
				linePrefixBlank = true
			}
			continue
		}

		if c == '"' || c == '\'' || c == '`' {
			quote = c
			code[i] = ' '
			linePrefixBlank = false
			continue
		}
		if c == '/' && i+1 < len(source) && source[i+1] == '*' {
			code[i], code[i+1] = ' ', ' '
			blockComment = true
			i++
			continue
		}
		if c == '/' && i+1 < len(source) && source[i+1] == '/' {
			code[i], code[i+1] = ' ', ' '
			lineComment = true
			i++
			continue
		}
		if c == '#' && linePrefixBlank {
			code[i] = ' '
			lineComment = true
			continue
		}
		if c == '\n' {
			linePrefixBlank = true
		} else if c != ' ' && c != '\t' && c != '\r' {
			linePrefixBlank = false
		}
	}
	return code
}
