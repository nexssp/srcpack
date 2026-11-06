package parser

import (
	"bytes"
	"strings"
	"testing"
)

func BenchmarkCleanSource(b *testing.B) {
	sample := `
// Service declaration
package service

/* Multi-line documentation
   block for the service */
func ProcessData() {
	    // Indented comment
	    var query = "http://example.com/api"
	    DoWork()


}
`
	repeated := strings.Repeat(sample, 500)
	src := []byte(repeated)
	opts := Options{
		StripComments:  true,
		TrimIndent:     true,
		TrimBlankLines: true,
	}

	var buf bytes.Buffer
	b.ResetTimer()
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()

	for range b.N {
		CleanSource(src, opts, &buf)
	}
}
