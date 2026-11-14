package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func captureFlowCLIStreams(t *testing.T, run func() error) (string, string, error) {
	t.Helper()
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		t.Fatal(err)
	}
	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	defer func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
	}()
	stdoutResult, stderrResult := make(chan string, 1), make(chan string, 1)
	go func() { data, _ := io.ReadAll(stdoutReader); stdoutResult <- string(data) }()
	go func() { data, _ := io.ReadAll(stderrReader); stderrResult <- string(data) }()
	runErr := run()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout, os.Stderr = originalStdout, originalStderr
	return <-stdoutResult, <-stderrResult, runErr
}

func TestFlowCLIPackSummaryStartsWithOriginalCommand(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Main() {}\n"})
	t.Chdir(root)
	args := []string{"pack", "--file", "main.go", "--prompt", "review API", "--output", "out.md"}
	_, stderr, err := captureFlowCLIStreams(t, func() error { return execute(context.Background(), args) })
	if err != nil {
		t.Fatal(err)
	}
	want := "Command: srcpack pack --file main.go --prompt 'review API' --output out.md\nSRCPACK"
	if !strings.HasPrefix(stderr, want) {
		t.Fatalf("summary did not start with the original invocation; want prefix %q, got:\n%s", want, stderr)
	}
	content, err := os.ReadFile("out.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "func Main()") || strings.HasPrefix(strings.TrimSpace(string(content)), "{") {
		t.Fatalf("normal output was not the expected rendered Markdown: %s", content)
	}
}

func TestFlowCLIPackWarnsWhenOnlyBinaryFilesAreSelected(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "asset.bin"), []byte{0x00, 0x01, 0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	_, stderr, err := captureFlowCLIStreams(t, func() error {
		return execute(context.Background(), []string{"pack", "--file", "asset.bin", "--stdout", "--quiet"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "Warning: no source files were included") {
		t.Fatalf("binary-only selection should explain the empty pack; stderr: %s", stderr)
	}
}

func TestFlowCLIPackMasksSecretsByDefault(t *testing.T) {
	secret := "sk-proj-" + strings.Repeat("R", 40)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yml"), []byte("api_token: \""+secret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	stdout, _, err := captureFlowCLIStreams(t, func() error {
		return execute(context.Background(), []string{"pack", "--include-config", "--output", "-"})
	})
	if err != nil {
		t.Fatal("srcpack pack failed")
	}
	if !strings.Contains(stdout, "[REDACTED_CREDENTIAL]") || strings.Contains(stdout, secret) {
		t.Fatal("default masking did not redact the included config value")
	}
}

func TestFlowCLINoMaskPreservesIncludedConfigSecret(t *testing.T) {
	const secret = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yml"), []byte("token: "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	stdout, _, err := captureFlowCLIStreams(t, func() error {
		return execute(context.Background(), []string{"pack", "--include-config", "--no-mask", "--output", "-"})
	})
	if err != nil {
		t.Fatal("srcpack --no-mask failed")
	}
	if !strings.Contains(stdout, "config.yml") || !strings.Contains(stdout, secret) {
		t.Fatal("unmasked config content missing from output")
	}
	if strings.Contains(stdout, "[REDACTED_SECRET]") {
		t.Fatal("--no-mask unexpectedly redacted config content")
	}
}

func TestFlowCLIFindAcceptsStdoutFlag(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"sample.go": "package sample\ntype UserService struct{}\n"})
	t.Chdir(root)
	stdout, _, err := captureFlowCLIStreams(t, func() error {
		return execute(context.Background(), []string{"find", "UserService", "--stdout"})
	})
	if err != nil {
		t.Fatalf("srcpack find --stdout failed: %v", err)
	}
	if !strings.Contains(stdout, "UserService") || !strings.Contains(stdout, "Found 1 match") {
		t.Fatalf("find stdout result = %q", stdout)
	}
}

