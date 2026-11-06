package terminal

import (
	"context"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func treeFacts() map[string]any {
	return map[string]any{
		"empty": false, "full": false, "included": 1, "excluded": 0,
		"total_files": 1, "total_tokens": int64(100), "raw_bytes": int64(1400), "packed_bytes": int64(350),
		"color": false, "languages": map[string]int{"go": 1},
		"rows": []map[string]any{{
			"name": "src", "dir": true, "prefix": "", "connector": "└── ",
			"files": 1, "tokens": int64(100), "funcs": 2, "lang": "go", "fully_excluded": false,
			"role": "", "age": -1, "lines": 0, "paired": "", "excluded": false, "target": false,
		}},
	}
}

func TestTreeStandalone(t *testing.T) {
	t.Parallel()
	text, err := Tree(context.Background(), treeFacts())
	ktest.RequireNoError(t, err)
	for _, want := range []string{"📦 Context Tree (1 files · ~100 tokens)", "Languages: go 100%", "└── 📁 src/", "100.0% [████████████████] [go · 1 files · 100 tk · 2 fn]", "75.0% [████░░░░░░░░░░░░]", "1.0 KiB saved of 1.4 KiB"} {
		ktest.RequireStringContains(t, text, want)
	}
	ktest.RequireCondition(t, !strings.Contains(text, "\x1b["), "plain tree must not produce ANSI")
}

func TestTreeStandaloneEmptyAndColor(t *testing.T) {
	t.Parallel()
	input := treeFacts()
	input["empty"] = true
	text, err := Tree(context.Background(), input)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, text, "")
	input["empty"], input["color"] = false, true
	text, err = Tree(context.Background(), input)
	ktest.RequireNoError(t, err)
	ktest.RequireStringContains(t, text, "\x1b[1m└── 📁 src/\x1b[0m")
	ktest.RequireStringContains(t, text, "\x1b[1;31m100.0%\x1b[0m")
}

func TestTreeLanguageTiesAreDeterministic(t *testing.T) {
	t.Parallel()
	input := treeFacts()
	input["languages"] = map[string]int{"yaml": 1, "rust": 1, "json": 1, "go": 1, "typescript": 1}
	for range 8 {
		text, err := Tree(context.Background(), input)
		ktest.RequireNoError(t, err)
		ktest.RequireStringContains(t, text, "Languages: go 20% · json 20% · rust 20%\n")
	}
}
