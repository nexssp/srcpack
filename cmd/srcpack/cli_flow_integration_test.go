package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/xtest/ktest"
	"github.com/nexssp/transport/tcli"
	tclinexssflow "github.com/nexssp/transport/tcli/nexssflow"

	"github.com/nexssp/srcpack/nexssflow"
)

func TestFlowCLIDemo(t *testing.T) {
	root := t.TempDir()
	for name, contents := range map[string]string{
		"app/main.go":      "package app\n\n// Hello is a demo function.\nfunc Hello() string { return \"hello\" }\n",
		"other/other.go":   "package other\n\nfunc ExcludedByTarget() {}\n",
		"app/main_test.go": "package app\n\nfunc TestHello() {}\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	transport := tcli.New(
		tcli.WithArgs("pack", "--target", "app", "--only-sig", "--no-tests"),
		tcli.WithOutput(&stdout, &stderr),
		tcli.WithExecutable("srcpack"),
	)
	cliBundle := tclinexssflow.Bundle(nil)
	cliBundle.Libraries[0] = tclinexssflow.LibraryWithTransport(transport)

	bundles := native.Bundles()
	//nolint:staticcheck // Verify that existing hosts can still use the deprecated aggregate.
	bundles = append(bundles, nexssflow.FullBundle(nil), cliBundle)
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		t.Fatalf("runner.BuildConfig() error = %v", err)
	}

	source, err := readNFlowFixture("cli_demo.nflow")
	if err != nil {
		t.Fatalf("read embedded CLI demo: %v", err)
	}
	_, err = runner.Execute(context.Background(), cfg, string(source), "nflows/cli_demo.nflow", nil)
	if err != nil {
		t.Fatalf("runner.Execute() error = %v; stderr: %s", err, stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{"main.go", "func Hello()"} {
		if !strings.Contains(output, want) {
			t.Errorf("CLI output %q does not contain %q", output, want)
		}
	}
	for _, unwanted := range []string{"other.go", "main_test.go", "TestHello", `return "hello"`} {
		if strings.Contains(output, unwanted) {
			t.Errorf("CLI output unexpectedly contains %q: %s", unwanted, output)
		}
	}
}

func TestFlowCLIDemoHelpUsesDeclaredSchema(t *testing.T) {
	var stdout, stderr bytes.Buffer
	transport := tcli.New(
		tcli.WithArgs("pack", "--help"),
		tcli.WithOutput(&stdout, &stderr),
		tcli.WithExecutable("srcpack"),
	)
	cliBundle := tclinexssflow.Bundle(nil)
	cliBundle.Libraries[0] = tclinexssflow.LibraryWithTransport(transport)
	bundles := native.Bundles()
	//nolint:staticcheck // Verify that existing hosts can still use the deprecated aggregate.
	bundles = append(bundles, nexssflow.FullBundle(nil), cliBundle)
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		t.Fatalf("runner.BuildConfig() error = %v", err)
	}
	source, err := readNFlowFixture("cli_demo.nflow")
	if err != nil {
		t.Fatalf("read embedded CLI demo: %v", err)
	}
	_, err = runner.Execute(context.Background(), cfg, string(source), "nflows/cli_demo.nflow", nil)
	if err != nil {
		t.Fatalf("runner.Execute() error = %v; stderr: %s", err, stderr.String())
	}
	for _, want := range []string{
		"Command: pack",
		"--target",
		"-t",
		"--only-sig",
		"--no-tests",
		"workspace paths to include (repeatable)",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("help output %q does not contain %q", stderr.String(), want)
		}
	}
	if strings.Contains(stderr.String(), "--output") || strings.Contains(stderr.String(), "--copy") {
		t.Errorf("demo help advertises unwired side effects: %s", stderr.String())
	}
}

