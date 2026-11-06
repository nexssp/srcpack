package configcmd

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

	"github.com/nexssp/srcpack/bundles/cliinput"
	_ "github.com/nexssp/srcpack/bundles/init"
	_ "github.com/nexssp/srcpack/bundles/manage"
	_ "github.com/nexssp/srcpack/bundles/pack"
	_ "github.com/nexssp/srcpack/bundles/system"
	_ "github.com/nexssp/srcpack/bundles/update"
	_ "github.com/nexssp/srcpack/nexssflow"
	"github.com/nexssp/srcpack/nflows"
)

func TestInspectReturnsFilesystemFactsWithoutChoosingCommand(t *testing.T) {
	got, err := inspect(context.Background(), map[string]any{"extra": "kept"})
	if err != nil {
		t.Fatal(err)
	}
	if got["sub"] != nil || got["extra"] != "kept" || got["cwd"] == "" {
		t.Fatalf("inspect result = %#v, want filesystem facts and preserved fields, no command policy", got)
	}
}

func TestPathPrintsMissingConfigMessage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	stdout, _, err := captureOutput(t, func() error {
		_, err := printPath(context.Background(), map[string]any{"text": "(no config file present)"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stdout, "(no config file present)\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestShowWritesEffectiveYAML(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("nexss.srcpack.yml", []byte("options:\n  no_tests: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := captureOutput(t, func() error {
		_, err := show(context.Background(), nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "options:") || !strings.Contains(stdout, "no_tests: false") {
		t.Fatalf("show output does not contain effective YAML: %q", stdout)
	}
}

func TestValidateActionPrintsReportAndReturnsDecision(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("nexss.srcpack.yml", []byte("options: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := captureOutput(t, func() error {
		input, err := inspect(context.Background(), map[string]any{"sub": "validate"})
		if err != nil {
			return err
		}
		out, err := validate(context.Background(), input)
		if err != nil {
			return err
		}
		if out["passed"] != true || out["error"] != nil {
			t.Errorf("validate result = %#v, want passed=true", out)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" || !strings.Contains(stderr, "Srcpack doctor") || !strings.Contains(stderr, "All checks passed.") {
		t.Fatalf("unexpected validation output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestConfigCommandThroughRunnerAndTCLI(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "nexss.srcpack.yml")
	if err := os.WriteFile(configPath, []byte("options: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeConfigCLI(t, "config", "path")
	if err != nil {
		t.Fatalf("config path: %v; stderr=%q", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != configPath {
		t.Fatalf("config path stdout = %q, want %q", got, configPath)
	}

	stdout, stderr, err = executeConfigCLI(t, "config")
	if err != nil {
		t.Fatalf("default config show: %v; stderr=%q", err, stderr)
	}
	if !strings.Contains(stdout, "options:") || strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("default show output is not plain YAML: %q", stdout)
	}
}

func TestConfigValidationFailureIsRoutedByFlow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("nexss.srcpack.yml", []byte("targets:\n  broken:\n    dir:\n      - missing-directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := executeConfigCLI(t, "config", "validate")
	if err == nil || !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("execute error = %v, want Flow validation failure; stderr=%q", err, stderr)
	}
	if stdout != "" || !strings.Contains(stderr, "Srcpack doctor") {
		t.Fatalf("unexpected failed validation output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func executeConfigCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	transport := tcli.New(
		tcli.WithArgs(args...),
		tcli.WithOutput(&stdout, &stderr),
		tcli.WithExecutable("srcpack"),
	)
	root, err := nflows.FS.ReadFile("srcpack.nflow")
	if err != nil {
		return stdout.String(), stderr.String(), err
	}
	base, err := runner.BuildConfig(native.Bundles())
	if err != nil {
		return stdout.String(), stderr.String(), err
	}
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	prepared, err := compiler.PrepareSource(ctx, base.Directives, string(root), "nflows/srcpack.nflow", base.CompileOpts...)
	if err != nil {
		return stdout.String(), stderr.String(), err
	}
	reqs, err := require.FromMeta(prepared.Metadata())
	if err != nil {
		return stdout.String(), stderr.String(), err
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
			return stdout.String(), stderr.String(), resolveErr
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
		return stdout.String(), stderr.String(), err
	}
	marker := ""
	ctx = cliinput.WithInvocation(ctx, "config", false, marker)
	var runErr error
	processStdout, processStderr, captureErr := captureOutput(t, func() error {
		_, runErr = runner.Execute(ctx, cfg, string(root), "nflows/srcpack.nflow", nil)
		return runErr
	})
	if captureErr != nil && runErr == nil {
		return processStdout + stdout.String(), processStderr + stderr.String(), captureErr
	}
	return processStdout + stdout.String(), processStderr + stderr.String(), runErr
}

func captureOutput(t *testing.T, run func() error) (string, string, error) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		return "", "", err
	}
	os.Stdout, os.Stderr = outW, errW
	var stdout, stderr bytes.Buffer
	outDone := make(chan error, 1)
	errDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&stdout, outR)
		outDone <- err
	}()
	go func() {
		_, err := io.Copy(&stderr, errR)
		errDone <- err
	}()
	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		_ = outW.Close()
		_ = errW.Close()
		_ = outR.Close()
		_ = errR.Close()
	}()
	runErr := run()
	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outCopyErr := <-outDone
	errCopyErr := <-errDone
	_ = outR.Close()
	_ = errR.Close()
	if outCopyErr != nil {
		return stdout.String(), stderr.String(), outCopyErr
	}
	if errCopyErr != nil {
		return stdout.String(), stderr.String(), errCopyErr
	}
	return stdout.String(), stderr.String(), runErr
}
