package configcmd

import (
	"os"
	"strings"
	"testing"
)

func TestConfigValidationSuccessUsesSharedFinish(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("nexss.srcpack.yml", []byte("options: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeConfigCLI(t, "config", "validate")
	if err != nil {
		t.Fatalf("config validate: %v; stderr=%q", err, stderr)
	}
	if stdout != "" || !strings.Contains(stderr, "All checks passed.") {
		t.Fatalf("unexpected validation output: stdout=%q stderr=%q", stdout, stderr)
	}
}
