package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
	"gopkg.in/yaml.v3"

	"github.com/nexssp/srcpack/bundles/config"
)

func TestStage9SaveViewNormalizesSelectorsAndOverwrites(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"nexss.srcpack.yml": "views:\n  existing:\n    - old/path#arch\n",
		"sub/one.go":        "package sub\n",
		"other/two.go":      "package other\n",
	})
	chdirForTest(t, filepath.Join(root, "sub"))

	ktest.RequireNoError(t, saveView(t.Context(), "existing", []string{"one.go,../other/two.go~vendor/**#arch@custom prompt"}))
	var raw map[string]any
	data, err := os.ReadFile(filepath.Join(root, config.ConfigFileName))
	ktest.RequireNoError(t, err)
	ktest.RequireNoError(t, yaml.Unmarshal(data, &raw))
	views, ok := raw["views"].(map[string]any)
	if !ok {
		t.Fatalf("views has type %T, want map[string]any", raw["views"])
	}
	selectors, ok := views["existing"].([]any)
	if !ok || len(selectors) != 1 {
		t.Fatalf("existing view has value %#v, want one selector", views["existing"])
	}
	ktest.RequireEqual(t, selectors[0], "sub/one.go,other/two.go~vendor/**#arch@custom prompt")
}

func TestStage9SaveViewPreservesKnownAndUnknownYAML(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"nexss.srcpack.yml": "editor: vim\nprompts:\n  custom: hello\nprofiles:\n  custom:\n    name: custom\noptions:\n  output: report.md\nfuture_setting:\n  nested: [one, two]\nviews:\n  old: [old.go]\n",
		"main.go":           "package main\n",
	})
	chdirForTest(t, root)
	ktest.RequireNoError(t, saveView(t.Context(), "old", []string{"main.go#impl"}))
	data, err := os.ReadFile(filepath.Join(root, config.ConfigFileName))
	ktest.RequireNoError(t, err)
	var after map[string]any
	ktest.RequireNoError(t, yaml.Unmarshal(data, &after))
	ktest.RequireEqual(t, after["editor"], any("vim"))
	prompts, ok := after["prompts"].(map[string]any)
	if !ok {
		t.Fatalf("prompts have type %T, want map[string]any", after["prompts"])
	}
	ktest.RequireEqual(t, prompts["custom"], any("hello"))
	profiles, ok := after["profiles"].(map[string]any)
	if !ok {
		t.Fatalf("profiles have type %T, want map[string]any", after["profiles"])
	}
	custom, ok := profiles["custom"].(map[string]any)
	if !ok {
		t.Fatalf("custom profile has type %T, want map[string]any", profiles["custom"])
	}
	ktest.RequireEqual(t, custom["name"], any("custom"))
	options, ok := after["options"].(map[string]any)
	if !ok {
		t.Fatalf("options have type %T, want map[string]any", after["options"])
	}
	ktest.RequireEqual(t, options["output"], any("report.md"))
	future, ok := after["future_setting"].(map[string]any)
	if !ok {
		t.Fatalf("future_setting has type %T, want map[string]any", after["future_setting"])
	}
	ktest.RequireEqual(t, future["nested"], any([]any{"one", "two"}))
	views, ok := after["views"].(map[string]any)
	if !ok {
		t.Fatalf("views have type %T, want map[string]any", after["views"])
	}
	ktest.RequireEqual(t, views["old"], any([]any{"main.go#impl"}))
	ktest.RequireNoError(t, runView([]string{"delete", "old"}))
	data, err = os.ReadFile(filepath.Join(root, config.ConfigFileName))
	ktest.RequireNoError(t, err)
	var deleted map[string]any
	ktest.RequireNoError(t, yaml.Unmarshal(data, &deleted))
	delete(after, "views")
	delete(deleted, "views")
	ktest.RequireEqual(t, deleted, after)
}

func TestStage9RunViewListShowDelete(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"nexss.srcpack.yml": "views:\n  zeta: [z.go]\n  alpha: [a.go, b.go#arch]\n",
	})
	chdirForTest(t, root)
	var output strings.Builder
	previous := os.Stdout
	read, write, err := os.Pipe()
	ktest.RequireNoError(t, err)
	t.Cleanup(func() { os.Stdout = previous; _ = read.Close(); _ = write.Close() })
	os.Stdout = write
	ktest.RequireNoError(t, runView([]string{"list"}))
	ktest.RequireNoError(t, write.Close())
	os.Stdout = previous
	_, err = io.Copy(&output, read)
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, strings.Index(output.String(), "alpha") < strings.Index(output.String(), "zeta"), "list should sort names: %s", output.String())
	ktest.RequireCondition(t, strings.Contains(output.String(), "a.go  b.go#arch"), "list omitted selectors: %s", output.String())

	ktest.RequireNoError(t, runView([]string{"show", "alpha"}))
	ktest.RequireNoError(t, runView([]string{"delete", "alpha"}))
	cfg, err := config.NewConfigStore(filepath.Join(root, config.ConfigFileName)).LoadRaw()
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, cfg.Views["alpha"] == nil, "deleted view still present: %#v", cfg.Views)
	ktest.RequireCondition(t, cfg.Views["zeta"] != nil, "delete removed unrelated view")
}

func TestStage9PreprocessViewsResolvesAndPreservesArguments(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"nexss.srcpack.yml": ""})
	chdirForTest(t, root)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdirForTest(t, filepath.Join(root, "sub"))
	cfg := config.Config{Views: map[string][]string{"view": {"src,lib~vendor/**#arch@prompt slot"}, "directory": {"wrong.go"}, "flagval": {"bad.go"}}}
	if err := os.MkdirAll("directory", 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"--target", "view", "--output=flagval", "--ext", "directory", "view", "directory", "unknown", "--", "view"}
	got := preprocessViews(args, cfg)
	want := []string{"--target", "view", "--output=flagval", "--ext", "directory", filepath.Join(root, "src") + "," + filepath.Join(root, "lib") + "~vendor/**#arch@prompt slot", "directory", "unknown", "--", filepath.Join(root, "src") + "," + filepath.Join(root, "lib") + "~vendor/**#arch@prompt slot"}
	ktest.RequireEqual(t, got, want)
}

func TestStage9SaveViewWriteFailureLeavesConfigUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, config.ConfigFileName)
	original := []byte("editor: vim\nviews:\n  keep: [keep.go]\n")
	ktest.RequireNoError(t, os.WriteFile(path, original, 0o600))
	ktest.RequireNoError(t, os.Chmod(root, 0o500))
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	chdirForTest(t, root)
	err := saveView(t.Context(), "new", []string{"new.go"})
	ktest.RequireCondition(t, err != nil, "saveView should surface a failed write")
	after, readErr := os.ReadFile(path)
	ktest.RequireNoError(t, readErr)
	ktest.RequireEqual(t, after, original)
}
