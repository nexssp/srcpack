package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/config"
)

func TestTreeDepthProductionRoot(t *testing.T) {
	for _, mode := range []struct {
		name string
		args []string
	}{
		{"document context", []string{"pack"}},
		{"document full", []string{"pack", "--full-tree"}},
		{"tree context", []string{"tree"}},
		{"tree full", []string{"tree", "--full-tree"}},
	} {
		for _, tc := range []struct {
			name, yaml string
			args       []string
			want       int
		}{
			{name: "default", want: config.DefaultTreeDepth},
			{name: "explicit", args: []string{"--tree-depth", "2"}, want: 2},
			{name: "depth alias", args: []string{"--depth", "3"}, want: 3},
			{name: "uppercase alias", args: []string{"-L", "2"}, want: 2},
			{name: "lowercase alias", args: []string{"-l", "3"}, want: 3},
			{name: "last flag wins", args: []string{"--tree-depth", "3", "-L", "1"}, want: 1},
			{name: "global config", yaml: "options:\n  tree_depth: 2\n", want: 2},
			{name: "cli wins config", yaml: "options:\n  tree_depth: 3\n", args: []string{"-L", "1"}, want: 1},
		} {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				files := map[string]string{"top/second/third/leaf.go": "package sample\nfunc Public() {}\n"}
				yaml := tc.yaml
				if mode.name == "document context" {
					if yaml == "" {
						yaml = "options:\n"
					}
					yaml = strings.Replace(yaml, "options:\n", "options:\n  tree: true\n", 1)
				}
				if yaml != "" {
					files["nexss.srcpack.yml"] = yaml
				}
				t.Chdir(materializeWorkspace(t, files))
				args := append([]string{}, mode.args...)
				args = append(args, "--target", "top", "--output", "_srcpack/depth.md", "--quiet")
				args = append(args, tc.args...)
				ktest.RequireNoError(t, execute(context.Background(), args))
				data, err := os.ReadFile("_srcpack/depth.md")
				ktest.RequireNoError(t, err)
				output := string(data)
				ktest.RequireCondition(t, strings.Contains(output, "📁 top/"), "first level missing from root output")
				ktest.RequireCondition(t, strings.Contains(output, "📁 second/") == (tc.want >= 2), "second level visibility must match configured depth")
				ktest.RequireCondition(t, strings.Contains(output, "📁 third/") == (tc.want >= 3), "third level visibility must match configured depth")
			})
		}
	}
}

func TestTreeDepthHelpUsesSharedDefault(t *testing.T) {
	var out bytes.Buffer
	printHelp(&out)
	ktest.RequireCondition(t, strings.Contains(out.String(), "Folder tree depth (default: 1)"), "help must show the shared default")
}
