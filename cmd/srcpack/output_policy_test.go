package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/operators"
	"github.com/nexssp/srcpack/nflows"
)

func capturePolicyStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	ktest.RequireNoError(t, err)
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original; _ = reader.Close(); _ = writer.Close() }()
	result := make(chan string, 1)
	go func() { data, _ := io.ReadAll(reader); result <- string(data) }()
	runErr := f()
	_ = writer.Close()
	os.Stdout = original
	return <-result, runErr
}

func TestCheckPolicyPreservesStateAndTarget(t *testing.T) {
	for _, command := range []string{"pack", "tree"} {
		for _, existing := range []bool{false, true} {
			name := command + "/new"
			if existing {
				name = command + "/existing"
			}
			t.Run(name, func(t *testing.T) {
				root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Preview() {}\n"})
				t.Chdir(root)
				const oldState = "preserve cache byte for byte\n"
				if existing {
					ktest.RequireNoError(t, os.WriteFile(operators.StateFileName, []byte(oldState), 0o600))
					ktest.RequireNoError(t, os.WriteFile("out.md", []byte("old target\n"), 0o600))
				}
				stdout, err := capturePolicyStdout(t, func() error {
					return execute(context.Background(), []string{command, "-f", "main.go", "-o", "out.md", "--check", "-q"})
				})
				ktest.RequireNoError(t, err)
				want := "[NEW] out.md\n"
				if existing {
					want = "[MODIFIED] out.md\n"
				}
				ktest.RequireEqual(t, stdout, want)
				if existing {
					data, err := os.ReadFile(operators.StateFileName)
					ktest.RequireNoError(t, err)
					ktest.RequireEqual(t, string(data), oldState)
					data, err = os.ReadFile("out.md")
					ktest.RequireNoError(t, err)
					ktest.RequireEqual(t, string(data), "old target\n")
				} else {
					_, err := os.Stat(operators.StateFileName)
					ktest.RequireCondition(t, os.IsNotExist(err), "check created state: %v", err)
					_, err = os.Stat("out.md")
					ktest.RequireCondition(t, os.IsNotExist(err), "check created target: %v", err)
				}
				temps, err := filepath.Glob(".srcpack-check-*")
				ktest.RequireNoError(t, err)
				ktest.RequireEqual(t, len(temps), 0)
			})
		}
	}
}

func TestCheckUnchangedAndNestedDestination(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Stable() {}\n"})
	t.Chdir(root)
	cli, err := parseCLI([]string{"pack", "-f", "main.go", "-o", "out.md", "-q"})
	ktest.RequireNoError(t, err)
	run, err := resolve(context.Background(), cli, time.Unix(1234567890, 0))
	ktest.RequireNoError(t, err)
	_, err = executeRootPipeline(context.Background(), cli, run)
	ktest.RequireNoError(t, err)
	state, err := os.ReadFile(operators.StateFileName)
	ktest.RequireNoError(t, err)
	cli.Check = true
	stdout, err := capturePolicyStdout(t, func() error { _, e := executeRootPipeline(context.Background(), cli, run); return e })
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, stdout, "[UNCHANGED] out.md\n")
	after, err := os.ReadFile(operators.StateFileName)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, string(after), string(state))
	run.OutputPath = "missing/nested/out.md"
	stdout, err = capturePolicyStdout(t, func() error { _, e := executeRootPipeline(context.Background(), cli, run); return e })
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, stdout, "[NEW] missing/nested/out.md\n")
	_, err = os.Stat("missing")
	ktest.RequireCondition(t, os.IsNotExist(err), "check created destination directory")
}

func TestOutputPolicyNoCopyAndExplicitDestinations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		stdout bool
		file   string
	}{
		{"no-copy shorthand uses stdout", []string{"-f", "main.go", "--no-copy", "-q"}, true, ""},
		{"no-copy explicit pack uses stdout", []string{"pack", "-f", "main.go", "--no-copy", "-q"}, true, ""},
		{"stdout flag prints the full package", []string{"pack", "-f", "main.go", "--stdout", "-q"}, true, ""},
		{"stdout flag accepts output sentinel", []string{"pack", "-f", "main.go", "--stdout", "-o", "-", "-q"}, true, ""},
		{"explicit output writes file", []string{"pack", "-f", "main.go", "-o", "out.md", "-q"}, false, "out.md"},
		{"stdout sentinel", []string{"pack", "-f", "main.go", "-o", "-", "-q"}, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc ExactlyOnce() {}\n"})
			t.Chdir(root)
			stdout, err := capturePolicyStdout(t, func() error { return execute(context.Background(), tc.args) })
			ktest.RequireNoError(t, err)
			if tc.stdout {
				ktest.RequireEqual(t, strings.Count(stdout, "func ExactlyOnce()"), 1)
			}
			if tc.file != "" {
				ktest.RequireEqual(t, stdout, "")
				_, err = os.Stat(tc.file)
				ktest.RequireNoError(t, err)
			} else {
				_, err = os.Stat("_srcpack/context.md")
				ktest.RequireCondition(t, os.IsNotExist(err), "implicit output created a file")
			}
		})
	}
}

