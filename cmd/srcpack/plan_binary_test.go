package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestPlanRealBinaryCLI(t *testing.T) {
	binary := buildPlanTestBinary(t)

	t.Run("human plan resolves settings without scanning or writing", func(t *testing.T) {
		root := planFixture(t)
		stdout, stderr, err := runPlanBinary(t, binary, root,
			"pack", "--target", "src", "--profile", "arch", "--no-tests", "--plan")
		if err != nil {
			t.Fatalf("srcpack plan failed: %v\nstderr: %s", err, stderr)
		}
		for _, want := range []string{
			"srcpack plan (schema v1)",
			"Profile: arch",
			"Targets:\n  - src",
			"no-tests=true",
			"scan: not_run",
			"parse: not_run",
			"No candidate-file list is produced by plan mode.",
		} {
			if !strings.Contains(string(stdout), want) {
				t.Errorf("plan output missing %q:\n%s", want, stdout)
			}
		}
		if strings.Contains(string(stdout), `return "source-marker"`) {
			t.Fatalf("plan unexpectedly included source content:\n%s", stdout)
		}
		assertPlanWorkspaceUntouched(t, root)
	})

	t.Run("json plan and explain alias share stable envelope", func(t *testing.T) {
		root := planFixture(t)
		planOut, planErr, err := runPlanBinary(t, binary, root,
			"pack", "--target", "src", "--profile", "arch", "--no-tests", "--plan", "--json")
		if err != nil {
			t.Fatalf("--plan failed: %v\nstderr: %s", err, planErr)
		}
		explainOut, explainErr, err := runPlanBinary(t, binary, root,
			"pack", "--target", "src", "--profile", "arch", "--no-tests", "--explain", "--json")
		if err != nil {
			t.Fatalf("--explain failed: %v\nstderr: %s", err, explainErr)
		}
		flagFirstOut, flagFirstErr, err := runPlanBinary(t, binary, root,
			"--explain", "--target", "src", "--profile", "arch", "--no-tests", "--json")
		if err != nil {
			t.Fatalf("flag-first --explain failed: %v\nstderr: %s", err, flagFirstErr)
		}
		var planValue, explainValue map[string]any
		if err := json.Unmarshal(planOut, &planValue); err != nil {
			t.Fatalf("--plan did not emit JSON: %v\n%s", err, planOut)
		}
		if err := json.Unmarshal(explainOut, &explainValue); err != nil {
			t.Fatalf("--explain did not emit JSON: %v\n%s", err, explainOut)
		}
		var flagFirstValue map[string]any
		if err := json.Unmarshal(flagFirstOut, &flagFirstValue); err != nil {
			t.Fatalf("flag-first --explain did not emit JSON: %v\n%s", err, flagFirstOut)
		}
		if !reflect.DeepEqual(planValue, explainValue) {
			t.Fatalf("plan and explain differ:\nplan: %s\nexplain: %s", planOut, explainOut)
		}
		if flagFirstValue["mode"] != "plan" || flagFirstValue["normal_output_mode"] != "stdout" || flagFirstValue["output_path"] != "" {
			t.Fatalf("flag-first plan did not report the stdout default: %#v", flagFirstValue)
		}
		if planValue["normal_output_mode"] != "stdout" {
			t.Fatalf("default pack output policy not represented: %#v", planValue["normal_output_mode"])
		}
		if planValue["schema_version"] != float64(1) || planValue["mode"] != "plan" {
			t.Fatalf("unexpected plan envelope: %#v", planValue)
		}
		if planValue["candidate_files_enumerated"] != false {
			t.Fatalf("plan must not claim an enumerated candidate list: %#v", planValue)
		}
		filters, ok := planValue["filters"].(map[string]any)
		if !ok || filters["no_tests"] != true {
			t.Fatalf("effective filters missing from plan: %#v", planValue["filters"])
		}
		if strings.Contains(string(planOut), `return "source-marker"`) {
			t.Fatalf("JSON plan unexpectedly included source content: %s", planOut)
		}
		assertPlanWorkspaceUntouched(t, root)
	})

	t.Run("side-effecting options are rejected before any write", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
			want string
		}{
			{name: "output", args: []string{"--output", "nested/out.md"}, want: "cannot be combined with --output"},
			{name: "clipboard", args: []string{"--copy"}, want: "cannot be combined with --copy"},
			{name: "check", args: []string{"--check"}, want: "cannot be combined with --check"},
			{name: "save view", args: []string{"--save-view", "saved"}, want: "cannot be combined with --save-view"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				root := planFixture(t)
				args := append([]string{"pack", "--target", "src", "--plan"}, tc.args...)
				_, stderr, err := runPlanBinary(t, binary, root, args...)
				if err == nil {
					t.Fatalf("expected plan conflict error, stderr: %s", stderr)
				}
				if !strings.Contains(stderr, tc.want) {
					t.Fatalf("stderr %q does not contain %q", stderr, tc.want)
				}
				assertPlanWorkspaceUntouched(t, root)
				if _, err := os.Stat(filepath.Join(root, "nested")); !os.IsNotExist(err) {
					t.Fatalf("plan created nested output directory: %v", err)
				}
			})
		}
	})

	t.Run("configured destination is explained but preserved", func(t *testing.T) {
		root := planFixture(t)
		config := "output: context.md\n"
		if err := os.WriteFile(filepath.Join(root, "nexss.srcpack.yml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		original := []byte("do not replace this file\n")
		if err := os.WriteFile(filepath.Join(root, "context.md"), original, 0o600); err != nil {
			t.Fatal(err)
		}
		state := []byte("opaque prior state\n")
		if err := os.WriteFile(filepath.Join(root, ".nexss.srcpack.state"), state, 0o600); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, err := runPlanBinary(t, binary, root,
			"pack", "--target", "src", "--plan", "--json")
		if err != nil {
			t.Fatalf("plan failed: %v\nstderr: %s", err, stderr)
		}
		var report map[string]any
		if err := json.Unmarshal(stdout, &report); err != nil {
			t.Fatalf("invalid plan JSON: %v\n%s", err, stdout)
		}
		if report["output_path"] != "context.md" {
			t.Fatalf("configured output path not resolved: %#v", report["output_path"])
		}
		gotOutput, err := os.ReadFile(filepath.Join(root, "context.md"))
		if err != nil || !bytes.Equal(gotOutput, original) {
			t.Fatalf("configured output changed: %q, %v", gotOutput, err)
		}
		gotState, err := os.ReadFile(filepath.Join(root, ".nexss.srcpack.state"))
		if err != nil || !bytes.Equal(gotState, state) {
			t.Fatalf("state cache changed: %q, %v", gotState, err)
		}
	})

	t.Run("resolution errors stay errors and do not fetch remotes", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
			want string
		}{
			{name: "unknown profile", args: []string{"pack", "--target", "src", "--profile", "not-a-profile", "--plan"}, want: "unknown profile"},
			{name: "remote full tree", args: []string{"pack", "--target", "github:nexssp/flow", "--full-tree", "--plan"}, want: "remote sources do not support --full-tree"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				root := planFixture(t)
				_, stderr, err := runPlanBinary(t, binary, root, tc.args...)
				if err == nil {
					t.Fatalf("expected resolution failure, stderr: %s", stderr)
				}
				if !strings.Contains(stderr, tc.want) {
					t.Fatalf("stderr %q does not contain %q", stderr, tc.want)
				}
				assertPlanWorkspaceUntouched(t, root)
				if _, err := os.Stat(filepath.Join(root, ".srcpack-remotes")); !os.IsNotExist(err) {
					t.Fatalf("plan unexpectedly materialized a remote source: %v", err)
				}
			})
		}
	})

	t.Run("help advertises both flag spellings", func(t *testing.T) {
		root := planFixture(t)
		stdout, stderr, err := runPlanBinary(t, binary, root, "pack", "--help")
		if err != nil {
			t.Fatalf("help failed: %v\nstderr: %s", err, stderr)
		}
		output := string(stdout) + stderr
		for _, want := range []string{"--plan", "--explain"} {
			if !strings.Contains(output, want) {
				t.Errorf("help missing %q:\n%s", want, output)
			}
		}
	})
}

func buildPlanTestBinary(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "../.."))
	binary := filepath.Join(t.TempDir(), "srcpack")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-o", binary, "./cmd/srcpack")
	cmd.Dir = moduleRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build srcpack binary: %v\n%s", err, output)
	}
	return binary
}

func runPlanBinary(t *testing.T, binary, root string, args ...string) ([]byte, string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	return stdout, stderr.String(), err
}

func planFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"src/main.go":      "package main\nfunc Public() string { return \"source-marker\" }\n",
		"src/main_test.go": "package main\nfunc TestPublic() {}\n",
		"vendor/dep.go":    "package vendor\nfunc Dependency() {}\n",
	}
	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func assertPlanWorkspaceUntouched(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"_srcpack", ".nexss.srcpack.state", ".srcpack.state"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Errorf("plan created %s: %v", name, err)
		}
	}
}
