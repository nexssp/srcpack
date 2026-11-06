package operators

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/terminal"
)

type stage8SummaryFixture struct {
	Name   string             `json:"name"`
	Config StatsCollectConfig `json:"config"`
	Stats  PackStats          `json:"stats"`
	Color  bool               `json:"color"`
	Tree   string             `json:"tree"`
	Output string             `json:"output"`
}

var stage8ElapsedLine = regexp.MustCompile(`Elapsed: (?:\x1b\[2m)?[^\n]*(?:\x1b\[0m)?\n`)

func TestStage8SummaryParity(t *testing.T) {
	b, e := os.ReadFile("testdata/stage8_summary_parity.json")
	ktest.RequireNoError(t, e)
	var cases []stage8SummaryFixture
	ktest.RequireNoError(t, json.Unmarshal(b, &cases))
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			out, e := terminal.Summary(context.Background(), terminalSummaryInput(c.Config, c.Stats, "0s", c.Color, c.Tree))
			ktest.RequireNoError(t, e)
			out = stage8ElapsedLine.ReplaceAllString(out, "Elapsed: <elapsed>\n")
			ktest.RequireEqual(t, out, c.Output)
		})
	}
}

func TestTerminalSummaryInputPreservesPricingData(t *testing.T) {
	rates := []map[string]any{{"label": "caller-rate", "currency": "USD", "per_million_tokens": 7.25}}
	input := terminalSummaryInput(StatsCollectConfig{ShowCost: true, Pricing: rates}, PackStats{}, "0s", false, "")
	config, ok := input["config"].(map[string]any)
	ktest.RequireCondition(t, ok, "expected config object, got %T", input["config"])
	got, ok := config["pricing"].([]map[string]any)
	ktest.RequireCondition(t, ok, "expected pricing list, got %T", config["pricing"])
	ktest.RequireEqual(t, got, rates)
}
