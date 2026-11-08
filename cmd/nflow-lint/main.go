// Temporary wrapper for Flow CLI limitations: fmt resolves @require and lint
// has no syntax-only mode. Remove it once Flow supports both operations directly.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nexssp/flow/cli"
	_ "github.com/nexssp/transport/tcli/nexssflow"

	// These registrations let Flow resolve srcpack's @require bundle IDs.
	_ "github.com/nexssp/srcpack/bundles/cliinput"
	_ "github.com/nexssp/srcpack/bundles/config_cli"
	_ "github.com/nexssp/srcpack/bundles/config_views"
	_ "github.com/nexssp/srcpack/bundles/crud"
	_ "github.com/nexssp/srcpack/bundles/document"
	_ "github.com/nexssp/srcpack/bundles/init"
	_ "github.com/nexssp/srcpack/bundles/manage"
	_ "github.com/nexssp/srcpack/bundles/pack"
	_ "github.com/nexssp/srcpack/bundles/system"
	_ "github.com/nexssp/srcpack/bundles/terminal"
	_ "github.com/nexssp/srcpack/bundles/update"
	_ "github.com/nexssp/srcpack/nexssflow"
)

const (
	commandLintProject = "lint-project"
	commandFmtCheck    = "fmt-check"
	commandFmtWrite    = "fmt-write"
)

// Flow lint has no syntax-only mode, so keep host-dependent files visible in warnings.
var lintNeedsHostContext = map[string]struct{}{
	"bundles/config_cli/config.nflow": {},
	"bundles/init/init.nflow":         {},
	"bundles/manage/manage.nflow":     {},
	"bundles/pack/pack.nflow":         {},
	"bundles/pack/plan_report.nflow":  {},
	"bundles/system/system.nflow":     {},
	"bundles/tree/tree.nflow":         {},
	"bundles/update/update.nflow":     {},
	"nexssflow/example.nflow":         {},
	"nflows/output.nflow":             {},
	"nflows/srcpack.nflow":            {},
}

// Flow fmt resolves @require; keep roots and fragments excluded. v0.68.0 adds
// trailing spaces to continued expressions in bundles/pack/resolution.nflow.
var fmtNeedsHostContext = map[string]struct{}{
	"bundles/pack/resolution.nflow":          {},
	"bundles/update/update.nflow":            {},
	"examples/flow-fold/report.nflow":        {},
	"nflows/srcpack.nflow":                   {},
	"tests/fixtures/nflows/serve_rest.nflow": {},
}

func main() {
	if len(os.Args) != 2 {
		usage()
	}

	command := os.Args[1]
	if command != commandLintProject && command != commandFmtCheck && command != commandFmtWrite {
		usage()
	}

	ctx := context.Background()
	root, err := findRepositoryRoot(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err = os.Chdir(root); err != nil {
		fmt.Fprintf(os.Stderr, "nflow: change to project root %q: %v\n", root, err)
		os.Exit(2)
	}

	files, skipped, err := projectNFlowFiles(ctx, command)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	warnSkipped(command, skipped)

	if command == commandLintProject {
		os.Exit(cli.Run(append([]string{"lint"}, files...)))
	}

	mode := "--check"
	if command == commandFmtWrite {
		mode = "--write"
	}
	var shared, fixtures []string
	for _, path := range files {
		if strings.HasPrefix(path, "tests/fixtures/nflows/") {
			fixtures = append(fixtures, path)
		} else {
			shared = append(shared, path)
		}
	}
	if len(shared) > 0 {
		if code := cli.Run(append([]string{"fmt", mode}, shared...)); code != 0 {
			os.Exit(code)
		}
	}
	for _, path := range fixtures {
		if code := cli.Run([]string{"fmt", mode, path}); code != 0 {
			os.Exit(code)
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: go run ./cmd/nflow-lint {%s|%s|%s}\n", commandLintProject, commandFmtCheck, commandFmtWrite)
	os.Exit(2)
}

func findRepositoryRoot(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("nflow: find project root: %w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", errors.New("nflow: find project root: git returned an empty path")
	}
	return filepath.Clean(root), nil
}

func projectNFlowFiles(ctx context.Context, command string) ([]string, []string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*.nflow")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("nflow: list project sources: %w", err)
	}

	var files, skipped []string
	for path := range strings.SplitSeq(string(output), "\x00") {
		if path == "" {
			continue
		}
		if _, err := os.Stat(filepath.FromSlash(path)); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, fmt.Errorf("nflow: stat source %q: %w", path, err)
		}

		production := strings.HasPrefix(path, "bundles/") || strings.HasPrefix(path, "nflows/") || strings.HasPrefix(path, "nexssflow/")
		if command == commandLintProject && !production {
			continue
		}
		omitted := lintNeedsHostContext
		if command != commandLintProject {
			omitted = fmtNeedsHostContext
		}
		if _, skip := omitted[path]; skip {
			skipped = append(skipped, path)
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	sort.Strings(skipped)
	if len(files) == 0 {
		return nil, nil, errors.New("nflow: no eligible .nflow files found")
	}
	return files, skipped, nil
}

func warnSkipped(command string, files []string) {
	if len(files) == 0 {
		return
	}
	coverage := "not format-checked"
	if command == commandLintProject {
		coverage = "not linted"
	}
	fmt.Fprintf(os.Stderr, "WARNING: %d NFlow files are %s by this gate:\n", len(files), coverage)
	for _, path := range files {
		fmt.Fprintf(os.Stderr, "  - %s\n", path)
	}
}