func TestFlowCLIPackJSONOutputIsValidJSONWithoutHumanSummary(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Main() {}\n"})
	t.Chdir(root)
	args := []string{"pack", "--file", "main.go", "--json", "--output", "-"}
	stdout, stderr, err := captureFlowCLIStreams(t, func() error { return execute(context.Background(), args) })
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Operation     string `json:"operation"`
		OutputPath    string `json:"output_path"`
		Content       string `json:"content"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("--json produced invalid JSON: %v\n%s", err, stdout)
	}
	if result.SchemaVersion != 1 || result.Operation != "pack" || result.OutputPath != "-" || !strings.Contains(result.Content, "func Main()") {
		t.Fatalf("unexpected JSON pack result: %#v", result)
	}
	if result.Command != "srcpack pack --file main.go --json --output -" {
		t.Fatalf("JSON command metadata = %q", result.Command)
	}
	if strings.Contains(stderr, "SRCPACK") || strings.Contains(stderr, "Completed in") {
		t.Fatalf("human-readable summary leaked in JSON mode: %s", stderr)
	}
}

func TestFlowCLIPackJSONFileOutputIsValidJSON(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Main() {}\n"})
	t.Chdir(root)
	args := []string{"pack", "--file", "main.go", "--json", "--output", "packed.json"}
	stdout, stderr, err := captureFlowCLIStreams(t, func() error { return execute(context.Background(), args) })
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("JSON file mode wrote unexpected terminal output; stdout=%q stderr=%q", stdout, stderr)
	}
	data, err := os.ReadFile("packed.json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("--json output file is invalid JSON: %v\n%s", err, data)
	}
	content, ok := result["content"].(string)
	if result["operation"] != "pack" || !ok || !strings.Contains(content, "func Main()") {
		t.Fatalf("unexpected JSON file result: %#v", result)
	}
}

func TestFlowCLIPackJSONCheckWritesJSONStatus(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Main() {}\n"})
	t.Chdir(root)
	args := []string{"pack", "--file", "main.go", "--json", "--output", "packed.json", "--check"}
	stdout, stderr, err := captureFlowCLIStreams(t, func() error { return execute(context.Background(), args) })
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status string `json:"status"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("--json --check produced invalid JSON: %v\n%s", err, stdout)
	}
	if result.Status != "NEW" || result.Path != "packed.json" || stderr != "" {
		t.Fatalf("unexpected JSON check result: %#v, stderr=%q", result, stderr)
	}
	if _, err := os.Stat("packed.json"); !os.IsNotExist(err) {
		t.Fatalf("check mode unexpectedly wrote packed.json: %v", err)
	}
}

func TestCommandInvocationNormalizesOnlyCommandSyntax(t *testing.T) {
	tests := []struct {
		args, want        []string
		command           string
		explicit, handled bool
	}{
		{[]string{"p", "a,b#arch"}, []string{"pack", "a,b#arch"}, "pack", false, true},
		{[]string{"pack", "--copy=false"}, []string{"pack", "--copy=false"}, "pack", true, true},
		{[]string{"ARCH", "-t", "src"}, []string{"pack", "ARCH", "-t", "src"}, "pack", false, true},
		{[]string{"up", "--check"}, []string{"update", "--check"}, "update", false, true},
		{[]string{"help", "init"}, []string{"help", "init"}, "help", false, true},
		{[]string{"doctor"}, []string{"doctor"}, "doctor", false, true},
		{nil, []string{"dashboard"}, "dashboard", false, true},
		{[]string{"--help"}, []string{"usage"}, "usage", false, true},
		{[]string{"profiles"}, []string{"list"}, "list", false, true},
		{[]string{"-H", "P", "src#arch@review"}, []string{"help", "pack", "src#arch@review"}, "help", false, true},
		{[]string{"PACK", "a,b~test#source@prompt"}, []string{"pack", "a,b~test#source@prompt"}, "pack", true, true},
		{[]string{"unknown#arch", "--copy=false"}, []string{"pack", "unknown#arch", "--copy=false"}, "pack", false, true},
		{[]string{"--version"}, []string{"version"}, "version", false, true},
		{[]string{"run-all"}, []string{"update"}, "update", false, true},
		{[]string{"validate"}, []string{"doctor"}, "doctor", false, true},
		{[]string{"type-list"}, []string{"types"}, "types", false, true},
		{[]string{"cookbook"}, []string{"cookbook"}, "cookbook", false, true},
		{[]string{"skill"}, []string{"skill"}, "skill", false, true},
	}
	for _, tc := range tests {
		got, cmd, explicit, err := commandInvocation(context.Background(), tc.args)
		if err != nil {
			t.Fatal(err)
		}
		handled := true
		if !reflect.DeepEqual(got, tc.want) || cmd != tc.command || explicit != tc.explicit || handled != tc.handled {
			t.Fatalf("%v: got %v %q %v %v", tc.args, got, cmd, explicit, handled)
		}
	}
}

