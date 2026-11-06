package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nexssp/srcpack/bundles/config"
	"github.com/nexssp/srcpack/bundles/operators"
)

// resolved carries every value the pipeline needs, computed once from
// CLI input and nexss.srcpack.yml. No further overlay occurs after
// resolve returns.
type resolved struct {
	WorkspaceRoot string
	ConfigPath    string
	Config        config.Config
	Chain         []config.Profile
	Options       config.Options

	Pack   operators.PackConfig
	Stats  operators.StatsCollectConfig
	Render operators.RenderConfig

	PromptText string
	OutputPath string
}

// resolve loads nexss.srcpack.yml, resolves the profile chain, applies
// CLI overrides on top, and produces the operator configs.
//
// Precedence, lowest to highest:
//
//	embedded defaults → selected profile chain → CLI flags
func resolve(ctx context.Context, cli cliOptions, startedAt time.Time) (resolved, error) {
	var out resolved

	cwd, err := os.Getwd()
	if err != nil {
		return out, fmt.Errorf("resolve: getwd: %w", err)
	}
	projectRoot, configPath := config.FindRoot(cwd)
	out.WorkspaceRoot = projectRoot

	cfg, err := config.Load(projectRoot, cli.ConfigPath)
	if err != nil {
		return out, err
	}
	out.Config = cfg
	if cli.ConfigPath != "" {
		out.ConfigPath = cli.ConfigPath
	} else {
		out.ConfigPath = configPath
	}

	profileRef := cli.Profile
	if profileRef == "" {
		profileRef = "default"
	}
	chain, err := config.ResolveProfileChain(profileRef, cfg, projectRoot)
	if err != nil {
		return out, err
	}
	out.Chain = chain

	opts := cfg.Options.ToOptions()
	opts = config.ResolveOptions(opts, chain)
	opts.Apply(cli.Overrides.Mutator())
	opts.TreeDepth = config.ResolveTreeDepth(opts.TreeDepth)
	out.Options = opts

	paths, files, err := expandTargets(cli.Config.Targets, cfg)
	if err != nil {
		return out, err
	}
	if len(paths) == 0 && len(files) == 0 && len(cli.Config.Files) == 0 {
		paths = []string{"."}
	}
	files = append(files, cli.Config.Files...)

	out.PromptText = resolvePromptText(cli, cfg, chain)
	out.OutputPath = resolveOutputPath(cli, cfg, chain, projectRoot, cli.DirPrefix)

	displayOutput := out.OutputPath
	if cli.Config.Copy && cli.Config.Output == "" {
		displayOutput = "(clipboard only)"
	}

	diffContent := ""
	if opts.DiffOnly {
		filter := operators.GitFilter{
			Since:    opts.Since,
			Modified: opts.Modified || opts.Changed,
			Staged:   opts.Staged,
		}
		if filter.Active() {
			patch, diffErr := operators.GitUnifiedDiff(ctx, projectRoot, filter)
			if diffErr != nil {
				return out, diffErr
			}
			diffContent = patch
		}
	}

	profileName := chain[len(chain)-1].Name
	targetsLabel := strings.Join(cli.Config.Targets, ", ")
	if targetsLabel == "" {
		targetsLabel = "."
	}

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
		TreeOnly:  opts.TreeOnly,
		FullTree:  opts.FullTree,
		TreeFiles: opts.TreeFiles,
		Depth:     strconv.Itoa(depth),
		Quiet:     cli.Config.Quiet,
		Color:     cli.Config.Color,
		Profile:   profileName,
		Targets:   targetsLabel,
		Prompt:    out.PromptText,
		Editor:    cli.Config.Editor,
		Directory: projectRoot,
		Output:    out.OutputPath,
		Clipboard: strconv.FormatBool(cli.Config.Copy),
		StartedAt: strconv.FormatInt(startedAt.UnixNano(), 10),
		ShowCost:  opts.CostROI,
	}
	out.Stats.Output = displayOutput

	out.Render = operators.RenderConfig{
		Editor:      cli.Config.Editor,
		Prompt:      out.PromptText,
		Start:       profileSection(chain, cfg, func(p config.Profile) config.FlexibleText { return p.Start }),
		End:         profileSection(chain, cfg, func(p config.Profile) config.FlexibleText { return p.End }),
		FileStart:   profileFileStart(chain, cfg, cli.Config.Editor),
		FileEnd:     profileFileEnd(chain, cfg, cli.Config.Editor),
		ProfileName: profileName,
		Targets:     targetsLabel,
		Dirs:        strings.Join(paths, ", "),
		OutputFile:  displayOutput,
		PromptName:  resolvePromptName(cli, cfg, chain),
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

// resolvePromptText returns the fully-resolved prompt text using the
// same precedence rule as the profile chain: CLI override → chain
// prompt → global config prompt.
func resolvePromptText(cli cliOptions, cfg config.Config, chain []config.Profile) string {
	if cli.Config.Prompt != "" {
		return cli.Config.Prompt
	}
	for i := range slices.Backward(chain) {
		p := &chain[i]

		if !p.Prompt.Set || p.Prompt.Disabled || p.Prompt.Value == "" {
			continue
		}
		if text, ok := cfg.Prompts[p.Prompt.Value]; ok {
			return text
		}
		return p.Prompt.Value
	}
	if cfg.Prompt.Set && !cfg.Prompt.Disabled && cfg.Prompt.Value != "" {
		if text, ok := cfg.Prompts[cfg.Prompt.Value]; ok {
			return text
		}
		return cfg.Prompt.Value
	}
	return ""
}

func resolvePromptName(cli cliOptions, cfg config.Config, chain []config.Profile) string {
	if cli.Config.Prompt != "" {
		return "cli"
	}
	for i := range slices.Backward(chain) {
		p := &chain[i]
		if p.Prompt.Set && !p.Prompt.Disabled && p.Prompt.Value != "" {
			return p.Prompt.Value
		}
	}
	if cfg.Prompt.Set && !cfg.Prompt.Disabled && cfg.Prompt.Value != "" {
		return cfg.Prompt.Value
	}
	return "(none)"
}

// resolveOutputPath computes the output file path. Precedence:
// CLI (-o) → last profile's output → default _srcpack/context.md.
// When dirPrefix is set the filename gains the workspace directory
// name as a prefix (e.g. `srcpack_arch.md`).
func resolveOutputPath(cli cliOptions, cfg config.Config, chain []config.Profile, root string, dirPrefix bool) string {
	var chosen string
	switch {
	case cli.Config.Output != "":
		chosen = cli.Config.Output
	default:
		for i := range slices.Backward(chain) {
			if out := chain[i].GetOutputFile(); out != "" {
				chosen = out
				break
			}
		}
		if chosen == "" {
			chosen = cfg.GetOutputFile()
		}
		if chosen == "" {
			chosen = filepath.Join(root, "_srcpack", "context.md")
		}
	}

	if !dirPrefix || chosen == "-" {
		return chosen
	}

	dir := filepath.Dir(chosen)
	base := filepath.Base(chosen)
	prefix := filepath.Base(root)
	if prefix == "" || prefix == "." || prefix == string(filepath.Separator) {
		return chosen
	}
	return filepath.Join(dir, prefix+"_"+base)
}

// profileSection returns the first profile (latest to earliest in the
// chain) that set the section, falling back to the global config.
func profileSection(chain []config.Profile, cfg config.Config, pick func(config.Profile) config.FlexibleText) string {
	for i := range slices.Backward(chain) {
		section := pick(chain[i])
		if section.Set {
			if section.Disabled {
				return ""
			}
			return section.Value
		}
	}
	global := pick(cfg.Profile)
	if global.Set && !global.Disabled {
		return global.Value
	}
	return ""
}

func profileFileStart(chain []config.Profile, cfg config.Config, editorName string) string {
	for i := range slices.Backward(chain) {
		if v := chain[i].FileStart; v != "" {
			return v
		}
	}
	if editorName == "" {
		editorName = editorMarkdown
	}
	if ed, ok := cfg.Editors[editorName]; ok {
		return ed.FileStart
	}
	return ""
}

func profileFileEnd(chain []config.Profile, cfg config.Config, editorName string) string {
	for i := range slices.Backward(chain) {
		if v := chain[i].FileEnd; v != "" {
			return v
		}
	}
	if editorName == "" {
		editorName = editorMarkdown
	}
	if ed, ok := cfg.Editors[editorName]; ok {
		return ed.FileEnd
	}
	return ""
}
