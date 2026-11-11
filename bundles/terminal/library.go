// Package terminal provides copyable, dependency-light terminal formatting primitives.
package terminal

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nexssp/flow/compiler"
	flowtemplate "github.com/nexssp/flow/extensions/template"
	"github.com/nexssp/kernel/action"
)

const ID = "terminal"

//go:embed *.nflow *.tmpl
var FS embed.FS

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/terminal/nexssflow", Bundle)
}

// Bundle exposes the upstream renderer used by the terminal NFlow modules.
func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: ID, Libraries: []action.Library{{Name: ID}, flowtemplate.Library()}, Symbols: []compiler.SymbolNamespace{{Name: ID, Symbols: map[string]any{
		"printf": fmt.Sprintf,
		"width":  displayWidth, "tokens": humanTokens, "bytes": humanBytes,
		"age": formatAge, "size": legacySize, "clean": clean,
		"paint": paint, "bar": bar, "template": templateSource,
	}}}}
}

// templateSource returns a bundled .tmpl asset by its file name. Restricting
// lookup to the bundle root keeps template access independent of host files.
func templateSource(name string) (string, error) {
	if filepath.Base(name) != name || filepath.Ext(name) != ".tmpl" {
		return "", fmt.Errorf("terminal.template: expected a bundled .tmpl file name, got %q", name)
	}
	data, err := fs.ReadFile(FS, name)
	if err != nil {
		return "", fmt.Errorf("terminal.template: %w", err)
	}
	return string(data), nil
}

func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

// paint and bar are presentation primitives; callers supply every color and
// dimension. Thresholds and product labels belong in NFlow, not these helpers.
func paint(text, code string, enabled bool) string {
	if !enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func bar(pct float64, width int, code string, color bool) string {
	pct = min(max(pct, 0), 100)
	width = min(max(width, 0), 4096)
	eighths := int(pct*float64(width)*8/100 + 0.5)
	full, rem := eighths/8, eighths%8
	filled := strings.Repeat(paint("█", code, color), full)
	if rem > 0 && full < width {
		filled += paint("▓", code, color)
		full++
	}
	return "[" + filled + strings.Repeat(paint("░", "2;37", color), width-full) + "]"
}

func (sourceLoader) Load(_, target string) ([]byte, string, error) {
	name := filepath.Base(target)
	data, err := fs.ReadFile(FS, name)
	return data, name, err
}

func displayWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case r == 0xFE0F:
		case r >= 0x1F300 && r <= 0x1FAFF, r >= 0x2600 && r <= 0x27BF:
			width += 2
		default:
			width++
		}
	}
	return width
}

func humanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return strconv.Itoa(n)
	}
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func legacySize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f := float64(n)
	i := -1
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func formatAge(days int) string {
	switch {
	case days < 0:
		return ""
	case days == 0:
		return "today"
	case days == 1:
		return "1d"
	case days < 7:
		return fmt.Sprintf("%dd", days)
	case days < 30:
		return fmt.Sprintf("%dw", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		return fmt.Sprintf("%dy", days/365)
	}
}

func clean(text string, width int) string {
	text = strings.Join(strings.Fields(text), " ")
	if width <= 0 || utf8.RuneCountInString(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	r := []rune(text)
	return string(r[:width-1]) + "…"
}
