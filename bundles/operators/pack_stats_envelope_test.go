package operators

import (
	"iter"
	"reflect"
	"testing"
)

func TestStatsCollectPreservesParsedFileRecords(t *testing.T) {
	files := []ParsedFile{
		{Path: "/workspace/first.go", RelPath: "first.go", Size: 10, Lang: "go", Content: []byte("café"), Snippets: []string{"ignored because content is present"}, FuncCount: 2},
		{Path: "/workspace/second.go", RelPath: "second.go", Size: 11, Lang: "go", Snippets: []string{"é", "xy"}, FuncCount: 3},
		{Path: "/workspace/omitted.go", RelPath: "omitted.go", Size: 90, Lang: "go", Content: []byte("not counted"), FuncCount: 99, Omitted: true},
		{Path: "/workspace/empty.go", RelPath: "empty.go", Size: 20, Lang: "go", FuncCount: 7},
	}

	collected, err := collectAny(StatsCollect(StatsCollectConfig{Quiet: true})(anyParsedFiles(files)))
	if err != nil {
		t.Fatalf("stats.collect: %v", err)
	}
	gotFiles, err := parsedFilesFromRecords(collected)
	if err != nil {
		t.Fatalf("decode stats.collect records: %v", err)
	}
	if !reflect.DeepEqual(gotFiles, files) {
		t.Fatalf("stats.collect changed or reordered records:\n got: %#v\nwant: %#v", gotFiles, files)
	}
}

func anyParsedFiles(files []ParsedFile) iter.Seq2[any, error] {
	return func(yield func(any, error) bool) {
		for _, file := range files {
			if !yield(file, nil) {
				return
			}
		}
	}
}

func collectAny(stream iter.Seq2[any, error]) ([]any, error) {
	var values []any
	var streamErr error
	stream(func(value any, err error) bool {
		if err != nil {
			streamErr = err
			return false
		}
		values = append(values, value)
		return true
	})
	return values, streamErr
}
