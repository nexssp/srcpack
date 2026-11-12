package operators

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	outputPreviewMaxBytes = 1536
	outputPreviewMaxLines = 10
)

// boundedOutputPreview limits only the terminal preview; output actions keep
// receiving the complete package string for their normal destination handling.
func boundedOutputPreview(content string) (string, bool) {
	end := len(content)
	truncated := false
	if end > outputPreviewMaxBytes {
		end = outputPreviewMaxBytes
		for end > 0 && end < len(content) && !utf8.RuneStart(content[end]) {
			end--
		}
		truncated = true
	}

	lines := 0
	for i := 0; i < end; i++ {
		if content[i] != '\n' {
			continue
		}
		lines++
		if lines >= outputPreviewMaxLines && i+1 < len(content) {
			end = i + 1
			truncated = true
			break
		}
	}
	if end < len(content) {
		truncated = true
	}

	preview := strings.TrimRight(content[:end], "\r\n")
	return sanitizeTerminalPreview(preview), truncated
}

// sanitizeTerminalPreview replaces control characters in the display-only
// preview so untrusted package text cannot manipulate the terminal.
func sanitizeTerminalPreview(text string) string {
	var clean strings.Builder
	clean.Grow(len(text))
	for _, r := range text {
		if r != '\n' && unicode.IsControl(r) {
			clean.WriteRune('\uFFFD')
			continue
		}
		clean.WriteRune(r)
	}
	return clean.String()
}
