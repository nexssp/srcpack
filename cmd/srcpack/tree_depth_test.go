package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestTreeDepthCLIParsing(t *testing.T) {
	cli, err := parseCLI(nil)
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, cli.Config.Depth, 1)
	ktest.RequireCondition(t, cli.Overrides.TreeDepth == nil, "default CLI depth must not masquerade as an explicit override")

	for _, tc := range []struct {
		flag string
		want int
	}{
		{"--tree-depth", 4}, {"--depth", 4}, {"-L", 4}, {"-l", 4},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			got, err := parseCLI([]string{tc.flag, strconv.Itoa(tc.want)})
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, got.Config.Depth, tc.want)
			ktest.RequireCondition(t, got.Overrides.TreeDepth != nil, "missing TreeDepth override")
			ktest.RequireEqual(t, *got.Overrides.TreeDepth, tc.want)
		})
	}

	for _, tc := range []struct {
		flag string
		val  string
	}{
		{"--tree-depth", "0"}, {"--depth", "-1"}, {"-L", "nope"}, {"-l", "0"},
	} {
		t.Run(tc.flag+"="+tc.val, func(t *testing.T) {
			_, err := parseCLI([]string{tc.flag, tc.val})
			ktest.RequireCondition(t, err != nil, "invalid depth %q was accepted", tc.val)
		})
	}
}

func TestResolveTreeDepthPrecedenceAndOperatorWiring(t *testing.T) {
	root := t.TempDir()
	const cfg = "options:\n  tree_depth: 3\nprofiles:\n  deep:\n    options:\n      tree_depth: 5\n"
	ktest.RequireNoError(t, os.WriteFile(filepath.Join(root, "nexss.srcpack.yml"), []byte(cfg), 0o600))
	t.Chdir(root)

	for _, tc := range []struct {
		name    string
		profile string
		args    []string
		want    int
	}{
		{name: "config", want: 3},
		{name: "profile", profile: "deep", want: 5},
		{name: "cli wins profile", profile: "deep", args: []string{"--tree-depth", "7"}, want: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli, err := parseCLI(tc.args)
			ktest.RequireNoError(t, err)
			cli.Profile = tc.profile
			got, err := resolve(context.Background(), cli, time.Unix(0, 0))
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, got.Options.TreeDepth, tc.want)
			ktest.RequireEqual(t, got.Stats.Depth, strconv.Itoa(tc.want))
			ktest.RequireEqual(t, got.Render.TreeDepth, strconv.Itoa(tc.want))
		})
	}
}