func TestFlowCLIFlagMatrix(t *testing.T) {
	const fakeCredential = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"

	workspace := map[string]string{
		"main.go":         "package main\n\n// Public says hello.\nfunc Public() string { return \"hi\" }\n\nfunc private() {}\n",
		"main_test.go":    "package main\n\nfunc TestPublic(t *testing.T) {}\n",
		"lib.go":          "package main\n\nfunc Lib() {}\n",
		"config.yml":      "key: value\ntoken: " + fakeCredential + "\n",
		"generated.pb.go": "package main\n",
		"vendor/dep.go":   "package vendor\n",
	}

	tests := []struct {
		name       string
		args       []string
		wantSubstr []string
		dropSubstr []string
	}{
		{
			name:       "no flags walks whole workspace",
			wantSubstr: []string{"main.go", "lib.go", "config.yml", "main_test.go"},
			dropSubstr: []string{"vendor/"},
		},
		{
			name:       "target narrows to one file",
			args:       []string{"--target", "lib.go"},
			wantSubstr: []string{"lib.go"},
			dropSubstr: []string{"main.go", "config.yml"},
		},
		{
			name:       "file arg bypasses walk",
			args:       []string{"--file", "lib.go"},
			wantSubstr: []string{"lib.go"},
			dropSubstr: []string{"main.go"},
		},
		{
			name:       "ext restricts to go",
			args:       []string{"--ext", "go"},
			wantSubstr: []string{"main.go"},
			dropSubstr: []string{"config.yml"},
		},
		{
			name:       "no-tests excludes test files",
			args:       []string{"--no-tests"},
			wantSubstr: []string{"main.go"},
			dropSubstr: []string{"main_test.go"},
		},
		{
			name:       "only-sig strips bodies",
			args:       []string{"--only-sig"},
			wantSubstr: []string{"func Public() string"},
			dropSubstr: []string{`return "hi"`},
		},
		{
			name:       "include-config adds masked raw manifests to signatures",
			args:       []string{"--only-sig", "--include-config", "--mask"},
			wantSubstr: []string{"config.yml", "key: value", "[REDACTED_CREDENTIAL]"},
			dropSubstr: []string{`return "hi"`, fakeCredential},
		},
		{
			name:       "dense compacts signatures",
			args:       []string{"--dense", "--only-sig"},
			wantSubstr: []string{"Public()"},
			dropSubstr: []string{"package main"},
		},
		{
			name:       "comments preserves comments when signature-only",
			args:       []string{"--only-sig", "--comments"},
			wantSubstr: []string{"Public says hello"},
			dropSubstr: []string{`return "hi"`},
		},
		{
			name:       "mask replaces secrets",
			args:       []string{"--include-config", "--mask"},
			wantSubstr: []string{"[REDACTED_CREDENTIAL]"},
			dropSubstr: []string{fakeCredential},
		},
		{
			name:       "exclude drops matching paths",
			args:       []string{"--exclude", "*.yml"},
			wantSubstr: []string{"main.go"},
			dropSubstr: []string{"config.yml"},
		},
		{
			name:       "max-lines truncates oversized files",
			args:       []string{"--max-lines", "2"},
			wantSubstr: []string{"TRUNCATED MIDDLE"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := runFlowCLI(t, workspace, test.args...)
			for _, want := range test.wantSubstr {
				if !strings.Contains(stdout, want) {
					t.Errorf("output missing %q\n%s", want, stdout)
				}
			}
			for _, drop := range test.dropSubstr {
				if strings.Contains(stdout, drop) {
					t.Errorf("output unexpectedly contains %q\n%s", drop, stdout)
				}
			}
		})
	}
}

func TestFlowCLIPackInlinesSlicedSourceRanges(t *testing.T) {
	t.Run("line slices reset for each file", func(t *testing.T) {
		output := runFlowCLI(t, map[string]string{
			"first.go":  "package first\n\nfunc First() {}\n",
			"second.go": "package second\n\nfunc Second() {}\n",
		}, "--target", "first.go:3-3", "--target", "second.go:3-3")

		for _, want := range []string{"## first.go:3", "## second.go:3"} {
			if !strings.Contains(output, want) {
				t.Errorf("output missing inline source header %q:\n%s", want, output)
			}
		}
		if strings.Contains(output, "<!--") {
			t.Fatalf("output still contains a separate source-location comment:\n%s", output)
		}
	})

	t.Run("dense signature points to original declaration", func(t *testing.T) {
		output := runFlowCLI(t, map[string]string{
			"api.go": "package api\n\n// Public is visible.\nfunc Public(\n\tvalue int,\n) string {\n\treturn \"ok\"\n}\n",
		}, "--file", "api.go", "--only-sig", "--dense")

		if !strings.Contains(output, "## api.go") {
			t.Fatalf("whole-file header missing:\n%s", output)
		}
		if strings.Contains(output, "api.go:") || strings.Contains(output, "<!--") {
			t.Fatalf("whole-file source gained an inline range or separate comment:\n%s", output)
		}
		if !strings.Contains(output, "Public") {
			t.Fatalf("dense signature missing from output:\n%s", output)
		}
	})
}

