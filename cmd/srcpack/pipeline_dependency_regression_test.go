package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestPackExecutePipelineDependencyRegression exercises the real command entry
// point rather than compiling an isolated pipeline snippet. The production
// pack_execute pipeline is declared in bundles/pack/pack.nflow and references
// pipeline.output_policy from the included output.nflow. A missing pipeline
// registration therefore fails during sub-pipeline materialization before the
// output file can be produced.
func TestPackExecutePipelineDependencyRegression(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"main.go": "package sample\n\nfunc Example() string { return \"pipeline dependency\" }\n",
	})
	t.Chdir(root)

	if err := execute(context.Background(), []string{
		"pack", "--file", "main.go", "--output", "out.md", "--quiet",
	}); err != nil {
		t.Fatalf("execute production pack pipeline: %v", err)
	}

	output, err := os.ReadFile("out.md")
	if err != nil {
		t.Fatalf("read output generated through pack_execute: %v", err)
	}
	if !strings.Contains(string(output), "func Example() string") {
		t.Fatalf("pack output does not contain the source content: %q", output)
	}
}
