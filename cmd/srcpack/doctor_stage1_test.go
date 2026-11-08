package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDoctorFlowReportsMissingTargetExpectedOutput locks the modern NFlow audit
// contract to concrete output instead of comparing it with the legacy validator.
func TestDoctorFlowReportsMissingTargetExpectedOutput(t *testing.T) {
	root := materializeWorkspace(t, nil)
	t.Chdir(root)
	if err := os.WriteFile("nexss.srcpack.yml", []byte("targets:\n  broken:\n    paths: [missing]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "\n🩺 Srcpack doctor\n" + strings.Repeat("─", 45) + "\n" +
		"✅ [config ] loaded nexss.srcpack.yml\n" +
		"❌ [target ] broken references missing path: missing\n" +
		strings.Repeat("─", 45) + "\nErrors found. Resolve issues above.\n\n"
	stdout, err := capturePolicyStdout(t, func() error {
		return execute(context.Background(), []string{"doctor"})
	})
	if err == nil {
		t.Fatal("doctor accepted a missing target path")
	}
	if stdout != want {
		t.Fatalf("modern doctor report changed:\ngot:  %q\nwant: %q", stdout, want)
	}
}

func TestDoctorStage1MissingAndMalformedConfig(t *testing.T) {
	for _, tc := range []struct {
		name, yaml string
		missing    bool
	}{
		{"missing", "", true},
		{"bad-yaml", "targets: [\n", false},
		{"wrong-target-shape", "targets: not-a-map\n", false},
		{"wrong-options-type", "options:\n  max_lines: [not, an, integer]\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if !tc.missing {
				if err := os.WriteFile("nexss.srcpack.yml", []byte(tc.yaml), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			stdout, err := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"doctor"}) })
			if err == nil {
				t.Fatalf("invalid configuration accepted: %s", stdout)
			}
			if tc.missing {
				if !strings.Contains(err.Error(), "no config file found") {
					t.Fatalf("missing config: %v", err)
				}
			} else if !strings.Contains(stdout, "[config ") || strings.Contains(stdout, "All checks passed") {
				t.Fatalf("invalid config lost doctor report: %s (%v)", stdout, err)
			}
		})
	}
}

func TestDoctorStage1RawShapeIsRejectedByNFlowSchema(t *testing.T) {
	for _, body := range []string{"targets: wrong\n", "profiles: []\n", "files: wrong\n", "false\n"} {
		t.Run(body, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile("nexss.srcpack.yml", []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			stdout, err := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"doctor"}) })
			if err == nil || !strings.Contains(stdout, "schema DoctorRawConfigShape failed") {
				t.Fatalf("raw shape did not go through @schema: %v\n%s", err, stdout)
			}
			if strings.Count(stdout, "[config ") != 1 || strings.Contains(stdout, "[target ") {
				t.Fatalf("shape failure did not stop audit: %s", stdout)
			}
		})
	}
}

func TestDoctorStage1PathsAreRelativeToConfigNotWorkingDirectory(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"src/a.go": "package a\n", "nested/keep.txt": "keep\n"})
	path := filepath.Join(root, "nexss.srcpack.yml")
	if err := os.WriteFile(path, []byte("targets:\n  code:\n    paths: [src]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "nested"))
	want := "\n🩺 Srcpack doctor\n" + strings.Repeat("─", 45) + "\n" +
		"✅ [config ] loaded nexss.srcpack.yml\n" +
		"✅ [target ] code: valid\n" + strings.Repeat("─", 45) + "\nAll checks passed.\n\n"
	stdout, err := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"doctor"}) })
	if err != nil || stdout != want {
		t.Fatalf("config-relative audit: %v\ngot: %q\nwant: %q", err, stdout, want)
	}
}
