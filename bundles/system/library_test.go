package systemcmd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/bundles/config"
)

func TestWithVersionIsContextScoped(t *testing.T) {
	if got := Version(context.Background()); got != "dev" {
		t.Fatalf("default version = %q", got)
	}
	one := WithVersion(context.Background(), "1.2.3")
	two := WithVersion(context.Background(), "2.0.0")
	if got := Version(one); got != "1.2.3" {
		t.Fatalf("first version = %q", got)
	}
	if got := Version(two); got != "2.0.0" {
		t.Fatalf("second version = %q", got)
	}
	if got := Version(context.Background()); got != "dev" {
		t.Fatalf("version leaked between contexts: %q", got)
	}
}

func TestSkillLinksMatchCLIReleaseOrDevelopmentBranch(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  context.Context
		ref  string
	}{
		{name: "tagged release", ctx: WithVersion(context.Background(), "v0.6.0"), ref: "v0.6.0"},
		{name: "explicit development version", ctx: WithVersion(context.Background(), "dev"), ref: "main"},
		{name: "default build version", ctx: context.Background(), ref: "main"},
	} {
		out, err := skill(test.ctx, nil)
		if err != nil {
			t.Fatalf("skill(%s) error = %v", test.name, err)
		}
		text, ok := out["content"].(string)
		if !ok {
			t.Fatalf("skill(%s) content type = %T", test.name, out["content"])
		}
		for _, want := range []string{
			"binary installs the srcpack CLI, not this optional agent instruction file",
			"https://github.com/nexssp/srcpack/blob/" + test.ref + "/skills/srcpack/SKILL.md",
			"https://raw.githubusercontent.com/nexssp/srcpack/" + test.ref + "/skills/srcpack/SKILL.md",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("skill(%s) omitted %q: %s", test.name, want, text)
			}
		}
	}
}

func TestHealthInspectReturnsFactsForMissingConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := healthInspect(WithVersion(context.Background(), "1.2.3"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out["config_present"] != false || out["config_valid"] != false {
		t.Fatalf("config facts = present:%v valid:%v", out["config_present"], out["config_valid"])
	}
	if out["version"] != "1.2.3" {
		t.Fatalf("version = %v", out["version"])
	}
}

func TestHelpTextIncludesCurrentTreeDepth(t *testing.T) {
	text := HelpText()
	if !strings.Contains(text, "--tree-depth N       Folder tree depth (default: "+itoa(config.DefaultTreeDepth)+")") {
		t.Fatalf("help text omitted configured tree-depth default:\n%s", text)
	}
	if strings.Contains(text, "{{TREE_DEPTH}}") {
		t.Fatal("help text left the depth placeholder unresolved")
	}
}

func TestProfileInfoCanonicalizesAliases(t *testing.T) {
	for _, profile := range config.BuiltinProfiles {
		for _, name := range append([]string{profile.Name}, profile.Aliases...) {
			out, err := profileInfo(context.Background(), map[string]any{"name": name})
			if err != nil {
				t.Fatalf("profileInfo(%q) error = %v", name, err)
			}
			want := profile.Name + ": " + profile.Description + "\n"
			if got := out["content"]; got != want {
				t.Errorf("profileInfo(%q) = %q, want %q", name, got, want)
			}
		}
	}
	for _, test := range []struct{ name, want string }{
		{name: " Full ", want: "source"},
		{name: "SIGNATURES", want: "dense"},
		{name: "Tests", want: "with-tests"},
	} {
		out, err := profileInfo(context.Background(), map[string]any{"name": test.name})
		if err != nil {
			t.Fatalf("profileInfo(%q) error = %v", test.name, err)
		}
		got, ok := out["content"].(string)
		if !ok || !strings.HasPrefix(got, test.want+": ") {
			t.Errorf("profileInfo(%q) = %v, want canonical name %q", test.name, out["content"], test.want)
		}
	}
	unknown, err := profileInfo(context.Background(), map[string]any{"name": "wat"})
	if err != nil {
		t.Fatal(err)
	}
	if got := unknown["error"]; got != "unknown profile \"wat\"; run 'srcpack list' to see built-in profiles" {
		t.Fatalf("unknown profile error = %q", got)
	}
}

func TestProfilesListUsesBuiltinMetadata(t *testing.T) {
	out, err := profiles(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	text, ok := out["content"].(string)
	if !ok {
		t.Fatalf("profile list content = %T, want string", out["content"])
	}
	for _, profile := range config.BuiltinProfiles {
		if !strings.Contains(text, profile.Name) || !strings.Contains(text, profile.Description) {
			t.Errorf("profile list omitted metadata for %q: %q", profile.Name, text)
		}
	}
}

func TestCleanDryRunIsNonDestructiveAndConfinesSymlinks(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "a.md")
	untouched := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(generated, []byte("remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(untouched, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	externalFile := filepath.Join(external, "outside.xml")
	if err := os.WriteFile(externalFile, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}

	inspected, err := cleanInspect(context.Background(), map[string]any{"path": root, "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	if inspected["count"] != 1 {
		t.Fatalf("matched count = %v, want 1", inspected["count"])
	}
	report, err := cleanDryRun(context.Background(), inspected)
	if err != nil {
		t.Fatal(err)
	}
	if got := report["content"]; got != "[DRY-RUN] 1 generated file(s) would be removed\n" {
		t.Fatalf("dry-run output = %q", got)
	}
	for _, path := range []string{generated, untouched, externalFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("dry-run changed %s: %v", path, err)
		}
	}
}

func TestCleanRemoveOnlyDeletesMatchingFilesUnderRoot(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "nested", "a.tmp")
	unmatched := filepath.Join(root, "nested", "keep.go")
	if err := os.Mkdir(filepath.Dir(generated), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generated, []byte("remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unmatched, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := cleanRemove(context.Background(), map[string]any{"path": root})
	if err != nil {
		t.Fatal(err)
	}
	if got := result["content"]; got != "🧹 Cleaned 1 generated file(s)\n" {
		t.Fatalf("remove output = %q", got)
	}
	if _, err := os.Stat(generated); !os.IsNotExist(err) {
		t.Fatalf("generated file remains: %v", err)
	}
	if _, err := os.Stat(unmatched); err != nil {
		t.Fatalf("unmatched file was removed: %v", err)
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
