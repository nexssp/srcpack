package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/expr-lang/expr/file"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xerr"
)

// writeCLIError changes presentation only. The original error and its complete
// unwrap chain remain available to callers and in SRCPACK_DEBUG=1 mode.
func writeCLIError(w io.Writer, err error, debug bool) {
	if err == nil {
		return
	}
	if debug {
		fmt.Fprintf(w, "srcpack: %v\n", err)
		return
	}
	diagnostic, hasDiagnostic := runner.DiagnosticOf(err)
	summary := conciseCLIError(err)
	if hasDiagnostic {
		if hint := strings.TrimSpace(diagnostic.Hint); hint != "" {
			if message := strings.TrimSpace(diagnostic.Message); message != "" {
				summary = message
			}
			if stage := strings.TrimSpace(diagnostic.Stage); stage != "" {
				summary += " (stage: " + stage + ")"
			}
			summary += "\n  hint: " + hint
		}
	}
	fmt.Fprintf(w, "srcpack: %s\n", summary)
	if summary != err.Error() {
		fmt.Fprintln(w, "  Full diagnostics: set SRCPACK_DEBUG=1 and rerun the command.")
	}
}

// Strip only known execution plumbing, never arbitrary semantic wrappers or the
// deepest cause alone: e.g. `target "ćmd"` and the OS path must survive together.
func conciseCLIError(err error) string {
	for err != nil {
		cause := errors.Unwrap(err)
		if cause == nil {
			if summary := conciseFlattenedTargetError(err.Error()); summary != "" {
				return summary
			}
			return err.Error()
		}
		//nolint:errorlint // Exact-node matching keeps unrelated semantic wrappers intact.
		switch e := err.(type) {
		case *file.Error:
			// Projection rewriting synthesizes this expression. It isn't the
			// user's source location; real expression errors keep their snippet.
			if strings.Contains(e.Snippet, "__flow_projection_") {
				err = cause
				continue
			}
		case *xerr.AppError:
			if e.Message == "projection: evaluation failed" {
				err = cause
				continue
			}
		case *runner.DiagnosticError:
			err = cause
			continue
		}
		message := err.Error()
		prefix, ok := strings.CutSuffix(message, cause.Error())
		if ok && strings.HasPrefix(prefix, "action ") && strings.HasSuffix(prefix, " execution failed: ") {
			name := strings.TrimSuffix(strings.TrimPrefix(prefix, "action "), " execution failed: ")
			if name != "" && !strings.ContainsAny(name, " \t\r\n:") {
				err = cause
				continue
			}
		}
		return message
	}
	return "unknown error"
}

// isCLIUsageError recognizes only request-parse BadRequests emitted by tcli.
// Domain validation/runtime BadRequests must retain the ordinary status 1.
func isCLIUsageError(err error) bool {
	// Check each wrapper because errors.As can find an outer runtime AppError before an inner CLI usage error.
	for current := err; current != nil; current = errors.Unwrap(current) {
		var appErr *xerr.AppError
		if !errors.As(current, &appErr) {
			continue
		}
		message := appErr.Message
		if strings.HasPrefix(message, "unknown flag: -") ||
			strings.HasPrefix(message, "unexpected positional argument ") ||
			strings.HasPrefix(message, "missing required CLI field ") ||
			(strings.HasPrefix(message, "flag -") && strings.Contains(message, " requires a value")) {
			return true
		}
	}
	return false
}

// Some Flow versions return a flattened pipeline error rather than preserving
// every Unwrap link. Keep the user-facing target and filesystem cause in that
// case, while still removing the surrounding execution/projection diagnostics.
func conciseFlattenedTargetError(message string) string {
	if !strings.Contains(message, "execution failed") {
		return ""
	}
	start := strings.LastIndex(message, `target "`)
	if start < 0 {
		return ""
	}
	summary := message[start:]
	if strings.Contains(summary, "execution failed") {
		return ""
	}
	if newline := strings.IndexByte(summary, '\n'); newline >= 0 {
		summary = summary[:newline]
	}
	if open := strings.LastIndex(summary, " ("); open >= 0 && strings.HasSuffix(summary, ")") {
		location := summary[open+2 : len(summary)-1]
		line, column, ok := strings.Cut(location, ":")
		if ok && digitsOnly(line) && digitsOnly(column) {
			summary = summary[:open]
		}
	}
	return summary
}

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// tcli is intentionally generic and also prints returned errors. Suppress only
// its exact trailing duplicate; keep help, warnings and unrelated diagnostics.
func flushTransportDiagnostics(w io.Writer, text string, err error) {
	for current := err; current != nil; current = errors.Unwrap(current) {
		duplicate := fmt.Sprintf("Error: %s\n", xerr.Sprint(current))
		if trimmed, ok := strings.CutSuffix(text, duplicate); ok {
			text = trimmed
			break
		}
	}
	fmt.Fprint(w, text)
}