func runFlowCLI(t *testing.T, workspace map[string]string, args ...string) string {
	t.Helper()

	root := t.TempDir()
	for name, body := range workspace {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	transport := tcli.New(
		tcli.WithArgs(append([]string{"pack"}, args...)...),
		tcli.WithOutput(&stdout, &stderr),
		tcli.WithExecutable("srcpack"),
	)
	cliBundle := tclinexssflow.Bundle(nil)
	cliBundle.Libraries[0] = tclinexssflow.LibraryWithTransport(transport)

	bundles := native.Bundles()
	//nolint:staticcheck // Verify that existing hosts can still use the deprecated aggregate.
	bundles = append(bundles, nexssflow.FullBundle(nil), cliBundle)
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		t.Fatalf("runner.BuildConfig: %v", err)
	}

	source, err := readNFlowFixture("cli_demo.nflow")
	if err != nil {
		t.Fatalf("read embedded CLI demo: %v", err)
	}
	if _, err := runner.Execute(context.Background(), cfg, string(source), "nflows/cli_demo.nflow", nil); err != nil {
		t.Fatalf("runner.Execute: %v\nstderr: %s", err, stderr.String())
	}
	return stdout.String()
}

func TestParseCLIRequest_ValueForms(t *testing.T) {
	t.Parallel()
	schema := tcli.CLIRequestSchema{
		Name: "Input",
		Fields: []tcli.CLIFieldSpec{
			{Name: "tags", Type: "[]string", Flags: []string{"tag", "t"}},
			{Name: "ext", Type: "string", Flags: []string{"ext"}},
			{Name: "verbose", Type: "bool", Flags: []string{"v", "verbose"}},
			{Name: "limit", Type: "int", Flags: []string{"limit"}},
			{Name: "ratio", Type: "float64", Flags: []string{"ratio"}},
			{Name: "path", Type: "string", Flags: []string{"path"}},
		},
	}

	tests := []struct {
		name    string
		args    []string
		want    map[string]any
		wantErr string
	}{
		// bool — four accepted spellings
		{name: "bool bare", args: []string{"--verbose"}, want: map[string]any{"verbose": true}},
		{name: "bool equals true", args: []string{"--verbose=true"}, want: map[string]any{"verbose": true}},
		{name: "bool equals false", args: []string{"--verbose=false"}, want: map[string]any{"verbose": false}},
		{name: "bool separate true", args: []string{"--verbose", "true"}, want: map[string]any{"verbose": true}},
		{name: "bool separate false", args: []string{"--verbose", "false"}, want: map[string]any{"verbose": false}},
		{name: "bool short bare", args: []string{"-v"}, want: map[string]any{"verbose": true}},
		{name: "bool last wins", args: []string{"--verbose", "--verbose=false"}, want: map[string]any{"verbose": false}},

		// string
		{name: "string separate", args: []string{"--ext", "go"}, want: map[string]any{"ext": "go"}},
		{name: "string equals", args: []string{"--ext=go"}, want: map[string]any{"ext": "go"}},
		{name: "string last wins", args: []string{"--ext", "go", "--ext", "ts"}, want: map[string]any{"ext": "ts"}},
		{name: "string value looks like flag", args: []string{"--ext", "-x"}, wantErr: "requires a value"},

		// slice
		{name: "slice repeated", args: []string{"--tag", "a", "--tag", "b"}, want: map[string]any{"tags": []string{"a", "b"}}},
		{name: "slice comma", args: []string{"--tag", "a,b"}, want: map[string]any{"tags": []string{"a", "b"}}},
		{name: "slice mixed", args: []string{"--tag", "a,b", "--tag", "c"}, want: map[string]any{"tags": []string{"a", "b", "c"}}},
		{name: "slice short alias", args: []string{"-t", "a"}, want: map[string]any{"tags": []string{"a"}}},
		{name: "slice equals form", args: []string{"--tag=a,b"}, want: map[string]any{"tags": []string{"a", "b"}}},
		{name: "slice whitespace trimmed", args: []string{"--tag", " a , b "}, want: map[string]any{"tags": []string{"a", "b"}}},

		// int / float
		{name: "int separate", args: []string{"--limit", "42"}, want: map[string]any{"limit": int64(42)}},
		{name: "int equals", args: []string{"--limit=42"}, want: map[string]any{"limit": int64(42)}},
		{name: "int negative separate", args: []string{"--limit", "-1"}, want: map[string]any{"limit": int64(-1)}},
		{name: "int negative equals", args: []string{"--limit=-1"}, want: map[string]any{"limit": int64(-1)}},
		{name: "int invalid", args: []string{"--limit", "abc"}, wantErr: "invalid syntax"},
		{name: "float", args: []string{"--ratio", "2.5"}, want: map[string]any{"ratio": 2.5}},

		// flag terminators and positionals
		{name: "double dash makes rest positional", args: []string{"--", "--verbose"}, wantErr: "unexpected positional"},
		{name: "unknown flag rejected", args: []string{"--missing"}, wantErr: "unknown flag"},
		{name: "short unknown flag rejected", args: []string{"-z"}, wantErr: "unknown flag"},

		// empty value semantics
		{name: "string equals empty produces empty", args: []string{"--ext="}, wantErr: "requires a value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := tcli.ParseCLIRequest(test.args, schema)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (payload %#v)", test.wantErr, got)
				}
				ktest.RequireStringContains(t, err.Error(), test.wantErr)
				return
			}
			ktest.RequireNoError(t, err)
			ktest.RequireEqual(t, got, test.want)
		})
	}
}

