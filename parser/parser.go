// Package parser preserves srcpack's historical parser API while delegating
// source extraction and cleanup to nexssp/lang.
package parser

import (
	"bytes"

	"github.com/nexssp/lang/extract"
)

// Options controls language extraction and source cleanup.
type Options = extract.Options

// Result contains extracted declaration snippets, function metadata, or
// processed content when signatures are unavailable.
type Result = extract.Result

// Parse delegates language-specific source extraction to nexssp/lang.
func Parse(path string, src []byte, lang string, opts Options) (Result, error) {
	return extract.Parse(path, src, lang, opts)
}

// CleanSource applies source cleanup using the shared language implementation.
func CleanSource(src []byte, opts Options, out *bytes.Buffer) {
	extract.CleanSource(src, opts, out)
}

// ProcessContent applies the requested cleanup to full source text.
func ProcessContent(src string, opts Options) string {
	return extract.ProcessContent(src, opts)
}

// StripComments removes comments while preserving string literals.
func StripComments(src string) string {
	return extract.StripComments(src)
}

// TrimIndent removes leading indentation from every line.
func TrimIndent(src string) string {
	return extract.TrimIndent(src)
}

// TrimBlankLines collapses runs of blank lines.
func TrimBlankLines(src string) string {
	return extract.TrimBlankLines(src)
}

// Truncate keeps configured head and tail lines and marks omitted content.
func Truncate(src []byte, opts Options, snippets []string) string {
	return extract.Truncate(src, opts, snippets)
}
