package operators

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/config"
)

func TestTreeDepthConsumers(t *testing.T) {
	root := t.TempDir()
	rel := "top/second/third/leaf.go"
	path := filepath.Join(root, filepath.FromSlash(rel))
	ktest.RequireNoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	ktest.RequireNoError(t, os.WriteFile(path, []byte("package sample\n"), 0o600))
	files := []ParsedFile{{Path: path, RelPath: rel, Lang: "go", Size: 15, Content: []byte("package sample\n")}}
	stats, err := foldPackStats(files)
	ktest.RequireNoError(t, err)

	for _, full := range []bool{false, true} {
		generator := func(depth int) string {
			if full {
				return GenerateFullProjectTree(root, files, depth, false, true)
			}
			return GenerateContextTree(files, depth, false, true)
		}
		for _, input := range []string{"", "0", "-1", "invalid", "999999999999999999999999999999", "1", "2", "3"} {
			t.Run(strconv.FormatBool(full)+"/"+input, func(t *testing.T) {
				depth, parseErr := strconv.Atoi(input)
				if parseErr != nil {
					depth = 0
				}
				depth = config.ResolveTreeDepth(depth)
				want := generator(depth)
				cfg := RenderConfig{Tree: !full, FullTree: full, TreeFiles: true, Directory: root, TreeDepth: input, Start: "${TREE}"}
				got, err := renderConfiguredTree(cfg, files)
				ktest.RequireNoError(t, err)
				ktest.RequireEqual(t, got, want)
				adapted, err := documentInput(context.Background(), cfg, files, stats)
				ktest.RequireNoError(t, err)
				tree, ok := adapted["tree"].(string)
				if !ok {
					t.Fatalf("adapted tree has type %T, want string", adapted["tree"])
				}
				ktest.RequireEqual(t, tree, want)
				var rendered strings.Builder
				for chunk, err := range RenderMarkdown(cfg)(anyParsedFiles(files)) {
					ktest.RequireNoError(t, err)
					rendered.Write(chunk.Content)
				}
				ktest.RequireCondition(t, strings.HasPrefix(rendered.String(), want), "legacy renderer must use the configured tree")
				count := 0
				for item, err := range StatsCollect(StatsCollectConfig{TreeOnly: true, FullTree: full, TreeFiles: true, Directory: root, Depth: input, Quiet: true})(anyParsedFiles(files)) {
					ktest.RequireNoError(t, err)
					file, ok := item.(ParsedFile)
					if !ok {
						t.Fatalf("stats tree output has type %T, want ParsedFile", item)
					}
					ktest.RequireEqual(t, string(file.Content), want)
					count++
				}
				ktest.RequireEqual(t, count, 1)
				ktest.RequireCondition(t, strings.Contains(want, "second/") == (depth >= 2), "second level visibility must follow depth")
				ktest.RequireCondition(t, strings.Contains(want, "third/") == (depth >= 3), "third level visibility must follow depth")
			})
		}
		for _, unset := range []int{0, -1} {
			ktest.RequireEqual(t, generator(unset), generator(config.DefaultTreeDepth))
		}
	}
}

func TestTreeAndStatsCollectionRespectCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := generateContextTreeContext(ctx, nil, 0, false, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("context tree error = %v, want context.Canceled", err)
	}
	if _, err := generateFullProjectTreeContext(ctx, t.TempDir(), nil, 0, false, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("full tree error = %v, want context.Canceled", err)
	}

	var got error
	for _, err := range StatsCollectWithContext(ctx, StatsCollectConfig{Quiet: true})(anyParsedFiles(nil)) {
		got = err
	}
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("stats error = %v, want context.Canceled", got)
	}
}
