package initcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/extensions/require"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/transport/tcli"
	tclinexssflow "github.com/nexssp/transport/tcli/nexssflow"

	_ "github.com/nexssp/srcpack/bundles/config_cli"
	_ "github.com/nexssp/srcpack/bundles/manage"
	_ "github.com/nexssp/srcpack/bundles/pack"
	_ "github.com/nexssp/srcpack/bundles/system"
	_ "github.com/nexssp/srcpack/bundles/update"
	_ "github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

func TestInspectReportsExistingConfigFacts(t *testing.T) {
	cwd := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(cwd, "nexss.srcpack.yaml"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := inspectWorkspace(context.Background(), map[string]any{"reset": true})
	if err != nil {
		t.Fatal(err)
	}
	if out["config_path"] != filepath.Join(cwd, "nexss.srcpack.yaml") {
		t.Errorf("config_path = %v, want existing alias", out["config_path"])
	}
	if out["config_exists"] != true || out["config_name"] != "nexss.srcpack.yaml" {
		t.Errorf("config facts = exists:%v name:%v", out["config_exists"], out["config_name"])
	}
}

func TestActionsGenerateSortedTargetsAndPreserveIgnore(t *testing.T) {
	cwd := chdirTemp(t)
	for _, dir := range []string{"zeta", "alpha"} {
		if err := os.Mkdir(filepath.Join(cwd, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ignorePath := filepath.Join(cwd, "nexss.srcpack.ignore")
	if err := os.WriteFile(ignorePath, []byte("user ignore\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := inspectWorkspace(context.Background(), map[string]any{
		"mode": "targets", "dirs": []string{"zeta", "alpha"}, "force": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err = generateTargets(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := state["config_body"].(string)
	if !ok {
		t.Fatalf("config_body has type %T, want string", state["config_body"])
	}
	if strings.Index(body, "  alpha:\n") > strings.Index(body, "  zeta:\n") {
		t.Errorf("target output is not sorted:\n%s", body)
	}
	if _, err := writeFilesAtomic(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	gotIgnore, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotIgnore) != "user ignore\n" {
		t.Errorf("existing ignore overwritten: %q", gotIgnore)
	}
}

func TestInitTargetsModeWritesGeneratedConfig(t *testing.T) {
	cwd := chdirTemp(t)
	if err := os.Mkdir(filepath.Join(cwd, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := runInitFlow(t, "init", "targets", "src")
	if legacy.err != nil {
		t.Fatalf("legacy init targets failed: %v\nstderr:\n%s", legacy.err, legacy.stderr)
	}
	generated, err := os.ReadFile(filepath.Join(cwd, "nexss.srcpack.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "  src:\n    dir:\n      - src\n") {
		t.Errorf("legacy targets spelling did not generate target config:\n%s", generated)
	}
}

func TestResetRemovesOnlyGeneratedFiles(t *testing.T) {
	cwd := chdirTemp(t)
	root := filepath.Join(cwd, "_srcpack")
	for name, body := range map[string]string{
		"nested/generated.md": "markdown", "temp.tmp": "tmp", "result.xml": "xml", "keep.go": "source",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := cleanGeneratedDir(context.Background(), map[string]any{"cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nested/generated.md", "temp.tmp", "result.xml"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Errorf("generated file %s still exists (err=%v)", name, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(root, "keep.go")); err != nil || string(got) != "source" {
		t.Errorf("user file changed: %q, err=%v", got, err)
	}
}

func TestGenerateErrorOccursBeforeResetInFlow(t *testing.T) {
	cwd := chdirTemp(t)
	keep := filepath.Join(cwd, "_srcpack", "keep.md")
	if err := os.MkdirAll(filepath.Dir(keep), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep until validation passes"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runInitFlow(t, "init", "--mode", "targets", "--reset", "missing-dir")
	if result.err == nil {
		t.Fatal("runner.Execute() succeeded, want invalid target failure")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("reset removed generated file before target validation: %v", err)
	}
}

func TestDirectNFlowRunnerUsesDeclaredSchemaAndDryRun(t *testing.T) {
	cwd := chdirTemp(t)
	result, actionOutput := captureStdout(t, func() initRunResult {
		return runInitFlow(t, "init", "--dry-run")
	})
	if result.err != nil {
		t.Fatalf("runner.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
	}
	if !strings.Contains(actionOutput, "[DRY-RUN] would write") {
		t.Errorf("action stdout %q does not show preview", actionOutput)
	}
	if strings.TrimSpace(result.stdout) == "null" {
		t.Errorf("tcli rendered a null result: %q", result.stdout)
	}
	if _, err := os.Stat(filepath.Join(cwd, "nexss.srcpack.yml")); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote config (stat err=%v)", err)
	}
}

func TestResetDryRunKeepsGeneratedFiles(t *testing.T) {
	cwd := chdirTemp(t)
	generated := filepath.Join(cwd, "_srcpack", "keep.md")
	if err := os.MkdirAll(filepath.Dir(generated), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, actionOutput := captureStdout(t, func() initRunResult {
		return runInitFlow(t, "init", "--reset", "--dry-run")
	})
	if result.err != nil {
		t.Fatalf("init --reset --dry-run failed: %v\n%s", result.err, result.stderr)
	}
	if !strings.Contains(actionOutput, "[DRY-RUN] would remove 1 generated file(s)") {
		t.Errorf("cleanup preview not reported: %q", actionOutput)
	}
	if _, err := os.Stat(generated); err != nil {
		t.Errorf("dry-run removed generated file: %v", err)
	}
}

type initRunResult struct {
	stdout string
	stderr string
	err    error
}

func runInitFlow(t *testing.T, args ...string) initRunResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	transport := tcli.New(
		tcli.WithArgs(args...),
		tcli.WithOutput(&stdout, &stderr),
		tcli.WithExecutable("srcpack"),
	)
	root, err := nflows.FS.ReadFile("srcpack.nflow")
	if err != nil {
		return initRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}
	base, err := runner.BuildConfig(native.Bundles())
	if err != nil {
		return initRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	prepared, err := compiler.PrepareSource(ctx, base.Directives, string(root), "nflows/srcpack.nflow", base.CompileOpts...)
	if err != nil {
		return initRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}
	reqs, err := require.FromMeta(prepared.Metadata())
	if err != nil {
		return initRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}
	bundles := native.Bundles()
	seen := make(map[string]bool, len(bundles))
	for _, bundle := range bundles {
		seen[bundle.ID] = true
	}
	for _, req := range reqs {
		if seen[require.NormalizeID(req.Import)] && req.Alias == "" {
			continue
		}
		selected, resolveErr := require.ResolveBundles([]require.Requirement{req})
		if resolveErr != nil {
			return initRunResult{stdout: stdout.String(), stderr: stderr.String(), err: resolveErr}
		}
		for _, bundle := range selected {
			if seen[bundle.ID] {
				continue
			}
			if bundle.ID == "tcli" {
				bundle.Libraries[0] = tclinexssflow.LibraryWithTransport(transport)
			}
			bundles = append(bundles, bundle)
			seen[bundle.ID] = true
		}
	}
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		return initRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}
	_, err = runner.Execute(ctx, cfg, string(root), "nflows/srcpack.nflow", nil)
	return initRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func captureStdout(t *testing.T, run func() initRunResult) (initRunResult, string) {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	result := run()
	_ = writer.Close()
	os.Stdout = old
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return result, string(output)
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	cwd := t.TempDir()
	t.Chdir(cwd)
	return cwd
}
