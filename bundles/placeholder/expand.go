// Package placeholder contains the shared literal placeholder scanner.
package placeholder

import "strings"

// ExpandOnce replaces ${KEY} tokens using lookup. Unknown and unterminated
// tokens are preserved, and inserted values are not scanned again.
func ExpandOnce(source string, lookup func(key string) (string, bool)) string {
	open := strings.Index(source, "${")
	if open < 0 {
		return source
	}

	var out strings.Builder
	out.Grow(len(source))
	for {
		open = strings.Index(source, "${")
		if open < 0 {
			out.WriteString(source)
			return out.String()
		}
		out.WriteString(source[:open])

		closeIndex := strings.IndexByte(source[open+2:], '}')
		if closeIndex < 0 {
			out.WriteString(source[open:])
			return out.String()
		}
		end := open + 2 + closeIndex
		key := source[open+2 : end]
		if value, ok := lookup(key); ok {
			out.WriteString(value)
		} else {
			out.WriteString(source[open : end+1])
		}
		source = source[end+1:]
	}
}