func TestParseCLIRequest_PositionalContract(t *testing.T) {
	t.Parallel()

	withPositional := tcli.CLIRequestSchema{
		Name: "Input",
		Fields: []tcli.CLIFieldSpec{
			{Name: "files", Type: "[]string", Positional: true},
			{Name: "ext", Type: "string", Flags: []string{"ext"}},
		},
	}
	withoutPositional := tcli.CLIRequestSchema{
		Name: "Input",
		Fields: []tcli.CLIFieldSpec{
			{Name: "ext", Type: "string", Flags: []string{"ext"}},
		},
	}

	t.Run("positional slice accumulates every bare argument", func(t *testing.T) {
		t.Parallel()
		got, err := tcli.ParseCLIRequest([]string{"a.go", "--ext", "go", "b.go"}, withPositional)
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, got, map[string]any{
			"files": []string{"a.go", "b.go"},
			"ext":   "go",
		})
	})

	t.Run("no positional field rejects bare argument", func(t *testing.T) {
		t.Parallel()
		_, err := tcli.ParseCLIRequest([]string{"a.go"}, withoutPositional)
		ktest.RequireStringContains(t, err.Error(), "unexpected positional argument")
	})

	t.Run("double dash routes rest to positional field", func(t *testing.T) {
		t.Parallel()
		got, err := tcli.ParseCLIRequest([]string{"--", "-not-a-flag", "file.go"}, withPositional)
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, got, map[string]any{
			"files": []string{"-not-a-flag", "file.go"},
		})
	})
}

func TestParseCLIRequest_RequiredAndUnknown(t *testing.T) {
	t.Parallel()

	schema := tcli.CLIRequestSchema{
		Name: "Input",
		Fields: []tcli.CLIFieldSpec{
			{Name: "name", Type: "string", Flags: []string{"name"}, Required: true},
			{Name: "count", Type: "int", Flags: []string{"count"}},
		},
	}

	t.Run("missing required field", func(t *testing.T) {
		t.Parallel()
		_, err := tcli.ParseCLIRequest([]string{"--count", "3"}, schema)
		ktest.RequireStringContains(t, err.Error(), "missing required CLI field")
	})

	t.Run("present required field", func(t *testing.T) {
		t.Parallel()
		got, err := tcli.ParseCLIRequest([]string{"--name", "x"}, schema)
		ktest.RequireNoError(t, err)
		ktest.RequireEqual(t, got, map[string]any{"name": "x"})
	})

	t.Run("unknown flag names the offending key", func(t *testing.T) {
		t.Parallel()
		_, err := tcli.ParseCLIRequest([]string{"--name", "x", "--typo", "y"}, schema)
		ktest.RequireStringContains(t, err.Error(), "unknown flag: -typo")
	})
}
