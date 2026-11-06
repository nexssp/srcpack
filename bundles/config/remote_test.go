package config

import (
	"strings"
	"testing"
)

func TestAuditTargetsRemoteSyntaxWithoutNetwork(t *testing.T) {
	report := ValidationReport{Passed: true}
	auditTargets(&report, Config{TargetsMap: map[string]TargetGroup{"remote": {Paths: []string{"github:example/nonexistent//src?ref=feature%2Fbranch"}, Files: []string{"github:example/nonexistent//README.md"}}}}, t.TempDir())
	if !report.Passed {
		t.Fatalf("valid remote treated as local: %#v", report)
	}
	found := false
	for _, issue := range report.Issues {
		if strings.Contains(issue.Message, "availability not checked") {
			found = true
		}
	}
	if !found {
		t.Fatal("audit must not imply remote availability was checked")
	}
	report = ValidationReport{Passed: true}
	auditTargets(&report, Config{TargetsMap: map[string]TargetGroup{"bad": {Paths: []string{"github:example/repo//../escape"}}}}, t.TempDir())
	if report.Passed {
		t.Fatal("invalid remote traversal accepted")
	}
}
