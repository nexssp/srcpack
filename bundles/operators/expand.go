package operators

import (
	"strconv"

	"github.com/nexssp/srcpack/bundles/placeholder"
)

// expandPlaceholders replaces ${KEY} tokens in tmpl using lookup. When
// lookup returns ok=false the original ${KEY} text is preserved, so a
// template can reference optional values without a nil check. A
// template with no "${" is returned unchanged and allocates nothing.
func expandPlaceholders(tmpl string, lookup func(key string) (string, bool)) string {
	return placeholder.ExpandOnce(tmpl, lookup)
}

func itoa(v int) string     { return strconv.Itoa(v) }
func i64toa(v int64) string { return strconv.FormatInt(v, 10) }
func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
