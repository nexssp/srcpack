package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/bundles/config"
)

func TestFlowCLIRemoteViewSaveDoesNotDownload(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"local/local.go": "package local\n"})
	t.Chdir(root)
	// A successful save with no Git executable proves the save branch does not
	// materialize. Deliberately nonexistent repositories also require no login.
	t.Setenv("PATH", t.TempDir())
	args := []string{"pack", "github:srcpack-fixture/nonexistent,srcpack-fixture/another//pkg?ref=feature%2Fdemo~*_test.go#sources", "local#arch", "--save-view", "mixed"}
	if err := execute(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Views["mixed"]
	if len(got) != 2 || !strings.HasPrefix(got[0], "github:srcpack-fixture/nonexistent,github:srcpack-fixture/another//pkg?ref=feature%2Fdemo") || !strings.HasSuffix(got[0], "~*_test.go#source") || got[1] != "local#arch" {
		t.Fatalf("saved view corrupted: %#v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "_srcpack", "context.md")); !os.IsNotExist(err) {
		t.Fatalf("view save packed output: %v", err)
	}
}

func TestFlowCLISourcesAliasLocal(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"local/main.go": "package local\nfunc Main() string { return \"SOURCE_BODY\" }\n"})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"local#sources", "-o", "out.md", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("out.md")
	if err != nil || !strings.Contains(string(data), "SOURCE_BODY") {
		t.Fatalf("sources alias lost bodies: %v\n%s", err, data)
	}
}

func TestFlowCLIUpdateProfileUsesDerivedTargets(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"src/main.go":       "package sample\nfunc Main() string { return \"BODY_OMITTED\" }\n",
		"nexss.srcpack.yml": "targets:\n  application:\n    paths: [src]\n",
	})
	t.Chdir(root)
	if err := execute(context.Background(), []string{"update", "arch", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("_srcpack/application_arch.md")
	if err != nil || !strings.Contains(string(data), "Main") || strings.Contains(string(data), "BODY_OMITTED") {
		t.Fatalf("derived update failed: %v\n%s", err, data)
	}
}
