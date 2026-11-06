package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

// This optional independent oracle compares actual Stage 3 and Stage 4 binaries,
// rather than two entry paths into the same implementation. No shell assertions.
func TestStage4IndependentParity(t *testing.T) {
	oldBin, newBin := os.Getenv("SRCPACK_STAGE3_BIN"), os.Getenv("SRCPACK_STAGE4_BIN")
	if oldBin == "" || newBin == "" {
		t.Skip("set SRCPACK_STAGE3_BIN and SRCPACK_STAGE4_BIN to run the independent migration oracle")
	}
	for _, tc := range []struct {
		name string
		args []string
		path string
	}{
		{"pack file", []string{"pack", "-f", "main.go", "-o", "out.md"}, "out.md"},
		{"alias file", []string{"p", "-f", "main.go", "-o", "out.md"}, "out.md"},
		{"implicit file", []string{"-f", "main.go", "-o", "out.md"}, "out.md"},
		{"raw", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "raw"}, "out.md"},
		{"fold", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "fold"}, "out.md"},
		{"zed", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "zed"}, "out.md"},
		{"tree", []string{"tree", "-f", "main.go", "--tree-files", "-o", "out.md"}, "out.md"},
		{"stdout", []string{"pack", "-f", "main.go", "-o", "-"}, ""},
		{"explicit pack default", []string{"pack", "-f", "main.go"}, ""},
		{"alias default", []string{"p", "-f", "main.go"}, "_srcpack/context.md"},
		{"check existing", []string{"pack", "-f", "main.go", "-o", "out.md", "--check"}, "out.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ktest.RequireNoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\n// Café is public.\nfunc Café() string { return \"café 東京\" }\n"), 0o600))
			if tc.name == "check existing" {
				ktest.RequireNoError(t, os.WriteFile(filepath.Join(root, "out.md"), []byte("unchanged destination\n"), 0o600))
			}
			var stdout, artifact []byte
			for i, bin := range []string{oldBin, newBin} {
				args := append(append([]string{}, tc.args...), "-q", "--color=never", "--no-header", "--no-start", "--no-end")
				cmd := exec.CommandContext(t.Context(), bin, args...) // #nosec G702 -- arguments are direct argv; no shell interprets them.
				cmd.Dir = root
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				output, err := cmd.Output()
				if err != nil {
					t.Fatalf("stage %d: %v; stderr: %s", i+3, err, stderr.String())
				}
				var data []byte
				if tc.path != "" {
					data, err = os.ReadFile(filepath.Join(root, tc.path))
					ktest.RequireNoError(t, err)
				}
				if i == 0 {
					stdout = output
					artifact = data
				} else {
					ktest.RequireEqual(t, string(output), string(stdout))
					ktest.RequireEqual(t, string(data), string(artifact))
				}
			}
		})
	}
}
