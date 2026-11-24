package operators

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/compiler/secret"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/nflows"
)

func captureOutput(t *testing.T, target **os.File, fn func()) string {
	t.Helper()
	old := *target
	r, w, err := os.Pipe()
	ktest.RequireNoError(t, err)
	*target = w
	fn()
	ktest.RequireNoError(t, w.Close())
	*target = old
	data, err := io.ReadAll(r)
	ktest.RequireNoError(t, err)
	ktest.RequireNoError(t, r.Close())
	return string(data)
}

func TestWriteOutputPreservesBytesWithoutFraming(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.bin")
	want := "raw\x00bytes\xff"
	got, err := writeOutputAction(context.Background(), OutputEnvelope{Policy: OutputPolicy{Path: path}, Content: want})
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, got.Content, want)
	data, err := os.ReadFile(path)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, string(data), want)
}

func TestCheckOutputStatusesAndDoesNotChangeTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	for _, test := range []struct{ existing, content, status string }{
		{"same", "same", "UNCHANGED"}, {"old", "new", "MODIFIED"}, {"", "new", "NEW"},
	} {
		if test.status == "NEW" {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte(test.existing), 0o600); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(path)
		stdout := captureOutput(t, &os.Stdout, func() {
			got, err := checkOutputAction(context.Background(), OutputEnvelope{Policy: OutputPolicy{Path: path}, Content: test.content})
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, got.Status, test.status)
		})
		ktest.RequireEqual(t, stdout, "["+test.status+"] "+path+"\n")
		if test.status == "NEW" {
			_, err := os.Stat(path)
			ktest.RequireCondition(t, os.IsNotExist(err), "check created missing target")
		} else {
			after, err := os.ReadFile(path)
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, string(after), string(before))
		}
	}
}

