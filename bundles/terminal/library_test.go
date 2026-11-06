package terminal

import (
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestFormattingPrimitives(t *testing.T) {
	t.Parallel()
	ktest.RequireEqual(t, displayWidth("📦x"), 3)
	ktest.RequireEqual(t, displayWidth("⚙️ "), 3)
	ktest.RequireEqual(t, clean("a   b\nc", 5), "a b c")
	ktest.RequireEqual(t, clean("zażółć", 4), "zaż…")
	ktest.RequireEqual(t, clean("abcdef", 1), "…")
	ktest.RequireEqual(t, legacySize(1024), "1.0 KB")
	ktest.RequireEqual(t, humanBytes(1024), "1.0 KiB")
	for _, tc := range []struct {
		n    int
		want string
	}{
		{0, "0"}, {999, "999"}, {1000, "1.0k"}, {9999, "10.0k"}, {10000, "10k"}, {1000000, "1.0M"},
	} {
		ktest.RequireEqual(t, humanTokens(tc.n), tc.want)
	}
	for _, tc := range []struct {
		days int
		want string
	}{
		{-1, ""}, {0, "today"}, {1, "1d"}, {6, "6d"}, {7, "1w"}, {30, "1mo"}, {365, "1y"},
	} {
		ktest.RequireEqual(t, formatAge(tc.days), tc.want)
	}
}

func TestBarPrimitives(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pct  float64
		want string
	}{
		{-1, "[░░░░]"}, {0, "[░░░░]"}, {1, "[░░░░]"}, {2, "[▓░░░]"}, {25, "[█░░░]"}, {100, "[████]"}, {101, "[████]"},
	} {
		ktest.RequireEqual(t, bar(tc.pct, 4, "1;32", false), tc.want)
	}
	ktest.RequireEqual(t, bar(50, 0, "1", false), "[]")
	ktest.RequireEqual(t, paint("x", "1;32", true), "\x1b[1;32mx\x1b[0m")
	ktest.RequireEqual(t, paint("x", "1;32", false), "x")
	colored := bar(25, 4, "1;32", true)
	ktest.RequireStringContains(t, colored, "\x1b[1;32m█\x1b[0m")
	ktest.RequireEqual(t, strings.Count(colored, "\x1b[2;37m░\x1b[0m"), 3)
}
