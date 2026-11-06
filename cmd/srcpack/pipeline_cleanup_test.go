package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestRunOneUpdateNamedTargetWritesExpectedOutput(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"main.go":           "package main\n\nfunc NamedUpdateMarker() {}\n",
		"nexss.srcpack.yml": "targets:\n  app:\n    paths: [. ]\n",
	})
	t.Chdir(root)

	result := runOneUpdate(context.Background(), root, "app", "", false, true, time.Unix(123, 0))
	ktest.RequireNoError(t, result.Err)
	ktest.RequireEqual(t, result.Status, "WROTE")
	wantPath := filepath.Join(root, "_srcpack", "app_default.md")
	ktest.RequireEqual(t, result.File, wantPath)
	data, err := os.ReadFile(wantPath)
	ktest.RequireNoError(t, err)
	ktest.RequireStringContains(t, string(data), "NamedUpdateMarker")
}

func TestRunOneUpdateNamedTargetCheckDoesNotWrite(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"main.go":           "package main\n\nfunc NamedCheckMarker() {}\n",
		"nexss.srcpack.yml": "targets:\n  app:\n    paths: [. ]\n",
	})
	t.Chdir(root)
	result := runOneUpdate(context.Background(), root, "app", "", true, true, time.Unix(123, 0))
	ktest.RequireNoError(t, result.Err)
	ktest.RequireEqual(t, result.Status, "CHECKED")
	ktest.RequireEqual(t, result.File, filepath.Join(root, "_srcpack", "app_default.md"))
	_, err := os.Stat(filepath.Join(root, "_srcpack", "app_default.md"))
	ktest.RequireCondition(t, os.IsNotExist(err), "check wrote target output: %v", err)
}

func TestRunOneUpdatePropagatesRootPipelineError(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{
		"main.go":           "package main\n\nfunc ErrorPropagationMarker() {}\n",
		"nexss.srcpack.yml": "targets:\n  app:\n    paths: [. ]\n",
	})
	t.Chdir(root)
	// A directory at the output path makes the root output action fail.
	outputPath := filepath.Join(root, "_srcpack", "app_default.md")
	ktest.RequireNoError(t, os.MkdirAll(outputPath, 0o700))
	result := runOneUpdate(context.Background(), root, "app", "", false, true, time.Unix(123, 0))
	ktest.RequireCondition(t, result.Err != nil, "expected root pipeline output error")
	ktest.RequireEqual(t, result.Status, "FAILED")
	ktest.RequireEqual(t, result.File, "")
	ktest.RequireStringContains(t, result.Err.Error(), "app_default.md")
}
