package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/srcpack/bundles/config"
)

// Config shape/type errors are raised by the YAML decoder used by Load,
// before doctor audits target paths or profiles. Keep this report boundary
// stable: @schema validates Flow payloads, not a YAML file read later by Go.
func TestValidateShapeErrorRemainsSingleConfigLoadIssue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, config.ConfigFileName)
	if err := os.WriteFile(path, []byte("targets:\n  backend: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report := config.Validate(root, path)
	if report.Passed {
		t.Fatal("malformed target shape unexpectedly passed")
	}
	if len(report.Issues) != 1 {
		t.Fatalf("issues = %#v, want exactly the load error", report.Issues)
	}
	issue := report.Issues[0]
	if issue.Severity != config.SeverityError || issue.Category != "config" {
		t.Fatalf("load issue = %#v, want ERROR/config", issue)
	}
	if got := issue.Message; len(got) < len("failed to load: ") || got[:len("failed to load: ")] != "failed to load: " {
		t.Fatalf("load message = %q, want the existing failed-to-load prefix", got)
	}
	if report.ConfigFile != path {
		t.Fatalf("ConfigFile = %q, want %q", report.ConfigFile, path)
	}
}
