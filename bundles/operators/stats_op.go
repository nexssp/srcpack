package operators

import (
	"context"
	"fmt"
	"iter"
	"os"
	"strconv"
	"time"

	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/config"
	"github.com/nexssp/srcpack/bundles/terminal"
)

const (
	statsPackedBytesField = "packed_bytes"
	statsRawBytesField    = "raw_bytes"
)

type StatsCollectConfig struct {
	TreeOnly    bool             `json:"tree_only"  cli:"tree-only"`
	CommandLine string           `json:"command_line"`
	FullTree    bool             `json:"full_tree"  cli:"full-tree"`
	TreeFiles   bool             `json:"tree_files" cli:"tree-files"`
	Depth       string           `json:"depth"      cli:"depth"`
	Quiet       bool             `json:"quiet"      cli:"quiet"`
	Color       string           `json:"color"      cli:"color"`
	Profile     string           `json:"profile"    cli:"profile"`
	Targets     string           `json:"targets"    cli:"targets"`
	Prompt      string           `json:"prompt"     cli:"prompt"`
	Editor      string           `json:"editor"     cli:"editor"`
	Directory   string           `json:"directory"  cli:"directory"`
	Output      string           `json:"output"     cli:"output"`
	Clipboard   string           `json:"clipboard"  cli:"clipboard"`
	StartedAt   string           `json:"started_at" cli:"started-at"`
	ShowCost    bool             `json:"cost"       cli:"cost"`
	Pricing     []map[string]any `json:"pricing"`
}

func StatsCollect(cfg StatsCollectConfig) action.StreamOp[any, any] {
	return StatsCollectWithContext(context.Background(), cfg)
}

// StatsCollectWithContext observes cancellation for callers that have an action context.
//
//nolint:contextcheck // A nil parent has no inherited state; retain the nil-context fallback.
func StatsCollectWithContext(ctx context.Context, cfg StatsCollectConfig) action.StreamOp[any, any] {
	if ctx == nil {
		ctx = context.Background()
	}
	return func(up iter.Seq2[any, error]) iter.Seq2[any, error] {
		return func(yield func(any, error) bool) {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			startedAt := time.Now()
			if nanos, err := strconv.ParseInt(cfg.StartedAt, 10, 64); err == nil && nanos > 0 {
				startedAt = time.Unix(0, nanos)
			}

			records, err := collectStatsRecords(ctx, up)
			if err != nil {
				yield(nil, err)
				return
			}
			files, stats, envelope, folded, err := prepareStatsRecords(records)
			if err != nil {
				yield(nil, err)
				return
			}
			depth, atoiErr := strconv.Atoi(cfg.Depth)
			if atoiErr != nil {
				depth = 0
			}
			depth = config.ResolveTreeDepth(depth)
			color := terminal.ColorEnabled(cfg.Color, os.Stderr)

			var tree string
			if cfg.FullTree {
				tree, err = generateFullProjectTreeContext(ctx, cfg.Directory, files, depth, color, cfg.TreeFiles)
			} else {
				tree, err = generateContextTreeContext(ctx, files, depth, color, cfg.TreeFiles)
			}
			if err != nil {
				yield(nil, fmt.Errorf("stats.collect tree: %w", err))
				return
			}

			if !cfg.Quiet {
				summary, err := terminal.Summary(ctx, terminalSummaryInput(cfg, stats, time.Since(startedAt).Round(time.Millisecond).String(), color, tree))
				if err != nil {
					yield(nil, fmt.Errorf("stats.collect terminal summary: %w", err))
					return
				}
				fmt.Fprint(os.Stderr, summary)
			}

			if cfg.TreeOnly {
				treeFile := ParsedFile{
					Path: "tree.txt", RelPath: "tree.txt", Lang: "text",
					Content: []byte(tree),
				}
				if folded {
					envelope.Files = []ParsedFile{treeFile}
					// The legacy renderer sees only the generated tree file in this
					// mode and computes its placeholders from that output stream.
					envelope.Stats = AggregatePackStats(envelope.Files)
					yield(envelope, nil)
				} else {
					yield(treeFile, nil)
				}
				return
			}

			if folded {
				yield(envelope, nil)
				return
			}
			for i := range files {
				if !yield(files[i], nil) {
					return
				}
			}
		}
	}
}

func collectStatsRecords(ctx context.Context, up iter.Seq2[any, error]) ([]any, error) {
	var records []any
	for record, err := range up {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func prepareStatsRecords(records []any) ([]ParsedFile, PackStats, packStatsEnvelope, bool, error) {
	if len(records) > 0 && isPackStatsEnvelope(records[0]) {
		if len(records) != 1 {
			return nil, PackStats{}, packStatsEnvelope{}, false, fmt.Errorf("stats.collect expected one NFlow pack envelope, got %d items", len(records))
		}
		envelope, err := unpackPackStatsEnvelope(records[0])
		if err != nil {
			return nil, PackStats{}, packStatsEnvelope{}, false, err
		}
		return envelope.Files, envelope.Stats, envelope, true, nil
	}
	files, err := parsedFilesFromRecords(records)
	if err != nil {
		return nil, PackStats{}, packStatsEnvelope{}, false, err
	}
	stats, err := foldPackStats(files)
	if err != nil {
		return nil, PackStats{}, packStatsEnvelope{}, false, err
	}
	return files, stats, packStatsEnvelope{}, false, nil
}

func StatsCollectOperator() action.NamedOperator {
	return action.NewOperator("stats.collect", StatsCollect)
}

// terminalSummaryInput exposes domain facts; defaults, labels, layout and color
// policy are owned by the independently copyable terminal NFlow bundle.
func terminalSummaryInput(cfg StatsCollectConfig, stats PackStats, elapsed string, color bool, tree string) map[string]any {
	clipboard, err := strconv.ParseBool(cfg.Clipboard)
	if err != nil {
		clipboard = false
	}
	return map[string]any{
		"config": map[string]any{
			"profile": cfg.Profile, "targets": cfg.Targets, "prompt": cfg.Prompt,
			"editor": cfg.Editor, "directory": cfg.Directory, "output": cfg.Output,
			"cost": cfg.ShowCost, "pricing": cfg.Pricing,
		},
		"stats": map[string]any{
			"file_count": stats.FileCount, "func_count": stats.FuncCount,
			statsRawBytesField: stats.RawBytes, statsPackedBytesField: stats.PackedBytes,
			"estimated_tokens": stats.EstimatedTokens, "saved_percent": stats.SavedPercent(),
		},
		"clipboard_requested": clipboard, "elapsed": elapsed, "color": color, "tree": tree,
		"command": cfg.CommandLine,
	}
}
