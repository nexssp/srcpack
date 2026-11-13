package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xerr"
)

func TestMalformedSelectorOrderGetsFlowDiagnosticHint(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := runCommandEntrypoint(context.Background(), []string{"../flow#arch~*.nflow", "-c"})
	if err == nil {
		t.Fatal("runCommandEntrypoint succeeded, want invalid-profile error")
	}
	var appErr *xerr.AppError
	if !errors.As(err, &appErr) || appErr == nil || !strings.Contains(appErr.Message, `unknown profile "arch~*.nflow"`) {
		t.Fatalf("typed error = %#v", appErr)
	}
	diagnostic, ok := runner.DiagnosticOf(err)
	if !ok || diagnostic.Stage != "run" {
		t.Fatalf("diagnostic = %#v / present %v, want stage run", diagnostic, ok)
	}
	if diagnostic.Hint != selectorOrderHint {
		t.Fatalf("diagnostic hint = %q, want %q", diagnostic.Hint, selectorOrderHint)
	}

	var output strings.Builder
	writeCLIError(&output, err, false)
	userOutput := output.String()
	if !strings.Contains(userOutput, selectorOrderHint) || !strings.Contains(userOutput, "(stage: run)") {
		t.Fatalf("normal CLI output omits Flow stage or selector hint: %s", userOutput)
	}
	if !strings.Contains(userOutput, `unknown profile "arch~*.nflow"`) || strings.Contains(userOutput, "execution failed") {
		t.Fatalf("normal CLI summary lost the cause or retained Flow wrapper noise: %s", userOutput)
	}
}
