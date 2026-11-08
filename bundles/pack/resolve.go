package packcmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nexssp/srcpack/bundles/config"
	remote "github.com/nexssp/srcpack/bundles/github_source"
	"github.com/nexssp/srcpack/bundles/operators"
)

// resolved carries every value the pipeline needs, computed once from
// CLI input and nexss.srcpack.yml. No further overlay occurs after
// resolve returns.
type resolved struct {
	Pack   operators.PackConfig
	Stats  operators.StatsCollectConfig
	Render operators.RenderConfig

	PromptText string
	OutputPath string
}

// finishResolution is a typed operator-config adapter. Precedence and stage
// choices have already been made by the calling NFlow pipeline.
func finishResolution(_ context.Context, cli cliOptions, startedAt time.Time, plan resolutionPlan, policy map[string]any) (resolved, error) {
	out := resolved{}
	projectRoot, chain := plan.WorkspaceRoot, plan.Chain
	paths, files := stringSliceField(policy["paths"]), stringSliceField(policy["files"])
	if len(files) == 0 && plan.FilesWereNil {
		files = nil
	}
	opts, err := mapPolicyOptions(policy["options"])
	if err != nil {
		return out, fmt.Errorf("resolve pack options: %w", err)
	}

	if err := validateRemoteSpecs(paths, files); err != nil {
		return out, err
	}

	out.PromptText = stringField(policy, "prompt", "")
	out.OutputPath = stringField(policy, "output", "")
	displayOutput := stringField(policy, "display_output", out.OutputPath)

	diffContent := stringField(policy, "diff_content", "")

	profileName := chain[len(chain)-1].Name
	targetsLabel := stringField(policy, "targets_label", ".")

	out.Pack = operators.PackConfig{
		Targets:        paths,
		Files:          files,
		TargetProfiles: cli.Config.TargetProfiles,
		TargetExcludes: cli.Config.TargetExcludes,
		TargetPrompts:  cli.Config.TargetPrompts,
		Ext:            cli.Config.Ext,
		NoTests:        opts.NoTests,
		NoGenerated:    opts.NoGenerated,
		NoDeps:         opts.NoDeps,
		NoExamples:     opts.NoExamples,
		Examples:       opts.Examples,
		Exclude:        cli.Config.Exclude,
		Changed:        opts.Changed,
		Since:          opts.Since,
		Modified:       opts.Modified,
		Staged:         opts.Staged,
		DiffOnly:       opts.DiffOnly,
		DiffContent:    diffContent,
		OnlySig:        opts.RemoveBodyFunc,
		Dense:          opts.Dense,
		Comments:       !opts.StripComments,
		Mask:           opts.MaskSecrets,
		IncludeConfig:  cli.Config.IncludeConfig,
		MaxLines:       opts.MaxLines,
		MaxLinesHead:   opts.MaxLinesHead,
		MaxLinesTail:   opts.MaxLinesTail,
		ExportedOnly:   opts.ExportedOnly,
		TrimIndent:     opts.TrimIndent,
		TrimBlankLines: opts.TrimBlankLines,
		MaxFileSize:    cli.Config.MaxFileSize,
		MaxFilesScan:   cli.Config.MaxFilesScan,

		// CLI overrides the per-target profile. Without these, a
		// selector's `#source` silently wins over `--dense` and the
		// user's explicit global flag is discarded.
		DenseOverride:    cli.Config.DenseOverride,
		OnlySigOverride:  cli.Config.OnlySigOverride,
		CommentsOverride: cli.Config.CommentsOverride,
		NoTestsOverride:  cli.Config.NoTestsOverride,
	}

	depth := opts.TreeDepth

	out.Stats = operators.StatsCollectConfig{
		TreeOnly:    opts.TreeOnly,
		CommandLine: cli.CommandLine,
		FullTree:    opts.FullTree,
		TreeFiles:   opts.TreeFiles,
		Depth:       strconv.Itoa(depth),
		Quiet:       cli.Config.Quiet || cli.JSON,
		Color:       cli.Config.Color,
		Profile:     profileName,
		Targets:     targetsLabel,
		Prompt:      out.PromptText,
		Editor:      cli.Config.Editor,
		Directory:   projectRoot,
		Output:      out.OutputPath,
		Clipboard:   strconv.FormatBool(boolField(policy, "copy")),
		StartedAt:   strconv.FormatInt(startedAt.UnixNano(), 10),
		ShowCost:    opts.CostROI,
	}
	out.Stats.Output = displayOutput

	out.Render = operators.RenderConfig{
		Editor:      cli.Config.Editor,
		Prompt:      out.PromptText,
		Start:       stringField(policy, "start", ""),
		End:         stringField(policy, "end", ""),
		FileStart:   stringField(policy, "file_start", ""),
		FileEnd:     stringField(policy, "file_end", ""),
		ProfileName: profileName,
		Targets:     targetsLabel,
		Dirs:        strings.Join(paths, ", "),
		OutputFile:  displayOutput,
		PromptName:  stringField(policy, "prompt_name", "(none)"),
		CurrentDate: startedAt.Format(time.RFC3339),
		Directory:   projectRoot,
		DiffContent: diffContent,
		NoHeader:    opts.NoHeader,
		NoPrompt:    opts.NoPrompt,
		NoStart:     opts.NoStart,
		NoEnd:       opts.NoEnd,
		Tree:        opts.Tree,
		FullTree:    opts.FullTree,
		TreeFiles:   opts.TreeFiles,
		TreeDepth:   strconv.Itoa(depth),
	}

	return out, nil
}

// expandTargets walks the target list, expanding any name that matches
// cfg.TargetsMap into its dirs and files (recursively through
// sub-targets). Names that do not resolve to a target are kept as
// literal paths so the source operator can report a clear error.
func expandTargets(names []string, cfg config.Config) (paths, files []string, err error) {
	visited := make(map[string]bool, len(names))

	var walk func(name string) error
	walk = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true

		if remote.IsRemote(name) {
			if _, err := remote.Parse(name); err != nil {
				return err
			}
			paths = append(paths, name)
			return nil
		}
		if tg, ok := cfg.TargetsMap[name]; ok {
			paths = append(paths, tg.Paths...)
			for _, entry := range tg.Dir {
				paths = append(paths, entry.Path)
			}
			files = append(files, tg.Files...)
			for _, sub := range tg.Targets {
				if err := walk(sub); err != nil {
					return err
				}
			}
			return nil
		}

		info, statErr := os.Stat(name)
		if statErr == nil {
			if info.IsDir() {
				paths = append(paths, name)
			} else {
				files = append(files, name)
			}
			return nil
		}
		paths = append(paths, name)
		return nil
	}

	for _, name := range names {
		if err := walk(name); err != nil {
			return nil, nil, err
		}
	}
	return paths, files, nil
}
