package main

import (
	"context"
	"strings"
	"testing"
)

func TestUpdateFailurePrintsReportBeforeReturningError(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"nexss.srcpack.yml": "targets:\n  broken:\n    paths: [missing-directory]\n",
	})
	t.Chdir(root)
	stdout, err := capturePolicyStdout(t, func() error {
		return execute(context.Background(), []string{"update", "--check"})
	})
	if err == nil || !strings.Contains(err.Error(), "1 target(s) failed") {
		t.Fatalf("Flow failure = %v", err)
	}
	if !strings.Contains(stdout, "broken") {
		t.Fatalf("update report omitted the target name: %s", stdout)
	}
	if stdout == "" {
		t.Fatal("update returned the target error before printing its report")
	}
}
