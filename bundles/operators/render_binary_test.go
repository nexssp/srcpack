package operators

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/document"
)

// This gate deliberately runs real NFlow projections before and after the
// byte-to-string boundary, then the independent production document formatter.
// JSON, fmt.Sprint and Unicode decoding must never touch source content.
func TestNFlowBinarySafePassthrough(t *testing.T) {
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"nul_invalid_utf8", []byte("begin\x00middle\xff\xfeend")},
		{"every_byte", allBytes},
		{"template_literal", []byte("${PROFILE_NAME}\x00${UNKNOWN}\xff\xc0\xaf\xed\xa0\x80")},
		{"already_newline", []byte("\x00\xff\r\n")},
		{"empty", []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := bytes.Clone(tc.raw)
			calls := 0
			transport := action.New("binary.transport", func(_ context.Context, input map[string]any) (map[string]any, error) {
				files, ok := input["files"].([]any)
				if !ok || len(files) != 1 {
					return nil, fmt.Errorf("projection changed file array: %T", input["files"])
				}
				file, ok := files[0].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("projected file has type %T, want map[string]any", files[0])
				}
				raw, ok := file["content"].([]byte)
				if !ok || !bytes.Equal(raw, original) {
					return nil, fmt.Errorf("projection corrupted raw bytes: %T %x; want %x", file["content"], file["content"], original)
				}
				calls++
				// The sole representation change permitted in production is Go's lossless
				// byte-to-string conversion; expression string([]byte) is not assumed safe.
				file["content"] = string(raw)
				return input, nil
			}).Build()
			bundles := append(native.Bundles(), document.Bundle(nil), compiler.Bundle{ID: "binary_gate", Libraries: []action.Library{{Name: "binary", Actions: []action.AnyAction{transport}}}})
			cfg, err := runner.BuildConfig(bundles)
			if err != nil {
				t.Fatal(err)
			}
			source := `@include "document.nflow"
{ files: map(.records, let file = #; { content: file.raw, omitted: false, snippets: [], values: {} }) }
-> binary.transport
-> { config: { editor: "raw" }, values: { PROFILE_NAME: "DO_NOT_EXPAND" }, tree: "", files: map(.files, let file = #; { content: file.content, omitted: file.omitted, snippets: file.snippets, values: file.values }) }
-> document.pipeline.document_render`
			ctx := compiler.WithSourceLoader(context.Background(), document.Loader())
			ex, err := runner.Execute(ctx, cfg, source, "binary-gate.nflow", map[string]any{"records": []any{map[string]any{"raw": tc.raw}}})
			if err != nil {
				t.Fatal(err)
			}
			output, ok := ex.Output.(map[string]any)
			if !ok {
				t.Fatalf("render output has type %T, want map[string]any", ex.Output)
			}
			content, ok := output["content"].(string)
			if !ok {
				t.Fatalf("render content has type %T, want string", output["content"])
			}
			got := []byte(content)
			want := bytes.Clone(original)
			// Existing formatter adds a final LF to nonempty file bodies; it is not
			// source corruption. Compare the complete expected output, not substrings.
			if len(want) > 0 && want[len(want)-1] != '\n' {
				want = append(want, '\n')
			}
			if calls != 1 || !bytes.Equal(got, want) || !bytes.Equal(tc.raw, original) {
				t.Fatalf("binary gate failed: calls=%d want=%x got=%x input=%x", calls, want, got, tc.raw)
			}
		})
	}
}
