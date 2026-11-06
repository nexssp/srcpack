package operators

import (
	"iter"

	"github.com/nexssp/kernel/action"
)

// packBatch materializes one stream into a single array using append. It does
// not cap records or compute statistics: source guardrails remain the only
// scan limit, and the downstream Flow fold owns aggregation.
func packBatch() action.StreamOp[map[string]any, []map[string]any] {
	return func(up iter.Seq2[map[string]any, error]) iter.Seq2[[]map[string]any, error] {
		return func(yield func([]map[string]any, error) bool) {
			items := make([]map[string]any, 0)
			for item, err := range up {
				if err != nil {
					yield(nil, err)
					return
				}
				items = append(items, item)
			}
			yield(items, nil)
		}
	}
}

// PackBatchOperator exposes the append-only materializer to NFlow.
func PackBatchOperator() action.NamedOperator {
	return action.NewSimpleOperator("pack.batch", packBatch())
}
