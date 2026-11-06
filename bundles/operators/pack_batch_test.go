package operators

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceLimitKeepsOmittedMarkersForBatchPipeline(t *testing.T) {
	root := t.TempDir()
	writeBatchFixture(t, root, "main.go", "package main\nfunc Main() {}\n")
	writeBatchFixture(t, root, "one_test.go", "package main\nfunc TestOne() {}\n")
	writeBatchFixture(t, root, "two_test.go", "package main\nfunc TestTwo() {}\n")
	t.Chdir(root)

	source, err := SourceOperator().DoStreamAny(context.Background(), PackConfig{
		Targets:      []string{"."},
		NoTests:      true,
		MaxFilesScan: 1,
	})
	if err != nil {
		t.Fatalf("srcpack.source: %v", err)
	}

	var files []ParsedFile
	var streamErr error
	source(func(value any, err error) bool {
		if err != nil {
			streamErr = err
			return false
		}
		file, ok := value.(ParsedFile)
		if !ok {
			streamErr = fmt.Errorf("source emitted %T, want ParsedFile", value)
			return false
		}
		files = append(files, file)
		return true
	})
	if streamErr != nil {
		t.Fatalf("consume srcpack.source: %v", streamErr)
	}
	if len(files) != 3 {
		t.Fatalf("source emitted %d records, want one included file and two omitted markers", len(files))
	}
	included, omitted := 0, 0
	for _, file := range files {
		if file.Omitted {
			omitted++
		} else {
			included++
		}
	}
	if included != 1 || omitted != 2 {
		t.Fatalf("source records included=%d omitted=%d, want 1 and 2", included, omitted)
	}
}

func writeBatchFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
