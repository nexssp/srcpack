package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/transport/tcli"

	"github.com/nexssp/srcpack/nflows"
)

// Compile the real root, not a hand-maintained subset of commands. Every
// declaration and every capability-valued pipeline reference must be mounted.
func TestProductionPipelineRegistryComplete(t *testing.T) {
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	body, err := nflows.FS.ReadFile("srcpack.nflow")
	if err != nil {
		t.Fatal(err)
	}
	transport := tcli.New(tcli.WithOutput(io.Discard, io.Discard))
	cfg, err := cliRunnerConfig(ctx, string(body), transport)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := runner.Compile(ctx, cfg, string(body), "srcpack.nflow")
	if err != nil {
		t.Fatalf("materialize complete production graph: %v", err)
	}
	references := regexp.MustCompile(`(?:[A-Za-z_][A-Za-z0-9_]*\.)*pipeline\.[A-Za-z_][A-Za-z0-9_.]*`)
	seen := make(map[string]bool)
	pipelineCount := collectModulePipelineRefs(compiled.Meta, "", seen, references)
	if pipelineCount == 0 {
		t.Fatal("production pipeline metadata is missing")
	}
	for _, required := range []string{
		"pipeline.command_entry", "pack.pipeline.pack", "update.pipeline.update",
		"update.update_item.pipeline.update_item", "pack.pack_workflow.pipeline.pack_resolve",
		"pack.pack_workflow.pipeline.pack_execute", "pack.pack_workflow.output.pipeline.output_policy",
	} {
		if !seen[required] {
			t.Errorf("required production dependency disappeared: %s", required)
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if _, mounted := compiled.Resolver.Action(name); !mounted {
			t.Errorf("unresolved production capability %s", name)
		}
	}
	t.Logf("verified %d declared pipelines across source modules and %d unique declarations/references: %s", pipelineCount, len(names), strings.Join(names, ", "))
}

func collectModulePipelineRefs(meta map[string]any, namespace string, seen map[string]bool, references *regexp.Regexp) int {
	pipelines, _ := meta["pipelines"].(map[string]string)
	for name, source := range pipelines {
		canonical := name
		if !strings.HasPrefix(canonical, "pipeline.") {
			canonical = "pipeline." + canonical
		}
		if namespace != "" {
			canonical = namespace + "." + canonical
		}
		seen[canonical] = true
		for _, reference := range references.FindAllString(source, -1) {
			if namespace != "" {
				reference = namespace + "." + reference
			}
			seen[reference] = true
		}
	}
	count := len(pipelines)
	for _, imported := range compiler.SourceImports(meta) {
		childNamespace := imported.Namespace
		if namespace != "" {
			childNamespace = namespace + "." + childNamespace
		}
		count += collectModulePipelineRefs(imported.Source.Metadata(), childNamespace, seen, references)
	}
	return count
}

// Exercise the user's exact install -> installed executable path. GOWORK=off
// proves the module replacement protects builds outside the supplied workspace.
func TestFreshGoInstallHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("fresh-install subprocess regression")
	}
	_, filename, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, workspace := range []string{"auto", "off"} {
		t.Run(workspace, func(t *testing.T) {
			bin := t.TempDir()
			env := make([]string, 0, len(os.Environ())+2)
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "GOBIN=") && !strings.HasPrefix(entry, "GOWORK=") {
					env = append(env, entry)
				}
			}
			env = append(env, "GOBIN="+bin, "GOWORK="+workspace)
			install := exec.CommandContext(t.Context(), goBin, "install", "./cmd/srcpack/")
			install.Dir, install.Env = moduleRoot, env
			if output, err := install.CombinedOutput(); err != nil {
				t.Fatalf("fresh go install (GOWORK=%s): %v\n%s", workspace, err, output)
			}
			name := "srcpack"
			if runtime.GOOS == "windows" {
				name = "srcpack.exe"
			}
			if name != "srcpack" && name != "srcpack.exe" {
				t.Fatalf("unexpected installed executable name %q", name)
			}
			for run := range 3 {
				// #nosec G204 -- name is explicitly limited to srcpack or srcpack.exe; bin is a fresh temporary directory.
				cmd := exec.CommandContext(t.Context(), filepath.Join(bin, name), "--help")
				cmd.Dir, cmd.Env = t.TempDir(), env
				output, err := cmd.CombinedOutput()
				if err != nil || !strings.Contains(string(output), "Usage:") {
					t.Fatalf("installed --help run %d (GOWORK=%s): %v\n%s", run, workspace, err, output)
				}
			}
		})
	}
}
