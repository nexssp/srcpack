package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/extensions/schema"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

func TestSrcpackNflowDeclaresIncludedCommandSchemas(t *testing.T) {
	t.Parallel()

	source, err := nflows.FS.ReadFile("srcpack.nflow")
	ktest.RequireNoError(t, err)

	bundles := native.Bundles()
	//nolint:staticcheck // This test preserves compatibility for existing aggregate-bundle hosts.
	bundles = append(bundles, nexssflow.FullBundle(nil))
	cfg, err := runner.BuildConfig(bundles)
	ktest.RequireNoError(t, err)

	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	prepared, err := compiler.PrepareSource(
		ctx,
		cfg.Directives,
		string(source),
		"srcpack.nflow",
		cfg.CompileOpts...,
	)
	ktest.RequireNoError(t, err)

	meta := prepared.Metadata()
	ktest.RequireEqual(t, len(schema.SchemasFromMap(meta)), 0)
	imports := make(map[string]compiler.SourceImport)
	for _, imported := range compiler.SourceImports(meta) {
		imports[imported.Namespace] = imported
	}

	commands := []struct {
		namespace string
		pipeline  string
		route     string
		schema    string
	}{
		{"init", "init", "init:", "InitInput"},
		{"pack", "pack", "pack:", "PackInput"},
		{"tree", "tree", "tree:", "TreeInput"},
		{"update", "update", "update:", "UpdateInput"},
		{"config", "config", "config:", "ConfigInput"},
		{"manage", "target", "target:", "ManageInput"},
		{"manage", "prompt", "prompt:", "ManageInput"},
		{"manage", "profile", "profile:", "ManageInput"},
		{"manage", "editor", "editor:", "ManageInput"},
		{"manage", "view", "view:", "ManageInput"},
		{"system", "dashboard", "dashboard:", "EmptyInput"},
		{"system", "usage", "usage:", "EmptyInput"},
		{"system", "version", "version:", "EmptyInput"},
		{"system", "list", "list:", "EmptyInput"},
		{"system", "info", "info:", "InfoInput"},
		{"system", "clean", "clean:", "CleanInput"},
		{"system", "doctor", "doctor:", "EmptyInput"},
		{"system", "types", "types:", "EmptyInput"},
		{"system", "health", "health:", "EmptyInput"},
		{"system", "demo", "demo:", "EmptyInput"},
		{"system", "cookbook", "cookbook:", "CookbookInput"},
		{"system", "find", "find:", "FindInput"},
	}
	for _, command := range commands {
		imported, ok := imports[command.namespace]
		ktest.RequireCondition(t, ok, "command module %q is not imported", command.namespace)
		moduleMeta := imported.Source.Metadata()
		pipelines, ok := moduleMeta["pipelines"].(map[string]string)
		ktest.RequireCondition(t, ok, "module %q pipelines is %T", command.namespace, moduleMeta["pipelines"])
		ktest.RequireCondition(t, pipelines[command.pipeline] != "", "module %q lacks pipeline %q", command.namespace, command.pipeline)
		modifiers, _ := moduleMeta["pipeline_modifiers"].(map[string][]string)
		pipelineModifiers := modifiers[command.pipeline]
		ktest.RequireCondition(t, hasModifierPrefix(pipelineModifiers, "cli="+command.route),
			"%s.%s lacks its CLI route; got %v", command.namespace, command.pipeline, pipelineModifiers)
		ktest.RequireCondition(t, hasModifierPrefix(pipelineModifiers, "request-schema="+command.schema),
			"%s.%s lacks its local request schema; got %v", command.namespace, command.pipeline, pipelineModifiers)
		findSchema(t, schema.SchemasFromMap(moduleMeta), command.schema)
	}

	packMeta := imports["pack"].Source.Metadata()
	packSchemas := schema.SchemasFromMap(packMeta)
	commonSchema := findSchema(t, packSchemas, "CommonFlags")
	ktest.RequireEqual(t, len(commonSchema.Fields), 3)
	packSchema := findSchema(t, packSchemas, "PackInput")
	names := make([]string, len(packSchema.Fields))
	for i, field := range packSchema.Fields {
		names[i] = field.JSONName
	}
	for _, want := range []string{
		"json", "quiet", "verbose",
		"targets", "files", "ext", "only_sig", "include_config", "output", "plan",
	} {
		ktest.RequireCondition(t, slices.Contains(names, want), "PackInput lacks %q; got %v", want, names)
	}
	ktest.RequireEqual(t, names[:3], []string{"json", "quiet", "verbose"})

	treeSchema := findSchema(t, schema.SchemasFromMap(imports["tree"].Source.Metadata()), "TreeInput")
	ktest.RequireCondition(t, reflect.DeepEqual(schemaFieldsWithoutPositions(packSchema.Fields), schemaFieldsWithoutPositions(treeSchema.Fields)),
		"TreeInput must preserve the PackInput CLI contract through schema embedding")
}

func findSchema(tb testing.TB, schemas []schema.Schema, name string) schema.Schema {
	tb.Helper()
	for _, s := range schemas {
		if s.Name == name {
			return s
		}
	}
	tb.Fatalf("schema %q not declared", name)
	return schema.Schema{}
}

func schemaFieldsWithoutPositions(fields []schema.Field) []schema.Field {
	cloned := slices.Clone(fields)
	for i := range cloned {
		cloned[i].Position = compiler.Position{}
	}
	return cloned
}

func hasModifierPrefix(modifiers []string, prefix string) bool {
	for _, m := range modifiers {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

func TestRunEntrypoint_AdministrativeCommandHandledByNFlow(t *testing.T) {
	t.Parallel()
	handled, err := runEntrypoint(context.Background(), []string{"help"})
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, handled, "administrative commands must execute through NFlow")
}

func TestRunEntrypoint_FlagFirstHandled(t *testing.T) {
	root := materializeWorkspace(t, map[string]string{"main.go": "package main\n"})
	t.Chdir(root)
	handled, err := runEntrypoint(context.Background(), []string{"--target", ".", "--output", "out.md", "--quiet"})
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, handled, "flag-first argv must execute the root pipeline")
	_, err = os.Stat("out.md")
	ktest.RequireNoError(t, err)
}

func TestRunEntrypoint_PackCommandRoutesToPipeline(t *testing.T) {
	root := t.TempDir()
	ktest.RequireNoError(t, os.WriteFile(
		filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc Hello() string { return \"hi\" }\n"),
		0o600,
	))

	t.Chdir(root)

	handled, err := runEntrypoint(context.Background(), []string{
		"pack", "--target", ".", "--only-sig", "--output", "out.md",
	})
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, handled, "pack must be routed by the entrypoint host")

	content, err := os.ReadFile(filepath.Join(root, "out.md"))
	ktest.RequireNoError(t, err)
	ktest.RequireStringContains(t, string(content), "func Hello() string")
	ktest.RequireStringNotContains(t, string(content), `return "hi"`)
}
