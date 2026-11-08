package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/nflows"
)

func TestParseCLIExampleRetainsSelectorsAndExplicitFile(t *testing.T) {
	cli, err := parseCLI([]string{"profile,folder#arch", "--file=asdd", "as"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cli.Config.Targets, []string{"profile", "folder", "as"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if got, want := cli.Config.Files, []string{"asdd"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
	if !cli.Config.Dense || !cli.Config.OnlySig || cli.Config.Comments {
		t.Fatalf("arch profile not applied: %+v", cli.Config)
	}
}

func TestParseCLIShorthandDenseAndRepeatedPositionals(t *testing.T) {
	cli, err := parseCLI([]string{"abc", "src", "--dense"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cli.Config.Targets, []string{"abc", "src"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if !cli.Config.Dense || !cli.Config.OnlySig || cli.Config.Comments {
		t.Fatalf("--dense should enable dense signatures and strip comments: %+v", cli.Config)
	}
}

func TestParseCLITargetFlagUsesSelectorGrammarAndPreservesOrder(t *testing.T) {
	cli, err := parseCLI([]string{"first", "--target=folder#arch", "last"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cli.Config.Targets, []string{"first", "folder", "last"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if cli.Config.Dense != true {
		t.Fatalf("selector profile arch should be applied: %+v", cli.Config)
	}
}

func TestParseCLIBareProfileNameIsAPathWhenItExistsUnderCwd(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "arch"), 0o755); err != nil {
		t.Fatal(err)
	}
	cli, err := parseCLI([]string{"--cwd", root, "arch"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cli.Config.Targets, []string{"arch"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if cli.Config.Dense {
		t.Fatalf("existing target path was misread as a profile: %+v", cli.Config)
	}
}

func TestParseCLIMixedSelectorProfilesAndModifiers(t *testing.T) {
	root := t.TempDir()
	cli, err := parseCLI([]string{
		"--cwd", root,
		"framework~vendor#dense",
		"ai#source@Review this subsystem",
		"--file=explicit.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cli.Config.Targets, []string{"framework", "ai"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if cli.Config.TargetProfiles[selectorTargetKey("framework", root)] != "dense" || cli.Config.TargetProfiles[selectorTargetKey("ai", root)] != "source" {
		t.Fatalf("per-target profiles = %#v", cli.Config.TargetProfiles)
	}
	if got := cli.Config.TargetExcludes[selectorTargetKey("framework", root)]; got != "vendor" {
		t.Fatalf("framework excludes = %q, want vendor", got)
	}
	if got := cli.Config.TargetPrompts[selectorTargetKey("ai", root)]; got != "Review this subsystem" {
		t.Fatalf("ai prompt = %q", got)
	}
	if got, want := cli.Config.Files, []string{"explicit.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit files = %#v, want %#v", got, want)
	}
}

func TestSelectorTargetKeyStripsLineRange(t *testing.T) {
	root := t.TempDir()
	got := selectorTargetKey("src/file.go:10-20", root)
	want := filepath.Join(root, "src/file.go")
	if got != want {
		t.Fatalf("selectorTargetKey = %q, want %q", got, want)
	}
}

func TestParseCLIExplicitOptionsAndSelectorProfileOrder(t *testing.T) {
	cli, err := parseCLI([]string{"arch#src", "--file", "one.go", "--file=two.go", "-C", ".", "-L", "3", "-o", "out.md", "--no-comments"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cli.Config.Targets, []string{"src"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if got, want := cli.Config.Files, []string{"one.go", "two.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
	if cli.Config.Output != "out.md" || cli.Cwd != "." || cli.Config.Depth != 3 {
		t.Fatalf("options not retained: %+v, cwd=%q", cli.Config, cli.Cwd)
	}
	if !cli.Config.Dense || !cli.Config.OnlySig || cli.Config.Comments {
		t.Fatalf("profile/explicit comments not applied: %+v", cli.Config)
	}
}

func TestParseCLIRejectsUnknownFlagsAndProfiles(t *testing.T) {
	for _, args := range [][]string{{"--not-a-real-option"}, {"src#unknown"}, {"--depth", "0"}, {"--color", "blink"}, {"--editor", "browser"}} {
		if _, err := parseCLI(args); err == nil {
			t.Errorf("parseCLI(%q) returned no error", args)
		}
	}
}

func TestParseSelectorKeepsAllCommaSeparatedPaths(t *testing.T) {
	parsed := parseSelector("one,two~vendor,testdata#api@Review this")
	if got, want := parsed.Targets, []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if parsed.Profile != "api" || parsed.Prompt != "Review this" {
		t.Fatalf("selector metadata = %+v", parsed)
	}
	if got, want := parsed.Excludes, []string{"vendor", "testdata"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("excludes = %#v, want %#v", got, want)
	}
}

func TestClipboardOptionDoesNotConsumeFollowingSelectorAndCwdUsesUppercaseC(t *testing.T) {
	cli, err := parseCLI([]string{"-c", "src", "-C", "."})
	if err != nil {
		t.Fatal(err)
	}
	if !cli.Config.Copy {
		t.Fatal("-c should enable clipboard output")
	}
	if got, want := cli.Config.Targets, []string{"src"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
	if cli.Cwd != "." {
		t.Fatalf("-C cwd = %q, want .", cli.Cwd)
	}
}

func TestLongCopyOptionAndNoCopyOverride(t *testing.T) {
	cli, err := parseCLI([]string{"--copy", "--no-copy", "src"})
	if err != nil {
		t.Fatal(err)
	}
	if cli.Config.Copy {
		t.Fatal("--no-copy should disable a preceding --copy")
	}
	if got, want := cli.Config.Targets, []string{"src"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestOptionErrorsIncludeValidEditorAndColorValues(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"--editor"}, []string{"requires a value", "markdown", "none", "fold"}},
		{[]string{"--editor=bad"}, []string{"expects one of", "markdown", "none", "fold"}},
		{[]string{"--color"}, []string{"requires a value", "auto", "always", "never"}},
		{[]string{"--color=blink"}, []string{"expects one of", "auto", "always", "never"}},
	} {
		_, err := parseCLI(test.args)
		if err == nil {
			t.Errorf("parseCLI(%q) returned no error", test.args)
			continue
		}
		for _, part := range test.want {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("parseCLI(%q) error = %q, want it to contain %q", test.args, err, part)
			}
		}
	}
}

func TestHelpListsEditorColorAndClipboardChoices(t *testing.T) {
	var help bytes.Buffer
	printHelp(&help)
	for _, choice := range []string{
		"-c, --copy",
		"-C, --cwd DIR",
		"markdown, raw, none, text, zed, claude, fold",
		"auto, always, never",
	} {
		if !strings.Contains(help.String(), choice) {
			t.Errorf("help text is missing %q", choice)
		}
	}
}

func TestCopyFlagFeedsRootOutputPolicy(t *testing.T) {
	for _, args := range [][]string{nil, {"-c", "src"}, {"-c", "-q", "src"}} {
		cli, err := parseCLI(args)
		if err != nil {
			t.Fatal(err)
		}
		values, err := rootIO(cli, resolved{})
		if err != nil {
			t.Fatal(err)
		}
		if values["copy"] != cli.Config.Copy || values["quiet"] != cli.Config.Quiet {
			t.Fatalf("clipboard config for %q: %v", args, values)
		}
	}
	source, err := nflows.FS.ReadFile("output.nflow")
	if err != nil {
		t.Fatal(err)
	}
	sourceText := strings.Join(strings.Fields(string(source)), " ")
	if !strings.Contains(sourceText, ".policy.copy -> srcpack.copy_output") {
		t.Fatal("output.nflow must explicitly choose the clipboard action")
	}
}
