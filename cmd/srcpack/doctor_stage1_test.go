package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/bundles/config"
)

// Doctor's Flow orchestration must retain the existing audit/report contract.
// These run through the real CLI entry point, not just the new primitives.
func TestDoctorStage1CLIReportParity(t *testing.T) {
	cases := []struct {
		name, yaml string
		files      map[string]string
	}{
		{"empty-config", "{}\n", nil},
		{"empty-document", "", nil},
		{"null-document", "null\n", nil},
		{"null-optional-sections", "targets: null\nprofiles: null\noptions: null\nfiles: null\n", nil},
		{"numeric-prompt-key", "prompts:\n  123: example\n", nil},
		{"numeric-prompt-value", "prompts:\n  example: 123\n", nil},
		{"yaml-alias", "targets:\n  first: &entry\n    paths: [.]\n  second: *entry\n", nil},
		{"existing-file-and-dir", "targets:\n  code:\n    paths: [src]\n    files: [src/a.go]\n", map[string]string{"src/a.go": "package a\n"}},
		{"unsorted-targets", "targets:\n  zebra:\n    paths: [missing-z]\n  alpha:\n    paths: [missing-a]\n", nil},
		{"missing-and-output-paths", "targets:\n  broken:\n    paths: [missing, _srcpack/out.md]\n", nil},
		{"undefined-target-and-cycle", "targets:\n  first:\n    targets: [second, absent]\n  second:\n    targets: [first]\n", nil},
		{"shared-child-is-not-cycle", "targets:\n  a:\n    targets: [b, c]\n  b:\n    targets: [d]\n  c:\n    targets: [d]\n  d:\n    paths: [.]\n", nil},
		{"remote-syntax-only", "targets:\n  remote:\n    paths: [github:nexssp/flow//compiler]\n", nil},
		{"undefined-prompt-is-warning", "profiles:\n  custom:\n    prompt: unknown-prompt\n", nil},
		{"dir-entry-aliases", "targets:\n  mixed:\n    dir:\n      - src\n      - path: other\n    files: [src/a.go]\n", map[string]string{"src/a.go": "package a\n", "other/b.go": "package b\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := materializeWorkspace(t, tc.files)
			t.Chdir(root)
			path := filepath.Join(root, "nexss.srcpack.yml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := config.Validate(root, path)
			stdout, runErr := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"doctor"}) })
			if (runErr == nil) != want.Passed {
				t.Fatalf("passed=%v error=%v\n%s", want.Passed, runErr, stdout)
			}
			if stdout != want.String() {
				t.Fatalf("report changed:\ngot: %s\nwant: %s", stdout, want.String())
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("doctor modified the configuration")
			}
		})
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
	want := config.Validate(filepath.Join(root, "nested"), path)
	stdout, err := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"doctor"}) })
	if err != nil || stdout != want.String() {
		t.Fatalf("config-relative audit: %v\ngot: %s\nwant: %s", err, stdout, want.String())
	}
}
