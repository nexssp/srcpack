package terminal

import (
	"os"
	"strings"
)

// ColorEnabled follows the CLI color policy for the supplied output stream.
func ColorEnabled(mode string, output *os.File) bool {
	switch mode {
	case "never":
		return false
	case "always":
		return true
	}
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return false
	}
	if output == nil {
		return false
	}
	info, err := output.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// RenderMarkdown presents the cookbook for a terminal while leaving raw
// Markdown output to callers that explicitly request it.
func RenderMarkdown(markdown string, color bool) string {
	lines := strings.Split(markdown, "\n")
	var rendered strings.Builder
	inCodeBlock := false
	shellCode := false

	for i, line := range lines {
		content := line
		ending := ""
		if before, found := strings.CutSuffix(content, "\r"); found {
			content = before
			ending = "\r"
		}
		trimmed := strings.TrimSpace(content)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			if inCodeBlock {
				inCodeBlock = false
				shellCode = false
			} else {
				inCodeBlock = true
				language := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
				shellCode = language == "sh" || language == "bash" || language == "shell"
			}
			content = ""
		case inCodeBlock && shellCode:
			switch {
			case strings.HasPrefix(trimmed, "#"):
				content = paint(content, "2;37", color)
			case trimmed != "":
				content = paint(content, "32", color)
			}
		case !inCodeBlock:
			content = renderMarkdownLine(content, color)
		}

		rendered.WriteString(content)
		rendered.WriteString(ending)
		if i < len(lines)-1 {
			rendered.WriteByte('\n')
		}
	}
	return rendered.String()
}

func renderMarkdownLine(line string, color bool) string {
	trimmed := strings.TrimLeft(line, " \t")
	prefix := line[:len(line)-len(trimmed)]
	heading := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
	if heading > 0 && heading <= 6 && len(trimmed) > heading && trimmed[heading] == ' ' {
		style := "1;36"
		if heading == 2 {
			style = "1;34"
		} else if heading >= 3 {
			style = "1;35"
		}
		return prefix + paint(strings.TrimSpace(trimmed[heading:]), style, color)
	}
	return renderInlineCode(line, color)
}

func renderInlineCode(line string, color bool) string {
	var rendered strings.Builder
	for line != "" {
		start := strings.IndexByte(line, '`')
		if start < 0 {
			rendered.WriteString(line)
			break
		}
		rendered.WriteString(line[:start])
		code, remainder, found := strings.Cut(line[start+1:], "`")
		if !found {
			rendered.WriteByte('`')
			rendered.WriteString(remainder)
			break
		}
		rendered.WriteString(paint(code, "33", color))
		line = remainder
	}
	return rendered.String()
}
