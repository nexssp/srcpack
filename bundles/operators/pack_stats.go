package operators

// PackStats is the shared aggregate contract for terminal summaries and
// rendered-document placeholders. AggregatePackStats remains the Go reference
// for NFlow-fold parity and the fallback used by legacy stream callers.
type PackStats struct {
	FileCount       int
	FuncCount       int
	PackedBytes     int64
	RawBytes        int64
	EstimatedTokens int64
}

// AggregatePackStats computes the totals for files that contribute content to
// a pack. Omitted files and empty signature-only results are intentionally
// excluded, matching both the terminal summary and document header behavior.
func AggregatePackStats(files []ParsedFile) PackStats {
	var stats PackStats
	for i := range files {
		file := &files[i]
		if file.Omitted || (len(file.Content) == 0 && len(file.Snippets) == 0) {
			continue
		}

		stats.FileCount++
		stats.FuncCount += file.FuncCount
		stats.RawBytes += file.Size

		packed := int64(len(file.Content))
		if packed == 0 {
			for _, snippet := range file.Snippets {
				packed += int64(len(snippet))
			}
		}
		stats.PackedBytes += packed
	}

	stats.EstimatedTokens = int64(float64(stats.PackedBytes) / 3.5)
	return stats
}

// SavedPercent returns the fraction of raw input bytes removed by packing.
// Zero-sized input reports 0, preserving the existing summary semantics.
func (stats PackStats) SavedPercent() float64 {
	if stats.RawBytes <= 0 {
		return 0
	}
	return float64(stats.RawBytes-stats.PackedBytes) / float64(stats.RawBytes) * 100
}
