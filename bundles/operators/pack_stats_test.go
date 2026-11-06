package operators

import (
	"math"
	"testing"
)

func TestAggregatePackStats(t *testing.T) {
	files := []ParsedFile{
		{
			Size:      10,
			Content:   []byte("abc"),
			Snippets:  []string{"ignored because content is present"},
			FuncCount: 2,
		},
		{
			Size:      20,
			Snippets:  []string{"x", "yy"},
			FuncCount: 1,
		},
		{
			Size:      100,
			Content:   []byte("omitted"),
			FuncCount: 99,
			Omitted:   true,
		},
		{
			Size:      50,
			FuncCount: 50,
		},
	}

	got := AggregatePackStats(files)
	want := PackStats{
		FileCount:       2,
		FuncCount:       3,
		PackedBytes:     6,
		RawBytes:        30,
		EstimatedTokens: 1,
	}
	if got != want {
		t.Fatalf("AggregatePackStats() = %#v, want %#v", got, want)
	}
	if got.SavedPercent() != 80 {
		t.Fatalf("SavedPercent() = %v, want 80", got.SavedPercent())
	}
}

func TestAggregatePackStatsZeroRawBytes(t *testing.T) {
	got := AggregatePackStats([]ParsedFile{{Content: []byte("packed")}})
	if got.FileCount != 1 || got.PackedBytes != 6 || got.RawBytes != 0 {
		t.Fatalf("AggregatePackStats() = %#v, want one six-byte packed file with zero raw bytes", got)
	}
	if math.Abs(got.SavedPercent()) > 1e-9 {
		t.Fatalf("SavedPercent() = %v, want 0", got.SavedPercent())
	}
}