func TestCheckOutputFailOnChange(t *testing.T) {
	for _, test := range []struct {
		status   string
		existing string
		content  string
	}{
		{status: "UNCHANGED", existing: "same", content: "same"},
		{status: "MODIFIED", existing: "old", content: "new"},
		{status: "NEW", content: "new"},
	} {
		t.Run(test.status, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out")
			if test.status != "NEW" {
				if err := os.WriteFile(path, []byte(test.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var got OutputEnvelope
			var actionErr error
			stdout := captureOutput(t, &os.Stdout, func() {
				got, actionErr = checkOutputAction(context.Background(), OutputEnvelope{
					Policy: OutputPolicy{Path: path, FailOnChange: true}, Content: test.content,
				})
			})
			if got.Status != test.status {
				t.Fatalf("status = %q, want %q", got.Status, test.status)
			}
			if stdout != "["+test.status+"] "+path+"\n" {
				t.Fatalf("stdout = %q", stdout)
			}
			if test.status == "UNCHANGED" {
				if actionErr != nil {
					t.Fatalf("unchanged output returned error: %v", actionErr)
				}
				return
			}
			var drift *OutputDriftError
			if !errors.As(actionErr, &drift) {
				t.Fatalf("error = %v, want OutputDriftError", actionErr)
			}
			if drift.ExitCode() != 3 {
				t.Fatalf("exit code = %d, want 3", drift.ExitCode())
			}
		})
	}
}

func TestCheckOutputReadFailure(t *testing.T) {
	path := t.TempDir()
	_, err := checkOutputAction(context.Background(), OutputEnvelope{Policy: OutputPolicy{Path: path}})
	ktest.RequireCondition(t, err != nil, "expected read error for directory target")
}

func TestCopyOutputClipboardSuccessAndError(t *testing.T) {
	writeClipboard := func(value string) error {
		ktest.RequireEqual(t, value, "content")
		return nil
	}
	_, err := copyOutputActionWithWriter(context.Background(), OutputEnvelope{Policy: OutputPolicy{Quiet: true}, Content: "content", RenderedItems: 1}, writeClipboard)
	ktest.RequireNoError(t, err)
	wantErr := errors.New("clipboard unavailable")
	writeClipboard = func(string) error { return wantErr }
	_, err = copyOutputActionWithWriter(context.Background(), OutputEnvelope{Policy: OutputPolicy{Quiet: true}, Content: "content", RenderedItems: 1}, writeClipboard)
	ktest.RequireCondition(t, errors.Is(err, wantErr), "clipboard error was not propagated")
}

func TestPrintOutputWritesExactlyOnceWithoutNewline(t *testing.T) {
	stdout := captureOutput(t, &os.Stdout, func() {
		_, err := printOutputAction(context.Background(), OutputEnvelope{Content: "exact", Policy: OutputPolicy{TrailingNewline: false}})
		ktest.RequireNoError(t, err)
	})
	ktest.RequireEqual(t, stdout, "exact")
}

func TestReportOutputDoesNotPrintClipboardPreview(t *testing.T) {
	stderr := captureOutput(t, &os.Stderr, func() {
		reportOutputAction(context.Background(), OutputEnvelope{
			Policy:        OutputPolicy{Copy: true, Color: "never"},
			Content:       "package secret\nfunc Sensitive() {}\n",
			RenderedItems: 1,
			ReportPrefix:  "✓ Completed in ",
			ReportSuffix:  "\nOutput: full context copied to clipboard\n",
		})
	})
	ktest.RequireStringContains(t, stderr, "Output: full context copied to clipboard")
	if strings.Contains(stderr, "Preview:") || strings.Contains(stderr, "func Sensitive()") {
		t.Fatalf("clipboard status leaked a partial context preview: %q", stderr)
	}
}

func TestReportOutputShowsPreviewWhenRequested(t *testing.T) {
	stderr := captureOutput(t, &os.Stderr, func() {
		reportOutputAction(context.Background(), OutputEnvelope{
			Policy:        OutputPolicy{Copy: true, Preview: true, Color: "never"},
			Content:       "preview sentinel\n",
			RenderedItems: 1,
			ReportPrefix:  "done ",
		})
	})
	ktest.RequireStringContains(t, stderr, "Preview:")
	ktest.RequireStringContains(t, stderr, "preview sentinel")
}

func TestCopyOutputRetainsEmptyStreamSemantics(t *testing.T) {
	calls := 0
	writeClipboard := func(value string) error { calls++; ktest.RequireEqual(t, value, ""); return nil }
	for _, items := range []int{0, 1} {
		_, err := copyOutputActionWithWriter(context.Background(), OutputEnvelope{Policy: OutputPolicy{Quiet: true}, RenderedItems: items}, writeClipboard)
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, calls, items)
	}
}

func TestOutputActionsComposeThroughFlow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	calls := 0
	writeClipboard := func(value string) error {
		calls++
		data, err := os.ReadFile(path)
		ktest.RequireNoError(t, err)
		sanitized, _ := sanitizeClipboardText(string(data))
		ktest.RequireEqual(t, sanitized, value)
		return nil
	}
	cfg, err := runner.BuildConfig(append(native.Bundles(), compiler.Bundle{ID: "srcpack_output_test", Libraries: []action.Library{ioLibraryWithClipboardWriter(writeClipboard)}}))
	ktest.RequireNoError(t, err)
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	for _, tc := range []struct {
		content   string
		items     int
		wantCalls int
	}{
		{"raw\x00bytes\xff", 1, 1}, {"", 0, 1}, {"", 1, 2},
	} {
		_, err := runner.Execute(ctx, cfg, "@include \"output.nflow\"\noutput.pipeline.emit_output", "compose_output.nflow", map[string]any{
			"policy":  map[string]any{"mode": "file", "path": path, "copy": true, "quiet": true, "color": "", "started_at": int64(0), "trailing_newline": false},
			"content": tc.content, "rendered_items": tc.items,
		})
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, calls, tc.wantCalls)
		data, err := os.ReadFile(path)
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, string(data), tc.content)
	}
}

