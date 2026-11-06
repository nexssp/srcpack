package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

// The binaries run sequentially and the default document header contains the
// current RFC3339 time. Normalize only that generated header timestamp, not
// arbitrary dates, source text, whitespace or other output bytes.
var stage5HeaderTime = regexp.MustCompile(`(<!-- Source Reference [^\r\n]*\]: )\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2})( -->)`)

func stage5StableDocument(data []byte) string {
	return stage5HeaderTime.ReplaceAllString(string(data), "${1}<CURRENT_DATE>${2}")
}

func TestStage5OracleNormalizesOnlyGeneratedHeaderTime(t *testing.T) {
	const source = "source date: 2026-10-18T21:23:19+02:00\n"
	a := []byte("<!-- Source Reference [Editor: zed]: 2026-10-18T21:23:19+02:00 -->\n" + source)
	b := []byte("<!-- Source Reference [Editor: zed]: 2026-10-18T21:23:20+02:00 -->\n" + source)
	ktest.RequireEqual(t, stage5StableDocument(a), stage5StableDocument(b))
	ktest.RequireEqual(t, stage5StableDocument([]byte(source)), source)
}

// TestStage5IndependentParity compares independently built Stage 4 and Stage 5
// executables. Set SRCPACK_STAGE4_BIN and SRCPACK_STAGE5_BIN to enable it.
func TestStage5IndependentParity(t *testing.T) {
	oldBin, newBin := os.Getenv("SRCPACK_STAGE4_BIN"), os.Getenv("SRCPACK_STAGE5_BIN")
	if oldBin == "" || newBin == "" {
		t.Skip("set SRCPACK_STAGE4_BIN and SRCPACK_STAGE5_BIN to run the independent parity oracle")
	}
	t.Setenv("NO_COLOR", "1")
	files := map[string]string{
		"main.go": "package main\n\n// Café is public.\nfunc Café() string { return \"café 東京\" }\n",
	}
	custom := "prompts:\n" +
		`  default: |-` + "\n" +
		`    Review the source carefully.` + "\n" +
		`    Preserve Unicode: café 東京.` + "\n" +
		`prompt: default` + "\n" +
		`start: |-` + "\n" +
		`  CUSTOM START ${PROFILE_NAME} ${FILE_COUNT} ${TREE}` + "\n" +
		`end: |-` + "\n" +
		`  CUSTOM END ${PROFILE_NAME} ${FUNC_COUNT}` + "\n" +
		`editor: markdown` + "\n" +
		`editors:` + "\n" +
		`  markdown:` + "\n" +
		`    file_start: |-` + "\n" +
		`      ## ${REL_PATH} (${LANG})` + "\n" +
		"      ```${LANG}\n" +
		`    file_end: |-` + "\n" +
		"      ```\n"
	cases := []struct {
		name, artifact string
		args           []string
		config         bool
	}{
		{"markdown", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "markdown"}, false},
		{"zed", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "zed"}, false},
		{"claude", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "claude"}, false},
		{"fold keeps document sections", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "fold"}, false},
		{"raw", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "raw"}, false},
		{"alias md", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "md"}, false},
		{"alias details", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "details"}, false},
		{"alias none", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "none"}, false},
		{"alias text", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "-e", "text"}, false},
		{"no header", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--no-header"}, false},
		{"no prompt", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--no-prompt"}, false},
		{"no start", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--no-start"}, false},
		{"no end", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--no-end"}, false},
		{"explicit stdout", "", []string{"pack", "-f", "main.go", "-o", "-"}, false},
		{"default pack output", "", []string{"pack", "-f", "main.go"}, false},
		{"p default output", "_srcpack/context.md", []string{"p", "-f", "main.go"}, false},
		{"check existing", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--check"}, false},
		{"check new", "", []string{"pack", "-f", "main.go", "-o", "out.md", "--check"}, false},
		{"tree", "out.md", []string{"tree", "-f", "main.go", "--tree-files", "-o", "out.md"}, false},
		{"custom templates", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--config", "config.yml"}, true},
		{"custom templates without header", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--config", "config.yml", "--no-header"}, true},
		{"multiline prompt", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--prompt", "First line\nSecond line café 東京"}, false},
		{"multiline prompt without prompt block", "out.md", []string{"pack", "-f", "main.go", "-o", "out.md", "--prompt", "line one\nline two", "--no-prompt"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range files {
				ktest.RequireNoError(t, os.WriteFile(filepath.Join(root, name), []byte(body), 0o600))
			}
			if tc.config {
				ktest.RequireNoError(t, os.WriteFile(filepath.Join(root, "config.yml"), []byte(custom), 0o600))
			}
			if tc.name == "check existing" {
				ktest.RequireNoError(t, os.WriteFile(filepath.Join(root, "out.md"), []byte("unchanged destination\n"), 0o600))
			}
			var wantStdout, wantArtifact []byte
			var wantFailure string
			for i, bin := range []string{oldBin, newBin} {
				// Each executable gets a fresh fixture: no artifact or hidden state
				// produced by its counterpart can influence source selection/output.
				if tc.artifact != "" && tc.name != "check existing" {
					_ = os.Remove(filepath.Join(root, tc.artifact))
				}
				for _, state := range []string{".nexss.srcpack.state", ".srcpack.state"} {
					_ = os.Remove(filepath.Join(root, state))
				}
				args := append([]string(nil), tc.args...)
				cmd := exec.CommandContext(t.Context(), bin, args...) // #nosec G702 -- direct argv; no shell interprets these arguments.
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "NO_COLOR=1")
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				stdout, err := cmd.Output()
				if tc.name == "alias md" || tc.name == "alias details" {
					ktest.RequireCondition(t, err != nil, "CLI must retain rejection of unsupported alias")
					if i == 0 {
						wantFailure = stderr.String()
					} else {
						ktest.RequireEqual(t, stderr.String(), wantFailure)
					}
					continue
				}
				if err != nil {
					t.Fatalf("binary %d (%s): %v; stderr: %s", i, bin, err, stderr.String())
				}
				var artifact []byte
				if tc.artifact != "" {
					artifact, err = os.ReadFile(filepath.Join(root, tc.artifact))
					ktest.RequireNoError(t, err)
				}
				if i == 0 {
					wantStdout, wantArtifact = stdout, artifact
				} else {
					ktest.RequireEqual(t, stage5StableDocument(stdout), stage5StableDocument(wantStdout))
					ktest.RequireEqual(t, stage5StableDocument(artifact), stage5StableDocument(wantArtifact))
				}
			}
		})
	}
}
