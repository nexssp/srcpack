package operators

import (
	"iter"
	"os"

	"github.com/nexssp/flow/extensions/fs"
	"github.com/nexssp/kernel/action"
)

func Stdout(_ struct{}) action.StreamOp[fs.FileContent, fs.FileContent] {
	return func(up iter.Seq2[fs.FileContent, error]) iter.Seq2[fs.FileContent, error] {
		return func(yield func(fs.FileContent, error) bool) {
			for item, err := range up {
				if err != nil {
					yield(fs.FileContent{}, err)
					return
				}
				if _, err := os.Stdout.Write(item.Content); err != nil {
					yield(fs.FileContent{}, err)
					return
				}
				if !yield(item, nil) {
					return
				}
			}
		}
	}
}

func StdoutOperator() action.NamedOperator {
	return action.NewOperator("srcpack.stdout", Stdout)
}
