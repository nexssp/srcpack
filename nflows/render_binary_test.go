package nflows

import (
	"bytes"
	"context"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/document"
	"github.com/nexssp/srcpack/bundles/operators"
	"github.com/nexssp/srcpack/bundles/terminal"
)

func TestRenderDocumentMacroPreservesBinaryContent(t *testing.T) {
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}

	testCases := []struct {
		name string
		raw  []byte
	}{
		{name: "nul_and_invalid_utf8", raw: []byte("begin\x00middle\xff\xfeend")},
		{name: "all_256_byte_values", raw: allBytes},
		{name: "ending_with_newline", raw: []byte("\x00\xff\r\n")},
		{name: "without_final_newline", raw: []byte("bytes\x00\xff")},
		{name: "empty", raw: []byte{}},
	}

	bundles := append(native.Bundles(), document.Bundle(nil), terminal.Bundle(nil), compiler.Bundle{
		ID: "srcpack_render_binary_test",
		Libraries: []action.Library{{
			Name: "srcpack",
			Actions: []action.AnyAction{
				operators.DocumentInputAction(),
				action.New("binary_test.records", func(_ context.Context, input map[string]any) (any, error) {
					return input["records"], nil
				}).Build(),
			},
		}},
	})
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		t.Fatal(err)
	}
	ctx := compiler.WithSourceLoader(context.Background(), Loader())
	source := `@include "render.nflow"
@pipeline render_binary_test
  { records: .records, config: { editor: "raw" } }
  -> render.pipeline.render_document_input_pipeline
@end
pipeline.render_binary_test`

	run := func(t *testing.T, records []any) []byte {
		t.Helper()
		ex, err := runner.Execute(ctx, cfg, source, "render-binary-test.nflow", map[string]any{"records": records})
		if err != nil {
			t.Fatal(err)
		}
		out, ok := ex.Output.(map[string]any)
		if !ok {
			t.Fatalf("render output has type %T, want map[string]any", ex.Output)
		}
		content, ok := out["content"].(string)
		if !ok {
			t.Fatalf("render content has type %T, want string", out["content"])
		}
		return []byte(content)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			original := bytes.Clone(tc.raw)
			file := operators.ParsedFile{
				Path: "/workspace/binary.bin", RelPath: "binary.bin", Lang: "text", Size: int64(len(tc.raw)),
				Content: tc.raw,
			}
			got := run(t, []any{file})
			want := bytes.Clone(original)
			// document_format emits a final LF for nonempty file bodies only.
			if len(want) > 0 && want[len(want)-1] != '\n' {
				want = append(want, '\n')
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("rendered bytes differ: want %x, got %x", want, got)
			}
			if !bytes.Equal(tc.raw, original) {
				t.Fatalf("render mutated input bytes: want %x, got %x", original, tc.raw)
			}
		})
	}

	t.Run("exported_pack_envelope_map", func(t *testing.T) {
		raw := []byte{0, 0xff, 0xc0, '\n'}
		original := bytes.Clone(raw)
		file := operators.ParsedFile{
			RelPath: "packed.bin", Lang: "text", Size: int64(len(raw)),
			Content: raw,
		}
		envelope := map[string]any{
			"files": []any{file}, "fileCount": 1, "funcCount": 0,
			"packedBytes": int64(len(raw)), "rawBytes": int64(len(raw)),
		}
		got := run(t, []any{envelope})
		if !bytes.Equal(got, original) {
			t.Fatalf("rendered pack-envelope bytes differ: want %x, got %x", original, got)
		}
		if !bytes.Equal(raw, original) {
			t.Fatalf("render mutated pack-envelope input bytes: want %x, got %x", original, raw)
		}
	})
}
