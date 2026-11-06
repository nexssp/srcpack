package systemcmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorPathStatReportsFileDirectoryAndMissingPath(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "present.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	fileFacts, err := doctorPathStat(context.Background(), map[string]any{"path": file})
	if err != nil {
		t.Fatal(err)
	}
	if fileFacts["exists"] != true || fileFacts["is_dir"] != false || fileFacts["size"] != int64(5) {
		t.Fatalf("file facts = %#v", fileFacts)
	}

	dirFacts, err := doctorPathStat(context.Background(), map[string]any{"path": root})
	if err != nil {
		t.Fatal(err)
	}
	if dirFacts["exists"] != true || dirFacts["is_dir"] != true {
		t.Fatalf("directory facts = %#v", dirFacts)
	}

	missingFacts, err := doctorPathStat(context.Background(), map[string]any{"path": filepath.Join(root, "missing")})
	if err != nil {
		t.Fatal(err)
	}
	if missingFacts["exists"] != false || missingFacts["is_dir"] != false || missingFacts["size"] != int64(0) {
		t.Fatalf("missing-path facts = %#v", missingFacts)
	}
}

func TestDoctorPathStatRejectsEmptyPathAndHonorsCancellation(t *testing.T) {
	if _, err := doctorPathStat(context.Background(), map[string]any{"path": ""}); err == nil {
		t.Fatal("empty path unexpectedly succeeded")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := doctorPathStat(ctx, map[string]any{"path": t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled stat error = %v", err)
	}
}
