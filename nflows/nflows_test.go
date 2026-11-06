package nflows_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"

	_ "github.com/nexssp/srcpack/bundles/config_cli"
	"github.com/nexssp/srcpack/bundles/document"
	_ "github.com/nexssp/srcpack/bundles/init"
	_ "github.com/nexssp/srcpack/bundles/manage"
	_ "github.com/nexssp/srcpack/bundles/pack"
	_ "github.com/nexssp/srcpack/bundles/system"
	_ "github.com/nexssp/srcpack/bundles/update"
	_ "github.com/nexssp/srcpack/nexssflow"
	nflows "github.com/nexssp/srcpack/nflows"
)

func TestFilesystemIncludesMatchCopyableRepositoryLayout(t *testing.T) {
	cwd, err := os.Getwd()
	ktest.RequireNoError(t, err)
	source, err := nflows.FS.ReadFile("srcpack.nflow")
	ktest.RequireNoError(t, err)
	cfg, err := runner.BuildConfig(native.Bundles())
	ktest.RequireNoError(t, err)
	// No embedded loader: all relative includes must resolve to real files.
	prepared, err := compiler.PrepareSource(context.Background(), cfg.Directives, string(source), filepath.Join(cwd, "srcpack.nflow"), cfg.CompileOpts...)
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, prepared.Metadata()["pipelines"] != nil, "filesystem source did not expose declared pipelines")
}

func TestEmbeddedLoaderNamespaces(t *testing.T) {
	root := t.TempDir()
	loader := nflows.Loader()
	from := filepath.Join(root, "srcpack.nflow")
	data, identity, err := loader.Load(from, filepath.Join(root, "bundles", "document", "document.nflow"))
	ktest.RequireNoError(t, err)
	want, err := document.FS.ReadFile("document.nflow")
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, string(data), string(want))
	ktest.RequireEqual(t, identity, "bundles/document/document.nflow")
	data, identity, err = loader.Load("bundles/init/init.nflow", "schema.nflow")
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, identity, "bundles/init/schema.nflow")
	ktest.RequireCondition(t, strings.Contains(string(data), "InitInput"), "command sibling schema was not loaded")
	data, identity, err = loader.Load("bundles/config_views/config_views.nflow", "../crud/crud.nflow")
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, identity, "bundles/crud/crud.nflow")
	ktest.RequireCondition(t, len(data) > 0, "bundle-relative CRUD module was empty")
	_, _, err = loader.Load("bundles/init/init.nflow", "models.nflow")
	ktest.RequireCondition(t, err != nil, "command include must not alias a flat NFlow basename")
	data, identity, err = loader.Load(from, filepath.Join(root, "pack.nflow"))
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, identity, "pack.nflow")
	ktest.RequireCondition(t, strings.Contains(string(data), "render.pipeline.render_configured_document_pipeline"), "production pack must use the configured shared NFlow formatter")
	for _, target := range []string{filepath.Join(root, "nested", "pack.nflow"), filepath.Join(root, "bundles", "other", "document.nflow")} {
		_, _, err := loader.Load(from, target)
		ktest.RequireCondition(t, err != nil, "unknown namespace must not alias an embedded basename")
	}
	_, _, err = loader.Load(from, filepath.Join(root, "missing.nflow"))
	ktest.RequireCondition(t, err != nil, "missing source must fail")
}

func TestProductionUsesNFlowFormattingAndSingleRoot(t *testing.T) {
	for _, name := range []string{"pack.nflow", "tree.nflow"} {
		data, err := nflows.FS.ReadFile(name)
		ktest.RequireNoError(t, err)
		ktest.RequireCondition(t, !strings.Contains(string(data), "srcpack.render"), "production %s must not invoke the legacy renderer", name)
		ktest.RequireCondition(t, strings.Contains(string(data), "-> collect\n    -> render.pipeline.render_configured_document_pipeline"), "stream must end before composing the formatter")
	}
	workflow, _, err := nflows.Loader().Load("bundles/pack/pack.nflow", "pack_workflow.nflow")
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, strings.Contains(string(workflow), "render.pipeline.render_document_input_pipeline"), "production pack workflow must use the shared NFlow input-and-render pipeline")
	data, err := nflows.FS.ReadFile("srcpack.nflow")
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, strings.Contains(string(data), "tcli.listen"), "tcli must own CLI routing")
	ktest.RequireCondition(t, !strings.Contains(string(data), "match(.command)"), "root must not retain the old command switch")
	for _, command := range []string{"init", "pack", "tree", "update", "config", "manage", "system"} {
		folder := command
		if command == "config" {
			folder = "config_cli"
		}
		include := "../bundles/" + folder + "/" + command + ".nflow"
		ktest.RequireCondition(t, strings.Contains(string(data), include), "root lacks command module %s", command)
		for _, file := range []string{command + ".nflow", "schema.nflow"} {
			target := filepath.Join("bundles", folder, file)
			body, identity, err := nflows.Loader().Load("srcpack.nflow", target)
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, identity, filepath.ToSlash(target))
			ktest.RequireCondition(t, len(body) > 0, "empty embedded command file %s", target)
		}
	}
	ktest.RequireCondition(t, strings.Contains(string(data), `@include "cli_invocation.nflow"`), "root must include its NFlow argv policy")
	invocation, identity, err := nflows.Loader().Load("srcpack.nflow", "cli_invocation.nflow")
	ktest.RequireNoError(t, err)
	ktest.RequireEqual(t, identity, "cli_invocation.nflow")
	ktest.RequireCondition(t, strings.Contains(string(invocation), "@pipeline normalize"), "included argv policy pipeline is missing")
}

func TestRenderConfigArgumentMacroCoversAllFields(t *testing.T) {
	source, err := nflows.FS.ReadFile("render.nflow")
	ktest.RequireNoError(t, err)
	body := string(source)
	ktest.RequireCondition(t, strings.Contains(body, "$field: @config.render_$field"), "render config macro must map each field to its same-suffix config reference")
	for _, field := range []string{
		"editor", "prompt", "start", "end", "file_start", "file_end",
		"profile_name", "targets", "dirs", "output_file", "prompt_name",
		"git_commit", "git_tag", "git_branch", "current_date", "directory",
		"diff_content", "no_header", "no_prompt", "no_start", "no_end", "tree",
		"full_tree", "tree_files", "tree_depth",
	} {
		call := "@render_config_field(" + field + ")"
		ktest.RequireCondition(t, strings.Contains(body, call), "render config mapping lacks %s", field)
	}
}