func TestOutputEffectsSanitizeDeclaredSecretBeforeEmission(t *testing.T) {
	const marker = "srcpack-output-redaction-marker"
	path := filepath.Join(t.TempDir(), "out.txt")
	var clipboardText string
	writeClipboard := func(value string) error {
		clipboardText = value
		return nil
	}
	makeOutput := action.New("test.make_output", func(_ context.Context, input map[string]any) (map[string]any, error) {
		value, ok := input["api_key"].(secret.Value)
		if !ok {
			return nil, fmt.Errorf("api_key type = %T, want secret.Value", input["api_key"])
		}
		var raw string
		if err := secret.WithValue(value, func(bytes []byte) error {
			raw = string(bytes)
			return nil
		}); err != nil {
			return nil, err
		}
		mode, _ := input["mode"].(string)
		return map[string]any{
			"content":        "generated from " + raw,
			"rendered_items": 1,
			"policy": map[string]any{
				"mode": mode, "path": path, "copy": mode == "clipboard", "preview": mode == "clipboard",
				"quiet": mode != "clipboard", "color": "never", "started_at": int64(0),
			},
		}, nil
	}).Build()
	bundle := compiler.Bundle{
		ID:        "srcpack-final-output-secret-test",
		Libraries: []action.Library{{Name: "test", Actions: []action.AnyAction{makeOutput}}},
		ArgSchemas: map[string][]compiler.ArgFieldSpec{
			"test.make_output": {
				{Name: "api_key", Kind: compiler.ArgSecret},
				{Name: "mode", Kind: compiler.ArgString},
			},
		},
	}
	cfg, err := runner.BuildConfig(append(native.Bundles(), compiler.Bundle{
		ID:        "srcpack-final-output-test",
		Libraries: []action.Library{ioLibraryWithClipboardWriter(writeClipboard)},
	}, bundle))
	ktest.RequireNoError(t, err)
	source := `@include "output.nflow"
@schema Request {
  APIKey secret ` + "`json:\"api_key\"`" + `
  Mode string ` + "`json:\"mode\"`" + `
}
@pipeline emit_output
  :request-schema=Request
  test.make_output(api_key: .api_key, mode: .mode)
  -> output.pipeline.emit_output
@end
pipeline.emit_output
`
	ctx := compiler.WithSourceLoader(t.Context(), nflows.Loader())
	for _, mode := range []string{"file", "stdout", "clipboard"} {
		payload := map[string]any{"api_key": marker, "mode": mode}
		var stderr string
		var runErr error
		switch mode {
		case "stdout":
			stdout := captureOutput(t, &os.Stdout, func() {
				_, runErr = runner.Execute(ctx, cfg, source, "final_output_secret.nflow", payload)
			})
			if strings.Contains(stdout, marker) || !strings.Contains(stdout, secret.Redacted) {
				t.Fatalf("stdout emission = %q, want Secret redacted", stdout)
			}
		case "clipboard":
			stderr = captureOutput(t, &os.Stderr, func() {
				_, runErr = runner.Execute(ctx, cfg, source, "final_output_secret.nflow", payload)
			})
			if strings.Contains(clipboardText, marker) || !strings.Contains(clipboardText, secret.Redacted) {
				t.Fatalf("clipboard emission = %q, want Secret redacted", clipboardText)
			}
			if strings.Contains(stderr, marker) || !strings.Contains(stderr, secret.Redacted) {
				t.Fatalf("preview emission = %q, want Secret redacted", stderr)
			}
		default:
			_, runErr = runner.Execute(ctx, cfg, source, "final_output_secret.nflow", payload)
			data, readErr := os.ReadFile(path)
			ktest.RequireNoError(t, readErr)
			if strings.Contains(string(data), marker) || !strings.Contains(string(data), secret.Redacted) {
				t.Fatalf("file emission = %q, want Secret redacted", data)
			}
		}
		ktest.RequireNoError(t, runErr)
	}
}
