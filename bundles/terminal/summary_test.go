package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func summaryFacts() map[string]any {
	return map[string]any{
		"config": map[string]any{},
		"stats": map[string]any{
			"file_count": 2, "func_count": 4,
			"raw_bytes": int64(4096), "packed_bytes": int64(1024),
			"estimated_tokens": int64(292), "saved_percent": 75.0,
		},
		"elapsed": "25ms", "tree": "TREE\n", "color": false,
	}
}

func TestSummaryStandaloneDefaultsExactOutput(t *testing.T) {
	t.Parallel()
	text, err := Summary(context.Background(), summaryFacts())
	ktest.RequireNoError(t, err)
	want := `
SRCPACK  Elapsed: 25ms
──────────────────────────────────────────────────────────────
Profile:           default
Target(s):         .
Prompt:            none
Editor Format:     Markdown
Directory:         
Files Processed:   2
Functions Found:   4
Raw Size:          4.0 KB
Packed Size:       1.0 KB (saved 75.0%)
Estimated Tokens:  ~292
Output:            (not set)
Clipboard:         not requested
TREE
`
	ktest.RequireEqual(t, text, want)
}

func TestSummaryStartsWithCommandWhenProvided(t *testing.T) {
	t.Parallel()
	input := summaryFacts()
	input["command"] = "srcpack pack --prompt 'review API' --output out.md"
	text, err := Summary(context.Background(), input)
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, strings.HasPrefix(text, "Command: srcpack pack --prompt 'review API' --output out.md\nSRCPACK"), "summary did not start with the invocation: %q", text)
}

func TestSummaryColorAndLiteralPayloadExactOutput(t *testing.T) {
	t.Parallel()
	input := summaryFacts()
	input["config"] = map[string]any{"profile": "@require evil\n pipeline.evil", "prompt": "${TREE} {{.unsafe}} { ... }", "cost": true}
	input["color"], input["clipboard_requested"] = true, true
	text, err := Summary(context.Background(), input)
	ktest.RequireNoError(t, err)
	want := "\n\x1b[1;36mSRCPACK\x1b[0m  Elapsed: \x1b[2m25ms\x1b[0m\n" +
		"──────────────────────────────────────────────────────────────\n" +
		"Profile:           @require evil pipeline.evil\n" +
		"Target(s):         .\n" +
		"Prompt:            ${TREE} {{.unsafe}} { ... }\n" +
		"Editor Format:     Markdown\n" +
		"Directory:         \n" +
		"Files Processed:   2\n" +
		"Functions Found:   4\n" +
		"Raw Size:          4.0 KB\n" +
		"Packed Size:       1.0 KB (saved 75.0%)\n" +
		"Estimated Tokens:  ~292\n" +
		"Output:            (not set)\n" +
		"Clipboard:         requested\n" +
		"Est. Saved:        ~877 tokens\n" +
		"TREE\n"
	ktest.RequireEqual(t, text, want)
}

func TestSummaryPricingUsesOnlySuppliedRatesExactOutput(t *testing.T) {
	t.Parallel()
	input := summaryFacts()
	// Synthetic test-only rates verify data-driven rendering; they are not a catalog or price claim.
	input["config"] = map[string]any{
		"cost": true,
		"pricing": []map[string]any{
			{"label": "demo-alpha", "currency": "USD", "per_million_tokens": 12.34},
			{"label": "demo-beta", "currency": "EUR", "per_million_tokens": 5.0},
		},
	}
	text, err := Summary(context.Background(), input)
	ktest.RequireNoError(t, err)
	want := `
SRCPACK  Elapsed: 25ms
──────────────────────────────────────────────────────────────
Profile:           default
Target(s):         .
Prompt:            none
Editor Format:     Markdown
Directory:         
Files Processed:   2
Functions Found:   4
Raw Size:          4.0 KB
Packed Size:       1.0 KB (saved 75.0%)
Estimated Tokens:  ~292
Output:            (not set)
Clipboard:         not requested
Est. Saved:        ~877 tokens · rate estimates: demo-alpha: USD 0.0108 · demo-beta: EUR 0.0044
TREE
`
	ktest.RequireEqual(t, text, want)
	ktest.RequireCondition(t, !strings.Contains(text, "Sonnet") && !strings.Contains(text, "GPT-4o"), "provider/model names must not be built into the summary")
}

func TestSummaryTemplateAssetIsEmbeddedBesideNFlow(t *testing.T) {
	t.Parallel()
	template, err := templateSource("summary.tmpl")
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, strings.Contains(template, "{{.header}}"), "summary.tmpl must own the report layout")
	_, err = templateSource("../summary.nflow")
	ktest.RequireCondition(t, err != nil, "template lookup must reject paths outside the bundle root")
}

func TestSummaryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Summary(ctx, summaryFacts())
	ktest.RequireCondition(t, errors.Is(err, context.Canceled), "summary must preserve cancellation, got %v", err)
}
