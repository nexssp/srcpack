package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nexssp/transport/tcli"

	"github.com/nexssp/srcpack/bundles/cliinput"
	"github.com/nexssp/srcpack/bundles/operators"
	packcmd "github.com/nexssp/srcpack/bundles/pack"
)

// This mirrors the CLI fields exercised here from bundles/pack/schema.nflow.
// The actual parser is tcli; the small schema keeps these tests independent of
// root-pipeline compilation while still testing the typed-input boundary.
func flowParityExtraSchema() tcli.CLIRequestSchema {
	return tcli.CLIRequestSchema{
		Name: "PackInput",
		Fields: []tcli.CLIFieldSpec{
			{Name: "targets", Type: "[]string", Flags: []string{"target", "t"}},
			{Name: "selectors", Type: "[]string", Positional: true},
			{Name: "files", Type: "[]string", Flags: []string{"file", "f"}},
			{Name: "profile", Type: "string", Flags: []string{"profile", "p"}},
			{Name: "prompt", Type: "string", Flags: []string{"prompt"}},
			{Name: "output", Type: "string", Flags: []string{"output", "o"}},
			{Name: "copy", Type: "bool", Flags: []string{"copy", "c"}},
			{Name: "no_copy", Type: "bool", Flags: []string{"no-copy"}},
			{Name: "comments", Type: "bool", Flags: []string{"comments"}},
			{Name: "no_comments", Type: "bool", Flags: []string{"no-comments"}},
			{Name: "examples", Type: "bool", Flags: []string{"examples"}},
			{Name: "no_examples", Type: "bool", Flags: []string{"no-examples"}},
			{Name: "no_tests", Type: "bool", Flags: []string{"no-tests"}},
		},
	}
}

func flowParityExtraResolved(t *testing.T, args []string) (operators.PackConfig, operators.PackConfig) {
	t.Helper()

	legacyCLI, err := parseCLI(args)
	if err != nil {
		t.Fatalf("legacy parseCLI(%q): %v", args, err)
	}
	legacy, err := resolve(context.Background(), legacyCLI, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("legacy resolve(%q): %v", args, err)
	}

	protected, marker := cliinput.ProtectCommas(args)
	parsed, err := tcli.ParseCLIRequest(protected, flowParityExtraSchema())
	if err != nil {
		t.Fatalf("tcli.ParseCLIRequest(%q): %v", args, err)
	}
	ctx := cliinput.WithInvocation(cliinput.WithArguments(context.Background(), args), "pack", true, marker)
	ctx = cliinput.WithRawArguments(ctx, protected)
	ctx = cliinput.WithInputSchemas(ctx, map[string]tcli.CLIRequestSchema{"PackInput": flowParityExtraSchema()})
	restored, err := cliinput.Restore(ctx, parsed)
	if err != nil {
		t.Fatalf("cliinput.Restore(%q): %v", args, err)
	}
	resolved, err := packcmd.ResolveInput(ctx, restored)
	if err != nil {
		t.Fatalf("packcmd.ResolveInput(%q): %v", args, err)
	}
	packed, ok := resolved["pack"].(map[string]any)
	if !ok {
		t.Fatalf("ResolveInput pack has type %T, want map[string]any", resolved["pack"])
	}
	data, err := json.Marshal(packed)
	if err != nil {
		t.Fatalf("marshal new pack config: %v", err)
	}
	var newPack operators.PackConfig
	if err := json.Unmarshal(data, &newPack); err != nil {
		t.Fatalf("decode new pack config: %v", err)
	}
	legacy.Pack.Copy = legacyCLI.Config.Copy
	ioFields, _ := resolved["io"].(map[string]any)
	newPack.Copy, _ = ioFields["copy"].(bool)
	return legacy.Pack, newPack
}

func requireFlowParityExtra(t *testing.T, args []string) {
	t.Helper()
	legacy, current := flowParityExtraResolved(t, args)
	for _, field := range []struct {
		name string
		old  any
		new  any
	}{
		{"Targets", legacy.Targets, current.Targets},
		{"Files", legacy.Files, current.Files},
		{"TargetProfiles", legacy.TargetProfiles, current.TargetProfiles},
		{"TargetPrompts", legacy.TargetPrompts, current.TargetPrompts},
		{"TargetExcludes", legacy.TargetExcludes, current.TargetExcludes},
		{"Comments", legacy.Comments, current.Comments},
		{"NoExamples", legacy.NoExamples, current.NoExamples},
		{"NoTests", legacy.NoTests, current.NoTests},
	} {
		if !reflect.DeepEqual(field.old, field.new) {
			t.Errorf("CLI parity mismatch for %s with args %q: legacy=%#v new=%#v", field.name, args, field.old, field.new)
		}
	}
	if cliinput.HasFlag(cliinput.WithArguments(context.Background(), args), "copy", "c", "no-copy") && legacy.Copy != current.Copy {
		t.Errorf("CLI parity mismatch for Copy with args %q: legacy=%t new=%t", args, legacy.Copy, current.Copy)
	}
}

func TestFlowParityExtra_GlobalFalseOverrides(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "nexss.srcpack.yml"), []byte("options:\n  strip_comments: true\n  no_tests: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	legacy, current := flowParityExtraResolved(t, []string{"--comments=true", "--no-tests=false"})
	if legacy.Comments != current.Comments || legacy.NoTests != current.NoTests {
		t.Fatalf("explicit false/true CLI values must override config defaults: legacy comments/no-tests=%t/%t, new=%t/%t", legacy.Comments, legacy.NoTests, current.Comments, current.NoTests)
	}
	if !legacy.Comments || legacy.NoTests {
		t.Fatalf("test fixture not exercising overrides: legacy comments/no-tests=%t/%t", legacy.Comments, legacy.NoTests)
	}
}

func TestFlowParityExtra_OrderedBooleanFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "comments then no-comments", args: []string{"--comments", "--no-comments"}},
		{name: "no-comments then comments", args: []string{"--no-comments", "--comments"}},
		{name: "examples then no-examples", args: []string{"--examples", "--no-examples"}},
		{name: "no-examples then examples", args: []string{"--no-examples", "--examples"}},
		{name: "copy then no-copy", args: []string{"--copy", "--no-copy"}},
		{name: "no-copy then copy", args: []string{"--no-copy", "--copy"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireFlowParityExtra(t, test.args)
		})
	}
}

func TestFlowParityExtra_CommaAndSpacePaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"comma,name.go", filepath.Join("folder with spaces", "space file.go")} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	requireFlowParityExtra(t, []string{"--file", "comma,name.go", "--file", filepath.Join("folder with spaces", "space file.go")})
}

func TestFlowParityExtra_SelectorPromptWithCommas(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	requireFlowParityExtra(t, []string{"source dir@inline prompt, with commas"})
}

func TestFlowParityExtra_TargetGroupingOrder(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"before", "flagged", "after"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	requireFlowParityExtra(t, []string{"before", "--target", "flagged", "after"})
}
