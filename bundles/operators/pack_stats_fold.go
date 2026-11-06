package operators

import (
	"errors"
	"fmt"

	"github.com/nexssp/flow/extensions/stream_ops"
)

const packStatsContentField = "content"

// packStatsFoldExpression mirrors AggregatePackStats' eligibility and byte
// counting rules while leaving filesystem and parsing concerns in SRCPACK.
const packStatsFoldExpression = `{
	fileCount: acc.fileCount + (!item.omitted && (len(item.content) > 0 || len(item.snippets) > 0) ? 1 : 0),
	funcCount: acc.funcCount + (!item.omitted && (len(item.content) > 0 || len(item.snippets) > 0) ? item.funcCount : 0),
	packedBytes: acc.packedBytes + (!item.omitted && (len(item.content) > 0 || len(item.snippets) > 0) ? (len(item.content) > 0 ? len(item.content) : sum(map(item.snippets, len(#)))) : 0),
	rawBytes: acc.rawBytes + (!item.omitted && (len(item.content) > 0 || len(item.snippets) > 0) ? item.size : 0)
}`

// foldPackStats preserves the existing Go-backed aggregate for legacy stream
// callers. The active pack.nflow path uses pack.batch plus stream.fold with
// its expression defined in nflows/pack.nflow; source selection, parsing, and
// rendering stay SRCPACK-owned in both paths.
func foldPackStats(files []ParsedFile) (PackStats, error) {
	input := func(yield func(any, error) bool) {
		for i := range files {
			file := &files[i]
			// Expr's string len counts characters; SRCPACK counts bytes. Use
			// byte slices so snippet length retains the existing UTF-8 semantics.
			snippets := make([][]byte, len(file.Snippets))
			for i, snippet := range file.Snippets {
				snippets[i] = []byte(snippet)
			}
			record := map[string]any{
				"omitted":             file.Omitted,
				packStatsContentField: file.Content,
				"snippets":            snippets,
				"funcCount":           file.FuncCount,
				"size":                file.Size,
			}
			if !yield(record, nil) {
				return
			}
		}
	}

	fold := stream_ops.Fold(stream_ops.FoldConfig{
		Initial: map[string]any{
			"fileCount":   0,
			"funcCount":   0,
			"packedBytes": 0,
			"rawBytes":    int64(0),
		},
		Expression: packStatsFoldExpression,
	})(input)

	var result any
	var foldErr error
	emitted := false
	fold(func(value any, err error) bool {
		if err != nil {
			foldErr = err
			return false
		}
		if emitted {
			foldErr = errors.New("stream.fold emitted more than one aggregate")
			return false
		}
		result = value
		emitted = true
		return true
	})
	if foldErr != nil {
		return PackStats{}, foldErr
	}
	if !emitted {
		return PackStats{}, errors.New("stream.fold emitted no aggregate")
	}

	fields, ok := result.(map[string]any)
	if !ok {
		return PackStats{}, fmt.Errorf("stream.fold returned %T, want map[string]any", result)
	}
	fileCount, err := foldInteger(fields["fileCount"])
	if err != nil {
		return PackStats{}, fmt.Errorf("stream.fold fileCount: %w", err)
	}
	funcCount, err := foldInteger(fields["funcCount"])
	if err != nil {
		return PackStats{}, fmt.Errorf("stream.fold funcCount: %w", err)
	}
	packedBytes, err := foldInteger(fields["packedBytes"])
	if err != nil {
		return PackStats{}, fmt.Errorf("stream.fold packedBytes: %w", err)
	}
	rawBytes, err := foldInteger(fields["rawBytes"])
	if err != nil {
		return PackStats{}, fmt.Errorf("stream.fold rawBytes: %w", err)
	}

	stats := PackStats{
		FileCount:   int(fileCount),
		FuncCount:   int(funcCount),
		PackedBytes: packedBytes,
		RawBytes:    rawBytes,
	}
	stats.EstimatedTokens = int64(float64(stats.PackedBytes) / 3.5)
	return stats, nil
}

func foldInteger(value any) (int64, error) {
	switch number := value.(type) {
	case int:
		return int64(number), nil
	case int64:
		return number, nil
	case float64:
		converted := int64(number)
		if float64(converted) != number {
			return 0, fmt.Errorf("got non-integer numeric value %v", number)
		}
		return converted, nil
	default:
		return 0, fmt.Errorf("got %T (%v), want integer", value, value)
	}
}
