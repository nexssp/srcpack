package operators

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	goFuncRe = regexp.MustCompile(`^func\s+(?:\(([^)]*)\)\s+)?([A-Za-z0-9_]+)\s*\(([^)]*)\)\s*(.*)$`)
	tsFuncRe = regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z0-9_]+)\s*\(([^)]*)\)\s*(?::\s*([^{]+))?`)
)

func DensifySnippets(lang string, snippets []string) string {
	if len(snippets) == 0 {
		return ""
	}

	out := make([]string, 0, len(snippets))
	for _, s := range snippets {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		switch lang {
		case "go":
			if d := denseGo(s); d != "" {
				out = append(out, d)
			}
		case "typescript", "tsx", "javascript", "jsx", "vue", "svelte":
			if d := denseTS(s); d != "" {
				out = append(out, d)
			}
		case "rust":
			if d := denseRust(s); d != "" {
				out = append(out, d)
			}
		default:
			out = append(out, s)
		}
	}

	return strings.Join(out, "\n")
}

func DensifyBody(content string) string {
	lines := strings.Split(content, "\n")
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}

	joined := strings.Join(out, "\n")

	joined = strings.ReplaceAll(joined, "{\n", "{")
	joined = strings.ReplaceAll(joined, "\n}", "}")
	joined = strings.ReplaceAll(joined, ",\n", ",")
	joined = strings.ReplaceAll(joined, "(\n", "(")
	joined = strings.ReplaceAll(joined, "\n)", ")")

	return joined
}

func denseGo(s string) string {
	if strings.HasPrefix(s, "func ") {
		m := goFuncRe.FindStringSubmatch(s)
		if m == nil {
			return s
		}

		recv := strings.TrimSpace(m[1])
		name := strings.TrimSpace(m[2])
		args := normalizeArgs(m[3])
		ret := normalizeReturns(m[4])

		if recv != "" {
			recvType := receiverType(recv)
			if recvType != "" {
				return fmt.Sprintf("%s.%s(%s)%s", recvType, name, args, ret)
			}
		}
		return fmt.Sprintf("%s(%s)%s", name, args, ret)
	}

	if after, ok := strings.CutPrefix(s, "type "); ok {
		rest := strings.TrimSpace(after)
		nameEnd := strings.IndexAny(rest, " {")
		if nameEnd == -1 {
			return s
		}
		typeName := rest[:nameEnd]
		body := strings.TrimSpace(rest[nameEnd+1:])

		if strings.HasPrefix(body, "struct {") {
			fields := extractGoStructFields(body)
			return fmt.Sprintf("%s{%s}", typeName, strings.Join(fields, ","))
		}
		if strings.HasPrefix(body, "interface {") {
			methods := extractGoInterfaceMethods(body)
			return fmt.Sprintf("%s{%s}", typeName, strings.Join(methods, ";"))
		}
	}
	return s
}

func normalizeArgs(args string) string {
	if args == "" {
		return ""
	}
	parts := strings.Split(args, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if idx := strings.LastIndex(p, " "); idx >= 0 {
			typePart := strings.TrimSpace(p[idx+1:])
			if typePart != "" {
				out = append(out, shortenType(typePart))
				continue
			}
		}
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

func normalizeReturns(ret string) string {
	ret = strings.TrimSpace(ret)
	ret = strings.TrimSuffix(ret, "{}")
	ret = strings.TrimSpace(ret)
	if ret == "" || ret == "()" {
		return ""
	}

	ret = strings.TrimPrefix(ret, "(")
	ret = strings.TrimSuffix(ret, ")")
	ret = strings.TrimSpace(ret)
	if ret == "" {
		return ""
	}

	parts := strings.Split(ret, ",")
	for i := range parts {
		parts[i] = shortenType(strings.TrimSpace(parts[i]))
	}
	if len(parts) == 1 {
		return ":" + parts[0]
	}
	return ":(" + strings.Join(parts, ",") + ")"
}

func receiverType(recv string) string {
	fields := strings.Fields(recv)
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	return strings.TrimPrefix(last, "*")
}

func shortenType(t string) string {
	switch t {
	case "string":
		return "str"
	case "bool", "boolean":
		return "bool"
	case "int":
		return "i32"
	case "int64":
		return "i64"
	case "uint64":
		return "u64"
	case "error":
		return "err"
	case "context.Context":
		return "Ctx"
	default:
		return t
	}
}

func extractGoStructFields(body string) []string {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "struct {")
	body = strings.TrimSuffix(body, "}")
	body = strings.TrimSpace(body)

	lines := strings.Split(body, "\n")
	fields := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "}" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[0] != "" {
			fieldName := parts[0]
			fieldType := shortenType(parts[1])
			fields = append(fields, fieldName+":"+fieldType)
		}
	}
	return fields
}

func extractGoInterfaceMethods(body string) []string {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "interface {")
	body = strings.TrimSuffix(body, "}")
	body = strings.TrimSpace(body)

	lines := strings.Split(body, "\n")
	methods := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "("); idx >= 0 {
			name := strings.TrimSpace(line[:idx])
			rest := line[idx:]
			methods = append(methods, name+rest)
		}
	}
	return methods
}

func denseTS(s string) string {
	if m := tsFuncRe.FindStringSubmatch(s); m != nil {
		name := m[1]
		args := normalizeArgs(m[2])
		ret := m[3]
		if ret != "" {
			ret = strings.TrimSpace(ret)
			ret = strings.TrimSuffix(ret, "{}")
			ret = strings.TrimSpace(ret)
			if ret != "" {
				return fmt.Sprintf("%s(%s):%s", name, args, ret)
			}
		}
		return fmt.Sprintf("%s(%s)", name, args)
	}
	return s
}

func denseRust(s string) string {
	s = strings.ReplaceAll(s, "pub ", "")
	s = strings.ReplaceAll(s, "fn ", "")
	return s
}