func TestOutputStdoutRejectsFileDestination(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"stdout before file", []string{"pack", "-f", "main.go", "--stdout", "-o", "out.md", "-q"}},
		{"file before stdout", []string{"pack", "-f", "main.go", "-o", "out.md", "--stdout", "-q"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc ExactlyOnce() {}\n"})
			t.Chdir(root)
			err := execute(context.Background(), tc.args)
			if err == nil || !strings.Contains(err.Error(), "--stdout conflicts with -o FILE") {
				t.Fatalf("execute() error = %v, want stdout/file conflict", err)
			}
			if _, err := os.Stat("out.md"); !os.IsNotExist(err) {
				t.Fatalf("conflicting output created out.md: %v", err)
			}
		})
	}
}

func TestUpdateCheckPolicyDoesNotWriteStateOrTargets(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc UpdatePreview() {}\n", "nexss.srcpack.yml": "targets:\n  app:\n    paths: [.]\n"})
	t.Chdir(root)
	stdout, err := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"update", "--check"}) })
	ktest.RequireNoError(t, err)
	ktest.RequireStringContains(t, stdout, "[NEW]")
	ktest.RequireStringContains(t, stdout, "Summary: 1 checked")
	_, err = os.Stat(operators.StateFileName)
	ktest.RequireCondition(t, os.IsNotExist(err), "update check created state")
	_, err = os.Stat("_srcpack")
	ktest.RequireCondition(t, os.IsNotExist(err), "update check created output")
}

func TestNFlowSelectsOutputEffects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, mode string
		copy, fail bool
		want       []string
	}{
		{name: "file", mode: "file", want: []string{"write_output", "report_output"}},
		{name: "file and clipboard", mode: "file", copy: true, want: []string{"write_output", "copy_output", "report_output"}},
		{name: "check and clipboard compatibility", mode: "check", copy: true, want: []string{"check_output", "copy_output", "report_output"}},
		{name: "clipboard only", mode: "clipboard", copy: true, want: []string{"copy_output", "report_output"}},
		{name: "stdout", mode: "stdout", want: []string{"print_output", "report_output"}},
		{name: "failed file stops clipboard and success", mode: "file", copy: true, fail: true, want: []string{"write_output"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var called []string
			var actions []action.AnyAction
			for _, name := range []string{"write_output", "check_output", "print_output", "copy_output", "report_output"} {
				actions = append(actions, action.New("srcpack."+name, func(_ context.Context, in operators.OutputEnvelope) (map[string]any, error) {
					called = append(called, name)
					if tc.fail && name == "write_output" {
						return nil, errors.New("simulated write failure")
					}
					return jsonObject(in)
				}).Build())
			}
			bundles := append(native.Bundles(), compiler.Bundle{ID: "output_effect_spies", Libraries: []action.Library{{Name: "output.effect.spies", Actions: actions}}})
			cfg, err := runner.BuildConfig(bundles)
			ktest.RequireNoError(t, err)
			ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
			payload, err := jsonObject(operators.OutputEnvelope{Policy: operators.OutputPolicy{Mode: tc.mode, Copy: tc.copy}, Content: "content", RenderedItems: 1})
			ktest.RequireNoError(t, err)
			_, err = runner.Execute(ctx, cfg, "@include \"output.nflow\"\noutput.pipeline.emit_output", "emit.nflow", payload)
			if tc.fail {
				ktest.RequireCondition(t, err != nil, "write failure must propagate")
			} else {
				ktest.RequireNoError(t, err)
			}
			ktest.RequireEqual(t, called, tc.want)
		})
	}
}

func TestCheckAliasesAndFailurePreserveState(t *testing.T) {
	for _, flag := range []string{"--check", "--diff", "--dry-run"} {
		t.Run(flag, func(t *testing.T) {
			root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc PreviewAlias() {}\n"})
			t.Chdir(root)
			const state = "saved state must survive failures\n"
			ktest.RequireNoError(t, os.WriteFile(operators.StateFileName, []byte(state), 0o600))
			ktest.RequireNoError(t, os.Mkdir("destination", 0o700))
			stdout, err := capturePolicyStdout(t, func() error {
				return execute(context.Background(), []string{"pack", "-f", "main.go", "-o", "destination", flag, "-q"})
			})
			ktest.RequireCondition(t, err != nil, "directory comparison must fail")
			ktest.RequireEqual(t, stdout, "")
			after, err := os.ReadFile(operators.StateFileName)
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, string(after), state)
		})
	}
}
