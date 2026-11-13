package usability

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	bytesPerEstimatedToken           = 3.5
	maxRenderedOverheadPercent int64 = 2
)

func TestUsabilityGoldensAndSizeComparison(t *testing.T) {
	root := repositoryRoot(t)
	binary := buildSrcpack(t, root)
	cases := []struct {
		name     string
		args     func(outputPath string) []string
		golden   string
		contains []string
		omits    []string
	}{
		{
			name: "focused Go implementation and TypeScript source",
			args: func(output string) []string {
				return []string{
					"pack",
					"tests/usability/testdata/project/go#impl",
					"tests/usability/testdata/project/web#source",
					"--exclude", "experimental.ts",
					"--no-header", "--quiet", "--output", output,
				}
			},
			golden: "focused-multilang.md",
			contains: []string{
				"## go/output_actions.go",
				"## web/client.ts",
				"func joinOutputAction",
				"export class AccountClient",
			},
			omits: []string{
				"output_actions_test.go",
				"client.test.ts",
				"experimentalRetryDelay",
				"NormalizeMessage",
			},
		},
		{
			name: "with-tests selectors include test files",
			args: func(output string) []string {
				return []string{
					"pack",
					"tests/usability/testdata/project/go#with-tests",
					"tests/usability/testdata/project/web#with-tests",
					"--no-header", "--quiet", "--output", output,
				}
			},
			golden: "with-tests-multilang.md",
			contains: []string{
				"## go/output_actions_test.go",
				"TestWriteOutputPreservesBytesWithoutFraming",
				"## web/client.test.ts",
				"accountClientRejectsFailedResponses",
				"experimentalRetryDelay",
			},
			omits: []string{"NormalizeMessage"},
		},
		{
			name: "selector exclusion pattern",
			args: func(output string) []string {
				return []string{
					"pack",
					"tests/usability/testdata/project/web~experimental.ts#source",
					"--no-header", "--quiet", "--output", output,
				}
			},
			golden:   "web-selector-exclusion.md",
			contains: []string{"## web/client.ts", "export class AccountClient"},
			omits:    []string{"experimental.ts", "client.test.ts", "experimentalRetryDelay"},
		},
	}

	outputs := make(map[string][]byte, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "context.md")
			stdout, stderr := runSrcpack(t, root, binary, tc.args(outputPath)...)
			if len(stdout) != 0 {
				t.Fatalf("saved-output invocation unexpectedly wrote %d bytes to stdout: %q", len(stdout), stdout)
			}
			got, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read saved output: %v; stderr: %s", err, stderr)
			}
			outputs[tc.name] = got
			goldenName := filepath.Base(tc.golden)
			if goldenName != tc.golden || !filepath.IsLocal(goldenName) || goldenName == "." {
				t.Fatalf("golden filename must be a local basename: %q", tc.golden)
			}
			goldenPath := filepath.Join(root, "tests/usability/testdata/golden", goldenName)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(goldenPath, got, 0o600); err != nil { // #nosec G703 -- goldenPath is a basename-validated fixture; got is file content, not a path.
					t.Fatalf("update golden %s: %v", tc.golden, err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v", tc.golden, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("saved context differs from %s (got %d bytes, want %d); run UPDATE_GOLDEN=1 go test ./tests/usability to review and refresh", tc.golden, len(got), len(want))
			}
			for _, text := range tc.contains {
				if !bytes.Contains(got, []byte(text)) {
					t.Errorf("saved context missing %q", text)
				}
			}
			for _, text := range tc.omits {
				if bytes.Contains(got, []byte(text)) {
					t.Errorf("saved context unexpectedly contains %q", text)
				}
			}
			if bytes.Contains(got, []byte("````##")) || bytes.Contains(got, []byte("```##")) {
				t.Fatal("adjacent file blocks merged a closing code fence with the next heading")
			}
		})
	}

	focused := outputs["focused Go implementation and TypeScript source"]
	if len(focused) == 0 {
		t.Fatal("focused golden output was not captured")
	}
	projectBytes, err := sourceBytes(filepath.Join(root, "tests/usability/testdata/project"))
	if err != nil {
		t.Fatalf("count fixture source bytes: %v", err)
	}
	selectedBytes := int64(fileSize(t, filepath.Join(root, "tests/usability/testdata/project/go/output_actions.go")) + fileSize(t, filepath.Join(root, "tests/usability/testdata/project/web/client.ts")))
	contextBytes := int64(len(focused))
	if contextBytes*100 > selectedBytes*(100+maxRenderedOverheadPercent) {
		t.Fatalf("rendered context framing overhead exceeds %d%%: %d rendered bytes vs %d selected source bytes (%+.1f%%)", maxRenderedOverheadPercent, contextBytes, selectedBytes, 100*float64(contextBytes-selectedBytes)/float64(selectedBytes))
	}
	if contextBytes >= projectBytes {
		t.Fatalf("selector did not reduce the full source corpus: rendered context %d bytes, all fixture source %d bytes", contextBytes, projectBytes)
	}
	if contextBytes*100 > projectBytes*80 {
		t.Fatalf("rendered context is not meaningfully smaller than the fixture: %d of %d source bytes", contextBytes, projectBytes)
	}
	t.Logf("whole fixture: %d bytes (~%d estimated tokens); rendered selection: %d bytes (~%d estimated tokens), %.1f%% of whole-source size", projectBytes, estimateTokens(projectBytes), contextBytes, estimateTokens(contextBytes), 100*float64(contextBytes)/float64(projectBytes))
	t.Logf("selected raw files: %d bytes (~%d estimated tokens); rendering overhead: %+d bytes (~%d estimated tokens), %.1f%%", selectedBytes, estimateTokens(selectedBytes), contextBytes-selectedBytes, estimateTokens(contextBytes-selectedBytes), 100*float64(contextBytes-selectedBytes)/float64(selectedBytes))
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate the usability test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s: %v", root, err)
	}
	return root
}

func buildSrcpack(t *testing.T, root string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "srcpack")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", binary, "./cmd/srcpack")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build srcpack CLI: %v\n%s", err, output)
	}
	return binary
}

func runSrcpack(t *testing.T, root, binary string, args ...string) ([]byte, []byte) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), binary, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("srcpack %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.Bytes(), stderr.Bytes()
}

func sourceBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".go", ".ts":
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func fileSize(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return int(info.Size())
}

func estimateTokens(byteCount int64) int64 {
	return int64(math.Floor(float64(byteCount) / bytesPerEstimatedToken))
}

func Example_tokenEstimateLimitations() {
	fmt.Println("srcpack's byte/3.5 estimate is a rough size proxy, not a tokenizer guarantee for any model")
	// Output: srcpack's byte/3.5 estimate is a rough size proxy, not a tokenizer guarantee for any model
}
