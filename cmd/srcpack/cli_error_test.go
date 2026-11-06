package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/expr-lang/expr/file"
	"github.com/nexssp/kernel/xerr"
)

func TestConciseCLIErrorPreservesSemanticContext(t *testing.T) {
	pathErr := &os.PathError{Op: "stat", Path: filepath.Join(t.TempDir(), "ćmd"), Err: os.ErrNotExist}
	cause := fmt.Errorf("target %q: %w", "ćmd", pathErr)
	synthetic := &file.Error{Message: cause.Error(), Snippet: "\n | __flow_projection_value_0__(ctx, root)", Prev: cause}
	wrapped := fmt.Errorf("action collect execution failed: %w", synthetic)
	wrapped = xerr.BadRequest("projection: evaluation failed", wrapped)
	for _, name := range []string{"projection.project", "pipe", "schema.wrap", "run"} {
		wrapped = fmt.Errorf("action %s execution failed: %w", name, wrapped)
	}
	if got := conciseCLIError(wrapped); got != cause.Error() {
		t.Fatalf("summary = %q, want %q", got, cause)
	}
	if !errors.Is(wrapped, os.ErrNotExist) {
		t.Fatal("original error chain lost")
	}
	var output bytes.Buffer
	writeCLIError(&output, wrapped, true)
	if got, want := output.String(), fmt.Sprintf("srcpack: %v\n", wrapped); got != want {
		t.Fatalf("debug output differs: %q", got)
	}
}

func TestConciseCLIErrorPreservesFlattenedTargetCause(t *testing.T) {
	message := "action run execution failed: action projection.project execution failed: [BadRequest] projection: evaluation failed: action collect execution failed: target \"ćmd\": stat /tmp/ćmd: no such file or directory (1:33)\n | __flow_projection_value_0__(ctx, root)"
	got := conciseCLIError(errors.New(message))
	want := "target \"ćmd\": stat /tmp/ćmd: no such file or directory"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestConciseCLIErrorDoesNotDiscardUnknownContext(t *testing.T) {
	cause := errors.New("permission denied")
	expression := &file.Error{Message: "invalid user expression", Snippet: "\n | foo / bar", Prev: cause}
	for _, err := range []error{
		fmt.Errorf("load configuration %q: %w", "project.yml", cause),
		xerr.NotFound("capability pipeline.output_policy not registered"),
		xerr.BadRequest("invalid profile", cause),
		expression,
		errors.Join(errors.New("first failure"), errors.New("second failure")),
	} {
		if got := conciseCLIError(err); got != err.Error() {
			t.Errorf("discarded useful context: %q -> %q", err, got)
		}
	}
}

func TestFlushTransportDiagnosticsPreservesHelp(t *testing.T) {
	err := xerr.BadRequest("invalid input", errors.New("actual cause"))
	var output bytes.Buffer
	flushTransportDiagnostics(&output, "Usage: srcpack pack\n"+fmt.Sprintf("Error: %s\n", xerr.Sprint(err)), fmt.Errorf("action run execution failed: %w", err))
	if got := output.String(); got != "Usage: srcpack pack\n" {
		t.Fatalf("help changed or duplicate retained: %q", got)
	}
	output.Reset()
	flushTransportDiagnostics(&output, "Error: unrelated warning\n", err)
	if output.String() != "Error: unrelated warning\n" {
		t.Fatalf("unrelated diagnostics lost: %q", output.String())
	}
}

func TestCLIMissingUnicodeTargetDisplay(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\nfunc Hello() {}\n"})
	t.Chdir(root)
	t.Setenv("NO_COLOR", "1")
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%v", debug), func(t *testing.T) {
			value := ""
			if debug {
				value = "1"
			}
			t.Setenv("SRCPACK_DEBUG", value)
			capture, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			oldArgs, oldStderr := os.Args, os.Stderr
			os.Args, os.Stderr = []string{"srcpack", ".,ćmd#source", "-c"}, capture
			defer func() { os.Args, os.Stderr = oldArgs, oldStderr }()
			if code := run(); code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			data, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			for _, want := range []string{`target "ćmd"`, filepath.Join(root, "ćmd")} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in %s", want, text)
				}
			}
			if debug {
				if !strings.Contains(text, "action run execution failed") {
					t.Fatalf("debug details missing: %s", text)
				}
			} else {
				for _, noise := range []string{"execution failed", "__flow_projection_", "Error: [BadRequest]"} {
					if strings.Contains(text, noise) {
						t.Fatalf("internal noise %q: %s", noise, text)
					}
				}
				if !strings.Contains(text, "SRCPACK_DEBUG=1") || strings.Count(text, "\n") > 3 {
					t.Fatalf("expected short diagnostic with debug hint: %s", text)
				}
			}
		})
	}
	if err := execute(context.Background(), []string{".,ćmd#source", "-c"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("execute must preserve original cause: %v", err)
	}
}
