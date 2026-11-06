package operators

import (
	"testing"
)

func TestFoldPackStatsMatchesAggregatePackStats(t *testing.T) {
	cases := []struct {
		name  string
		files []ParsedFile
	}{
		{
			name: "omitted file is excluded",
			files: []ParsedFile{{
				Size:      16,
				Content:   []byte("omitted"),
				FuncCount: 3,
				Omitted:   true,
			}},
		},
		{
			name: "empty signature result is excluded",
			files: []ParsedFile{{
				Size:      50,
				FuncCount: 4,
			}},
		},
		{
			name: "content takes precedence over snippets",
			files: []ParsedFile{{
				Size:      8,
				Content:   []byte("abc"),
				Snippets:  []string{"ignored"},
				FuncCount: 2,
			}},
		},
		{
			name: "snippet byte lengths are counted",
			files: []ParsedFile{{
				Size:      10,
				Snippets:  []string{"é", "xy"},
				FuncCount: 1,
			}},
		},
		{
			name: "zero raw size keeps saved percentage at zero",
			files: []ParsedFile{{
				Content: []byte("packed"),
			}},
		},
		{
			name: "empty input emits the initial aggregate",
		},
		{
			name: "combined fixture",
			files: []ParsedFile{
				{Size: 16, Content: []byte("omitted"), FuncCount: 3, Omitted: true},
				{Size: 50, FuncCount: 4},
				{Size: 8, Content: []byte("abc"), Snippets: []string{"ignored"}, FuncCount: 2},
				{Size: 10, Snippets: []string{"é", "xy"}, FuncCount: 1},
				{Content: []byte("packed")},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := foldPackStats(tc.files)
			if err != nil {
				t.Fatalf("foldPackStats() error = %v", err)
			}
			want := AggregatePackStats(tc.files)
			if got != want {
				t.Fatalf("foldPackStats() = %#v, AggregatePackStats() = %#v", got, want)
			}
			if got.SavedPercent() != want.SavedPercent() {
				t.Fatalf("fold SavedPercent() = %v, AggregatePackStats SavedPercent() = %v", got.SavedPercent(), want.SavedPercent())
			}
		})
	}
}
