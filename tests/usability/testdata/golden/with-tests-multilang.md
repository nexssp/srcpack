## go/output_actions.go
```go
package operators

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nexssp/flow/extensions/fs"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/terminal"
)

const (
	outputFieldContent = "content"
	outputFieldColor   = "color"
)

// OutputPolicy describes the already-resolved output behavior. Actions execute
// the selected operation; they do not choose which operations are appropriate.
type OutputPolicy struct {
	Mode            string `json:"mode"`
	SaveState       bool   `json:"save_state"`
	Path            string `json:"path"`
	Copy            bool   `json:"copy"`
	Quiet           bool   `json:"quiet"`
	Color           string `json:"color"`
	JSON            bool   `json:"json"`
	TrailingNewline bool   `json:"trailing_newline"`
	StartedAt       int64  `json:"started_at"`
}

// OutputEnvelope carries the content and policy through output actions.
type OutputEnvelope struct {
	Policy        OutputPolicy `json:"policy"`
	Content       string       `json:"content"`
	Status        string       `json:"status,omitempty"`
	RenderedItems int          `json:"rendered_items"`
	ReportPrefix  string       `json:"report_prefix,omitempty"`
	ReportSuffix  string       `json:"report_suffix,omitempty"`
}

// joinOutputAction retains empty-stream information without changing content.
func joinOutputAction(_ context.Context, files []fs.FileContent) (map[string]any, error) {
	return map[string]any{outputFieldContent: JoinRendered(files), "items": len(files)}, nil
}

// Flow predicates address JSON field names. Return explicit maps, not nested Go
// structs (expr does not apply json tags when traversing nested struct values).
func outputEffect(name string, effect func(context.Context, OutputEnvelope) (OutputEnvelope, error)) action.AnyAction {
	return action.New(name, func(ctx context.Context, input OutputEnvelope) (map[string]any, error) {
		out, err := effect(ctx, input)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			outputFieldContent: out.Content, "rendered_items": out.RenderedItems, "status": out.Status,
			"policy": map[string]any{
				"mode": out.Policy.Mode, "path": out.Policy.Path, "copy": out.Policy.Copy, "json": out.Policy.JSON, "save_state": out.Policy.SaveState,
				"quiet": out.Policy.Quiet, outputFieldColor: out.Policy.Color, "trailing_newline": out.Policy.TrailingNewline,
				"started_at": out.Policy.StartedAt,
			},
			"report_prefix": out.ReportPrefix, "report_suffix": out.ReportSuffix,
		}, nil
	}).Build()
}

// OutputActions exposes output effects and their data-only rendering adapter.
func OutputActions() []action.AnyAction {
	return outputActionsWithClipboardWriter(writeSystemClipboard)
}

func outputActionsWithClipboardWriter(writeClipboard func(string) error) []action.AnyAction {
	return []action.AnyAction{
		action.New("srcpack.join_output", joinOutputAction).Build(),
		outputEffect("srcpack.write_output", writeOutputAction),
		outputEffect("srcpack.check_output", checkOutputAction),
		outputEffect("srcpack.print_output", printOutputAction),
		outputEffect("srcpack.copy_output", func(ctx context.Context, envelope OutputEnvelope) (OutputEnvelope, error) {
			return copyOutputActionWithWriter(ctx, envelope, writeClipboard)
		}),
		outputEffect("srcpack.report_output", reportOutputAction),
	}
}

func writeOutputAction(_ context.Context, envelope OutputEnvelope) (OutputEnvelope, error) {
	parent := filepath.Dir(envelope.Policy.Path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return envelope, fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(envelope.Policy.Path, []byte(envelope.Content), 0o600); err != nil {
		return envelope, fmt.Errorf("write output: %w", err)
	}
	return envelope, nil
}

func checkOutputAction(_ context.Context, envelope OutputEnvelope) (OutputEnvelope, error) {
	current, err := os.ReadFile(envelope.Policy.Path)
	switch {
	case err != nil && !os.IsNotExist(err):
		return envelope, fmt.Errorf("read existing output: %w", err)
	case os.IsNotExist(err):
		envelope.Status = "NEW"
	case bytes.Equal(current, []byte(envelope.Content)):
		envelope.Status = "UNCHANGED"
	default:
		envelope.Status = "MODIFIED"
	}
	if envelope.Policy.JSON {
		payload, err := json.Marshal(struct {
			Status string `json:"status"`
			Path   string `json:"path"`
		}{Status: envelope.Status, Path: envelope.Policy.Path})
		if err != nil {
			return envelope, fmt.Errorf("encode JSON check result: %w", err)
		}
		if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
			return envelope, err
		}
		return envelope, nil
	}
	fmt.Fprintf(os.Stdout, "[%s] %s\n", envelope.Status, envelope.Policy.Path)
	return envelope, nil
}

func printOutputAction(_ context.Context, envelope OutputEnvelope) (OutputEnvelope, error) {
	content := envelope.Content
	if envelope.Policy.TrailingNewline {
		content += "\n"
	}
	if _, err := fmt.Fprint(os.Stdout, content); err != nil {
		return envelope, err
	}
	return envelope, nil
}

func copyOutputActionWithWriter(_ context.Context, envelope OutputEnvelope, writeClipboard func(string) error) (OutputEnvelope, error) {
	input := func(yield func(fs.FileContent, error) bool) {
		if envelope.RenderedItems > 0 {
			yield(fs.FileContent{Content: []byte(envelope.Content)}, nil)
		}
	}
	for _, err := range clipboardWithWriter(ClipboardConfig{Enabled: true, Quiet: envelope.Policy.Quiet}, writeClipboard)(input) {
		if err != nil {
			return envelope, err
		}
	}
	return envelope, nil
}

func reportOutputAction(_ context.Context, envelope OutputEnvelope) (OutputEnvelope, error) {
	if envelope.ReportPrefix == "" {
		return envelope, nil
	}
	duration := time.Since(time.Unix(0, envelope.Policy.StartedAt)).Round(time.Millisecond).String()
	status := envelope.ReportPrefix + duration
	color := terminal.ColorEnabled(envelope.Policy.Color, os.Stderr)
	if envelope.Policy.Copy && color {
		status = "\x1b[1;32m" + status + "\x1b[0m"
	}
	fmt.Fprint(os.Stderr, status, envelope.ReportSuffix)
	if envelope.Policy.Copy && envelope.RenderedItems > 0 {
		heading := "Preview:"
		if color {
			heading = "\x1b[1;36m" + heading + "\x1b[0m"
		}
		preview, truncated := boundedOutputPreview(envelope.Content)
		fmt.Fprintln(os.Stderr, heading)
		if preview == "" {
			fmt.Fprintln(os.Stderr, "(empty package)")
		} else {
			fmt.Fprintln(os.Stderr, preview)
		}
		if truncated {
			fmt.Fprintln(os.Stderr, "… preview truncated; only the preview is shortened, not the full destination output.")
		}
	}
	return envelope, nil
}
```
## go/output_actions_test.go
```go
package operators

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/flow/compiler"
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
```
## web/client.test.ts
```typescript
import { AccountClient } from "./client";

export async function accountClientRejectsFailedResponses(): Promise<void> {
  const client = new AccountClient(
    "https://api.example.test",
    () => "test-token",
    async () => new Response("", { status: 503 }),
  );

  await client.getAccount("acct-7").then(
    () => {
      throw new Error("expected a failed HTTP response to reject");
    },
    (error: unknown) => {
      if (!(error instanceof Error) || !error.message.includes("503")) {
        throw error;
      }
    },
  );
}
```
## web/client.ts
```typescript
export interface Account {
  id: string;
  displayName: string;
  email: string;
}

export interface AccountGateway {
  getAccount(id: string, signal?: AbortSignal): Promise<Account>;
}

export class AccountClient {
  constructor(
    private readonly baseURL: string,
    private readonly token: () => string,
    private readonly fetcher: typeof fetch = fetch,
  ) {}

  async getAccount(id: string, signal?: AbortSignal): Promise<Account> {
    const response = await this.fetcher(
      `${this.baseURL}/accounts/${encodeURIComponent(id)}`,
      {
        headers: { Authorization: `Bearer ${this.token()}` },
        signal,
      },
    );

    if (!response.ok) {
      throw new Error(`Account request failed: ${response.status}`);
    }

    return (await response.json()) as Account;
  }
}
```
## web/experimental.ts
```typescript
export function experimentalRetryDelay(attempt: number): number {
  const boundedAttempt = Math.max(0, Math.min(attempt, 8));
  const jitter = Math.random() * 125;
  return 250 * 2 ** boundedAttempt + jitter;
}

export function shouldRetry(status: number): boolean {
  return status === 408 || status === 429 || status >= 500;
}
```
