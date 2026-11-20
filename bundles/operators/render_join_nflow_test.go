package operators

import (
	"bytes"
	"context"
	"iter"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/extensions/fs"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"
)

func TestNFlowRenderedJoinPreservesRawBytes(t *testing.T) {
	t.Parallel()

	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	chunks := [][]byte{
		allBytes[:73],
		{0x00, 0xff, 0xfe},
		allBytes[73:193],
		{},
		allBytes[193:],
	}
	want := make([]byte, 0, 259)
	files := make([]fs.FileContent, 0, len(chunks))
	for _, chunk := range chunks {
		want = append(want, chunk...)
		files = append(files, fs.FileContent{Content: chunk})
	}

	fileStream := action.NewStream("probe.files", func(context.Context, struct{}) (iter.Seq2[fs.FileContent, error], error) {
		return func(yield func(fs.FileContent, error) bool) {
			for _, file := range files {
				if !yield(file, nil) {
					return
				}
			}
		}, nil
	})
	probe := compiler.Bundle{
		ID: "srcpack_render_join_test",
		Libraries: []action.Library{{
			Name:    "probe",
			Sources: []action.AnyStreamAction{fileStream},
		}},
	}
	cfg, err := runner.BuildConfig(append(native.Bundles(), probe))
	if err != nil {
		t.Fatalf("BuildConfig() error = %v", err)
	}

	const source = "@schema RenderedOutput {\n" +
		"  Content string `json:\"content\"`\n" +
		"}\n" +
		"@pipeline join_rendered(): RenderedOutput\n" +
		"  probe.files\n" +
		"    -> stream.map ( expression: \"item.Content\" )\n" +
		"    -> collect\n" +
		"    -> bytes.concat\n" +
		"    -> { content: . }\n" +
		"@end\n" +
		"pipeline.join_rendered\n"
	result, err := runner.Execute(t.Context(), cfg, source, "render_join_test.nflow", nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", result.Output)
	}
	content, ok := output["content"].(string)
	if !ok {
		t.Fatalf("content type = %T, want string", output["content"])
	}
	// The runner result is still a Go string here; JSON encoding invalid UTF-8
	// later can replace those bytes, so the CLI path must write it directly.
	if got := []byte(content); !bytes.Equal(got, want) {
		t.Fatalf("joined output bytes = % x, want % x", got, want)
	}
}
