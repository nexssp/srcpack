package operators

import (
	"iter"

	"github.com/nexssp/kernel/action"
)

// packStatsInput adapts ParsedFile into the small set of facts the NFlow fold
// needs. The original record is retained so the envelope can restore it for
// SRCPACK-owned summary and rendering stages. Snippets are converted to byte
// slices because Expr len(string) counts Unicode characters, while SRCPACK
// reports packed sizes in bytes.
func packStatsInput() action.StreamOp[ParsedFile, map[string]any] {
	return func(up iter.Seq2[ParsedFile, error]) iter.Seq2[map[string]any, error] {
		return func(yield func(map[string]any, error) bool) {
			for file, err := range up {
				if err != nil {
					yield(nil, err)
					return
				}

				snippets := make([][]byte, len(file.Snippets))
				for i, snippet := range file.Snippets {
					snippets[i] = []byte(snippet)
				}
				record := map[string]any{
					"file":      file,
					"omitted":   file.Omitted,
					"content":   file.Content,
					"snippets":  snippets,
					"funcCount": file.FuncCount,
					"size":      file.Size,
				}
				if !yield(record, nil) {
					return
				}
			}
		}
	}
}

// PackStatsInputOperator prepares ParsedFile facts for the NFlow-owned stats
// fold without moving source selection, parsing, or rendering out of SRCPACK.
func PackStatsInputOperator() action.NamedOperator {
	return action.NewSimpleOperator("pack.stats_input", packStatsInput())
}
