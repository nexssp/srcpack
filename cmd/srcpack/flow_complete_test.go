package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"

	"github.com/nexssp/srcpack/bundles/config"
	"github.com/nexssp/srcpack/nflows"
)

func TestEveryCommandIsDeclaredInNFlowSourceModules(t *testing.T) {
	cfg, err := runner.BuildConfig(native.Bundles())
	if err != nil {
		t.Fatal(err)
	}
	body, err := nflows.FS.ReadFile("srcpack.nflow")
	if err != nil {
		t.Fatal(err)
	}
	ctx := compiler.WithSourceLoader(context.Background(), nflows.Loader())
	prepared, err := compiler.PrepareSource(ctx, cfg.Directives, string(body), "srcpack.nflow", cfg.CompileOpts...)
	if err != nil {
		t.Fatal(err)
	}
	commands := map[string]bool{}
	var collectCommands func(map[string]any)
	collectCommands = func(meta map[string]any) {
		modifiers, _ := meta["pipeline_modifiers"].(map[string][]string)
		for _, mods := range modifiers {
			for _, modifier := range mods {
				if trimmed, ok := strings.CutPrefix(modifier, "cli="); ok {
					name, _, _ := strings.Cut(trimmed, ":")
					commands[name] = true
				}
			}
		}
		for _, imported := range compiler.SourceImports(meta) {
			collectCommands(imported.Source.Metadata())
		}
	}
	collectCommands(prepared.Metadata())

	for _, name := range []string{"init", "pack", "tree", "update", "config", "target", "prompt", "profile", "editor", "view", "dashboard", "usage", "version", "list", "info", "clean", "doctor", "types", "health", "demo", "cookbook"} {
		if !commands[name] {
			t.Errorf("command %s is not declared in the NFlow source modules", name)
		}
	}
}

func TestProductionHostContainsOnlyBootstrap(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	var production []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			production = append(production, name)
		}
	}
	sort.Strings(production)
	// cli_error.go is presentation only; command dispatch remains in NFlow.
	if strings.Join(production, ",") != "cli_error.go,flow_cli.go,main.go" {
		t.Fatalf("legacy command handlers remain in executable: %v", production)
	}
}

func TestFlowAdministrativeResourcesRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"target", "add", "api", "src", "other,dir"},
		{"target", "show", "api"},
		{"target", "list"},
		{"prompt", "add", "assist", "help"},
		{"prompt", "show", "assist"},
		{"prompt", "ls"},
		{"profile", "show", "arch"},
		{"profile", "list"},
		{"editor", "show", "markdown"},
		{"editor", "list"},
		{"prompt", "rm", "assist"},
		{"target", "delete", "api"},
	} {
		if _, err := capturePolicyStdout(t, func() error { return execute(ctx, args) }); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"target", "add", "missing"}, {"prompt", "show", "absent"}, {"profile", "delete", "arch"}, {"editor", "add", "x"}, {"target", "--unknown"}} {
		if err := execute(ctx, args); err == nil {
			t.Fatalf("invalid request accepted: %v", args)
		}
	}
}

func TestFlowCookbookPrintsPracticalExamples(t *testing.T) {
	t.Chdir(t.TempDir())
	text, err := capturePolicyStdout(t, func() error { return execute(context.Background(), []string{"cookbook"}) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# srcpack cookbook",
		`srcpack "internal/auth#api" "internal/store#with-tests"`,
		`service.go:80-#source`,
		`service.go:-140#source`,
		"github:OWNER/REPO//PATH?ref=REF",
		"--modified --diff-only",
		"srcpack view list",
		"srcpack target add auth internal/auth",
		`srcpack ".#arch" --include-config -o context.md`,
		"best-effort basis",
		".env` variants and common credential/private-key files remain excluded",
		`srcpack "internal/auth/service.go:80-140#source" -o -`,
		`<!-- srcpack-source: {"path":"internal/auth/service.go","start_line":80,"end_line":140} -->`,
		`"symbols":[{"name":"Lookup","kind":"func","start_line":12,"end_line":18}]`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("cookbook output omitted %q", want)
		}
	}
}

