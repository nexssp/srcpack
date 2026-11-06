package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nexssp/flow/extensions/schema"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/operators"
)

func TestRootPayloadNormalizesSlices(t *testing.T) {
	t.Parallel()
	payload, err := rootPayload("pack", resolved{Pack: operators.PackConfig{Targets: []string{"src"}}})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, payload["command"], any("pack"))
	ktest.RequireEqual(t, payload["targets"], any([]any{"src"}))
	ktest.RequireEqual(t, payload["files"], any([]any{}))
	ktest.RequireEqual(t, payload["examples"], any([]any{}))
	flowSchema := schema.Schema{Name: "Input", Fields: []schema.Field{
		{JSONName: "targets", Kind: schema.KindString, Slice: true},
		{JSONName: "files", Kind: schema.KindString, Slice: true},
	}}
	ktest.RequireNoError(t, schema.Validate(context.Background(), flowSchema, payload))
}

func TestRootPayloadPreservesDomainOptions(t *testing.T) {
	t.Parallel()
	payload, err := rootPayload("tree", resolved{Pack: operators.PackConfig{
		Examples: []string{"examples"}, TargetProfiles: map[string]string{"src": "arch"},
		CommentsOverride: new(false), NoTests: true,
	}})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, payload["examples"], any([]any{"examples"}))
	ktest.RequireEqual(t, payload["comments_override"], any(false))
	ktest.RequireEqual(t, payload["no_tests"], any(true))
	ktest.RequireEqual(t, payload["target_profiles"], any(map[string]any{"src": "arch"}))
}

func TestRootConfigSeparatesDisplayAndDomainValues(t *testing.T) {
	t.Parallel()
	run := resolved{
		Pack:       operators.PackConfig{Targets: []string{"/actual/source"}, Prompt: "source prompt"},
		Stats:      operators.StatsCollectConfig{Targets: "stats targets", Prompt: "stats prompt", Output: "display output"},
		Render:     operators.RenderConfig{Targets: "render targets", Prompt: "render prompt", OutputFile: "header output"},
		OutputPath: "/actual/output",
	}
	values, err := rootConfig(cliOptions{}, run)
	ktest.RequireNoError(t, err)
	for key, want := range map[string]string{
		"stats_targets": "stats targets", "render_targets": "render targets",
		"stats_prompt": "stats prompt", "render_prompt": "render prompt",
		"stats_output": "display output", "render_output_file": "header output",
	} {
		ktest.RequireEqual(t, values[key], want)
	}
	_, present := values["targets"]
	ktest.RequireCondition(t, !present, "display targets must not shadow domain targets")
}

func TestRootPipelineRejectsUnknownCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := executeRootPipeline(context.Background(), cliOptions{Command: "unsupported"}, resolved{})
	if err == nil || !strings.Contains(err.Error(), "unknown srcpack pipeline command") {
		t.Fatalf("unexpected root routing result: %v", err)
	}
	entries, err := os.ReadDir(".")
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, len(entries), 0)
}

func TestRootPipelineBindsMultilineConfigAsData(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Hello() {}\n"})
	t.Chdir(root)
	cli, err := parseCLI([]string{"pack", "-t", ".", "-o", "out.md", "-q", "--color=never"})
	ktest.RequireNoError(t, err)
	run, err := resolve(context.Background(), cli, time.Unix(1234567890, 0))
	ktest.RequireNoError(t, err)
	template := "line one `tick` \"quote\" : -> runtime.fail\nsecond line @config.not_a_reference\n"
	run.Render.Start = template
	run.Render.End = "END OF DOCUMENT\n"
	run.Render.Prompt = "prompt with ` and \"\nnew line"
	run.OutputPath = filepath.Join(root, "output.md")
	_, err = executeRootPipeline(context.Background(), cli, run)
	ktest.RequireNoError(t, err)
	content, err := os.ReadFile(run.OutputPath)
	ktest.RequireNoError(t, err)
	ktest.RequireStringContains(t, string(content), template)
	ktest.RequireStringContains(t, string(content), strings.ReplaceAll(run.Render.Prompt, "\n", "\n> "))
	ktest.RequireStringContains(t, string(content), "END OF DOCUMENT")
}