func TestFlowCLIInitConfigAndNoJSONLeak(t *testing.T) {
	t.Chdir(t.TempDir())
	stdout, err := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"init"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Next steps:") || strings.Contains(stdout, "\nnull\n") || strings.Contains(stdout, "\"config_body\"") {
		t.Fatalf("unexpected init output %q", stdout)
	}
	if _, err := os.Stat("nexss.srcpack.yml"); err != nil {
		t.Fatal(err)
	}
	stdout, err = capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"config", "path"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(stdout), "nexss.srcpack.yml") {
		t.Fatalf("unexpected config path output %q", stdout)
	}
}

func TestFlowCLIDashboardFormatsAndSortsWorkspaceFacts(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"nexss.srcpack.yml": "targets:\n  zebra:\n    paths: [.]\n  alpha:\n    paths: [.]\nviews:\n  zeta: [z.go]\n  alpha: [a.go]\n",
	})
	t.Chdir(root)
	stdout, err := capturePolicyStdout(t, func() error {
		return execute(context.Background(), []string{"dashboard"})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Working in:  " + root,
		"Config:      nexss.srcpack.yml",
		"Targets:     alpha, zebra",
		"Views:       alpha, zeta",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("dashboard output omitted %q:\n%s", want, stdout)
		}
	}
}

func TestFlowCLIInitUnknownFlagAndFalseDoNotOverwrite(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := execute(context.Background(), []string{"init", "--does-not-exist"}); err == nil {
		t.Fatal("unknown flag accepted")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("invalid flags wrote files")
	}
	if err := os.WriteFile("nexss.srcpack.yml", []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(context.Background(), []string{"init", "--force=false"}); err == nil {
		t.Fatal("false override ignored")
	}
	data, err := os.ReadFile("nexss.srcpack.yml")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep\n" {
		t.Fatalf("existing config changed %q", data)
	}
}

func TestFlowCLIInitInvalidTargetsDoesNotReset(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"_srcpack/context.md": "keep generated\n", "_srcpack/notes.txt": "keep handwritten\n"})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"init", "--reset", "targets", "missing-dir"}); err == nil {
		t.Fatal("missing target accepted")
	}
	for _, name := range []string{"context.md", "notes.txt"} {
		if _, err := os.Stat(filepath.Join("_srcpack", name)); err != nil {
			t.Fatalf("validation removed %s: %v", name, err)
		}
	}
}

func TestFlowCLIMixedCommaSelectorsKeepPerTargetProfiles(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"a/a.go": "package a\nfunc A() string { return \"BODY_A\" }\n",
		"b/b.go": "package b\nfunc B() string { return \"BODY_B\" }\n",
		"c/c.go": "package c\nfunc C() string { return \"BODY_C\" }\n",
	})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"pack", "a,b#arch", "c#source", "--output", "out.md", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("out.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, name := range []string{"A():str", "B():str", "func C()"} {
		if !strings.Contains(s, name) {
			t.Fatalf("missing %s in output:\n%s", name, s)
		}
	}
	if strings.Contains(s, "BODY_A") || strings.Contains(s, "BODY_B") || !strings.Contains(s, "BODY_C") {
		t.Fatalf("per-target profiles lost: %s", s)
	}
}

func TestFlowCLIOpenEndedLineSelectorKeepsProfile(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"main.go": "package main\n\nfunc Selected() string {\n\treturn \"selected body\"\n}\n",
	})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"main.go:1-#source", "--output", "out.md", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("out.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `return "selected body"`) {
		t.Fatalf("open-ended selector lost its source profile:\n%s", data)
	}
}