func TestFlowSystemCommandsAndSafeClean(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	ctx := context.Background()
	if err := execute(ctx, []string{"help", "target"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"help"}, {"--version"}, {"list"}, {"info"}, {"info", "sources"}, {"types"}, {"health"}, {"demo"}, {"cookbook"}} {
		text, err := capturePolicyStdout(t, func() error { return execute(ctx, args) })
		if err != nil || text == "" {
			t.Fatalf("%v: %v output=%q", args, err, text)
		}
		if len(args) > 0 && args[0] == "demo" && (!strings.Contains(text, "Add(") || strings.Contains(text, "return a + b") || strings.Contains(text, "internal")) {
			t.Fatalf("demo did not apply the requested AST policy: %q", text)
		}
		if strings.HasSuffix(strings.TrimSpace(text), "\nnull") {
			t.Fatalf("internal JSON leaked for %v: %q", args, text)
		}
	}
	if err := os.Mkdir("_srcpack", 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.xml", "c.tmp", "keep.go"} {
		if err := os.WriteFile(filepath.Join("_srcpack", name), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := execute(ctx, []string{"clean", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("_srcpack/a.md"); err != nil {
		t.Fatal("dry-run deleted a file")
	}
	if err := execute(ctx, []string{"clean"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("_srcpack/a.md"); !os.IsNotExist(err) {
		t.Fatal("generated file was not removed")
	}
	if _, err := os.Stat("_srcpack/keep.go"); err != nil {
		t.Fatal("non-generated file removed")
	}
}

func TestFlowProfileAliasesAndTypesOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		canonical string
	}{
		{"  ARCHITECTURE-MAP  ", "arch"},
		{" FULL ", "source"},
		{" Sources ", "source"},
		{" WithTests ", "with-tests"},
	} {
		got, err := capturePolicyStdout(t, func() error { return execute(ctx, []string{"info", tc.name}) })
		profile, ok := config.FindBuiltinProfile(tc.canonical)
		if !ok {
			t.Fatalf("canonical profile %q missing from metadata", tc.canonical)
		}
		want := profile.Name + ": " + profile.Description + "\n"
		if err != nil || got != want {
			t.Errorf("info %q: output=%q err=%v, want %q", tc.name, got, err, want)
		}
	}
	if err := execute(ctx, []string{"info", "wat"}); err == nil || !strings.Contains(err.Error(), `unknown profile "wat"; run 'srcpack list' to see built-in profiles`) {
		t.Fatalf("unknown profile error = %v", err)
	}

	got, err := capturePolicyStdout(t, func() error { return execute(ctx, []string{"types"}) })
	if err != nil {
		t.Fatal(err)
	}
	want := `Available file types:
  config:      *.json, *.yaml, *.yml, *.toml, *.ini
  go:          *.go
  javascript:  *.js, *.jsx, *.mjs, *.cjs
  js:          *.js, *.jsx, *.mjs, *.cjs
  markdown:    *.md, *.mdx, *.markdown
  md:          *.md, *.mdx, *.markdown
  php:         *.php
  py:          *.py, *.pyi
  python:      *.py, *.pyi
  rs:          *.rs
  rust:        *.rs
  svelte:      *.svelte
  ts:          *.ts, *.tsx
  typescript:  *.ts, *.tsx
  vue:         *.vue
  web:         *.html, *.htm, *.css, *.scss, *.js, *.jsx, *.ts, *.tsx, *.svelte, *.vue
  zig:         *.zig

Filter by extension suffixes with --ext (not type labels): srcpack . --ext json,yaml
`
	if got != want {
		t.Fatalf("types output = %q, want %q", got, want)
	}
}
