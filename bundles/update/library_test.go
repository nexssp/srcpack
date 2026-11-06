package updatecmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTargetsReturnsRawNamesAndFilesystemContext(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	config := "targets:\n  zeta:\n    paths: [z]\n  alpha:\n    paths: [a]\n"
	if err := os.WriteFile(filepath.Join(root, "nexss.srcpack.yml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"profile":     "arch",
		"profile_arg": "impl",
		"check":       true,
		"quiet":       true,
		"unrelated":   "preserved",
	}
	got, err := targets(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	names, ok := got["target_names"].([]string)
	if !ok || len(names) != 2 || !reflect.DeepEqual(map[string]bool{names[0]: true, names[1]: true}, map[string]bool{"alpha": true, "zeta": true}) {
		t.Fatalf("target names = %#v, want alpha and zeta", got["target_names"])
	}
	if got["cwd"] != root || got["output_prefix"] != filepath.Join(root, "_srcpack")+string(os.PathSeparator) {
		t.Fatalf("filesystem context = %#v", got)
	}
	if got["profile"] != "arch" || got["profile_arg"] != "impl" || got["check"] != true || got["quiet"] != true || got["unrelated"] != "preserved" {
		t.Fatalf("typed update options = %#v", got)
	}
}

func TestTargetsDefaultsMissingUpdateOptions(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	got, err := targets(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names, ok := got["target_names"].([]string)
	if !ok || len(names) != 0 {
		t.Fatalf("unexpected configured targets: %#v", got["target_names"])
	}
	if got["cwd"] != root || got["profile"] != "" || got["profile_arg"] != "" || got["check"] != false || got["quiet"] != false {
		t.Fatalf("fallback inputs = %#v", got)
	}
}
