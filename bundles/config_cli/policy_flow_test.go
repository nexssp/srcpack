package configcmd

import (
	"context"
	"testing"
)

// The Go adapter reports the missing config path; Flow owns the response policy.
func TestConfigInspectReportsMissingFileFacts(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := inspect(context.Background(), map[string]any{"sub": "validate"})
	if err != nil {
		t.Fatal(err)
	}
	if out["path"] != "" || out["sub"] != "validate" {
		t.Fatalf("inspect must return filesystem facts unchanged: %#v", out)
	}
}
