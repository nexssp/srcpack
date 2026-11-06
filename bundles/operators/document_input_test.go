package operators

import (
	"context"
	"os"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/document"
	"github.com/nexssp/srcpack/bundles/terminal"
)

func renderShapeSource(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../nflows/render.nflow")
	ktest.RequireNoError(t, err)
	return string(source)
}

func TestDocumentInputActionRecordAndEnvelopeContracts(t *testing.T) {
	files := []ParsedFile{{RelPath: "a.go", Lang: "go", Size: 7, Content: []byte("\x00\xff"), FuncCount: 2}}
	stats, err := foldPackStats(files)
	ktest.RequireNoError(t, err)
	act := DocumentInputAction()
	request := func(records []any) map[string]any {
		return map[string]any{"records": records, "config": map[string]any{"editor": "raw", "no_header": "true", "tree_files": "false"}}
	}
	direct, err := act.DoAny(context.Background(), request([]any{files[0]}))
	ktest.RequireNoError(t, err)
	envelope, err := act.DoAny(context.Background(), request([]any{packStatsEnvelope{Files: files, Stats: stats}}))
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, direct, envelope)
	out, ok := direct.(map[string]any)
	if !ok {
		t.Fatalf("document input has type %T, want map[string]any", direct)
	}
	outputFiles, ok := out["files"].([]map[string]any)
	if !ok || len(outputFiles) != 1 {
		t.Fatalf("document files have value %#v, want one map", out["files"])
	}
	content, ok := outputFiles[0]["content"].(string)
	if !ok {
		t.Fatalf("document content has type %T, want string", outputFiles[0]["content"])
	}
	ktest.RequireEqual(t, content, "\x00\xff")
	configValues, ok := out["config"].(map[string]any)
	if !ok {
		t.Fatalf("document config has type %T, want map[string]any", out["config"])
	}
	noHeader, ok := configValues["no_header"].(bool)
	if !ok {
		t.Fatalf("no_header has type %T, want bool", configValues["no_header"])
	}
	ktest.RequireEqual(t, noHeader, true)
	for _, bad := range []any{
		map[string]any{},
		request([]any{"not a parsed file"}),
		request([]any{packStatsEnvelope{}, packStatsEnvelope{}}),
		request([]any{map[string]any{"files": []any{files[0]}}}),
		map[string]any{"records": []any{}, "config": map[string]any{"tree": "not a bool"}},
	} {
		_, err := act.DoAny(context.Background(), bad)
		ktest.RequireCondition(t, err != nil, "malformed domain input must fail")
	}
}

func TestDocumentInputIsAvailableWithoutLegacyRenderer(t *testing.T) {
	// The independent document bundle cannot accidentally invoke srcpack.render.
	cfg, err := runner.BuildConfig(append(native.Bundles(), document.Bundle(nil), terminal.Bundle(nil), compiler.Bundle{
		ID: "domain_adapter_only", Libraries: []action.Library{{Name: "srcpack", Actions: []action.AnyAction{DocumentInputAction()}}},
	}))
	ktest.RequireNoError(t, err)
	ctx := compiler.WithSourceLoader(context.Background(), document.Loader())
	input := map[string]any{"records": []any{ParsedFile{Content: []byte("source")}}, "config": map[string]any{"editor": "raw"}}
	ex, err := runner.Execute(ctx, cfg, renderShapeSource(t)+"\nsrcpack.document_input -> @shape_document() -> document.pipeline.document_render", "adapter.nflow", input)
	ktest.RequireNoError(t, err)
	output, ok := ex.Output.(map[string]any)
	if !ok {
		t.Fatalf("render output has type %T, want map[string]any", ex.Output)
	}
	content, ok := output["content"].(string)
	if !ok {
		t.Fatalf("render content has type %T, want string", output["content"])
	}
	ktest.RequireEqual(t, content, "source\n")
}
