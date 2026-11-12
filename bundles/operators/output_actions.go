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
