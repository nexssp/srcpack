package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Opt-in network smoke test; the normal test suite remains offline.
func TestFlowCLIRemoteLive(t *testing.T) {
	if os.Getenv("SRCPACK_REMOTE_LIVE") != "1" {
		t.Skip("set SRCPACK_REMOTE_LIVE=1 to test GitHub downloads")
	}
	root := materializeWorkspace(t, map[string]string{"local/local.go": "package local\nfunc Local() string { return \"LOCAL_BODY_MUST_BE_OMITTED\" }\n"})
	t.Chdir(root)
	args := []string{"github:nexssp/flow//compiler/errors.go,nexssp/kernel//action/action.go#sources", "local#arch", "--output", "remote.md", "--quiet", "--no-tests", "--no-header", "--no-prompt", "--no-start", "--no-end"}
	if err := execute(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("remote.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, marker := range []string{"github/nexssp/flow@", "github/nexssp/kernel@", "compiler/errors.go", "action/action.go", "Local"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("missing %q from output", marker)
		}
	}
	if strings.Contains(text, "LOCAL_BODY_MUST_BE_OMITTED") || strings.Contains(text, "srcpack-remote-") {
		t.Fatalf("profile or path leak in output: %s", text)
	}
	if err := execute(context.Background(), []string{"tree", "github:nexssp/kernel//action", "--tree-files", "--depth", "8", "--output", "tree.md", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	tree, err := os.ReadFile("tree.md")
	if err != nil || !strings.Contains(string(tree), "action.go") || !strings.Contains(string(tree), "kernel@") {
		t.Fatalf("remote tree failed: %v\n%s", err, tree)
	}
	yaml := "targets:\n  dependencies:\n    files:\n      - 'github:nexssp/kernel//action/action.go'\n"
	if err := os.WriteFile("nexss.srcpack.yml", []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(context.Background(), []string{"update", "arch"}); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile("_srcpack/dependencies_arch.md")
	if err != nil || !strings.Contains(string(updated), "github/nexssp/kernel@") {
		t.Fatalf("configured remote update failed: %v\n%s", err, updated)
	}
}
