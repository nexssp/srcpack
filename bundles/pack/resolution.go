package packcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nexssp/srcpack/bundles/config"
)

type resolutionPlan struct {
	WorkspaceRoot string
	Chain         []config.Profile
	Facts         map[string]any
	FilesWereNil  bool
}

// prepareResolution performs only host-bound discovery and projects its
// results into facts. The CLI path can pass Facts through pack_resolution_policy
// and then give the returned policy plus this plan to finishResolution.
func prepareResolution(cli cliOptions) (resolutionPlan, error) {
	var plan resolutionPlan
	cwd, err := os.Getwd()
	if err != nil {
		return plan, fmt.Errorf("resolve: getwd: %w", err)
	}
	root, _ := config.FindRoot(cwd)
	cfg, err := config.Load(root, cli.ConfigPath)
	if err != nil {
		return plan, err
	}
	chain, err := config.ResolveProfileChain(cli.Profile, cfg, root)
	if err != nil {
		return plan, err
	}
	paths, files, err := expandTargets(cli.Config.Targets, cfg)
	if err != nil {
		return plan, err
	}
	facts, err := resolutionFacts(cli, cfg, chain, root, cfg.Options.ToOptions(), paths, files)
	if err != nil {
		return plan, err
	}
	plan = resolutionPlan{WorkspaceRoot: root, Chain: chain, Facts: facts, FilesWereNil: files == nil && cli.Config.Files == nil}
	return plan, nil
}

// resolutionFacts projects filesystem/configuration results into JSON-safe
// facts, leaving all precedence and fallback choices to resolution.nflow.
func resolutionFacts(cli cliOptions, cfg config.Config, chain []config.Profile, root string, base config.Options, paths, files []string) (map[string]any, error) {
	baseMap, err := jsonObject(base)
	if err != nil {
		return nil, fmt.Errorf("pack resolution: encode base options: %w", err)
	}
	cliLayer, err := sparseOptionsMap(cli.Overrides)
	if err != nil {
		return nil, fmt.Errorf("pack resolution: encode CLI options: %w", err)
	}
	profiles := make([]any, 0, len(chain))
	for i := range chain {
		profile := &chain[i]
		layer, err := sparseOptionsMap(profile.Options)
		if err != nil {
			return nil, fmt.Errorf("pack resolution: encode profile %q options: %w", profile.Name, err)
		}
		output := profile.GetOutputFile()
		profiles = append(profiles, map[string]any{
			"options":         layer,
			"prompt":          profile.Prompt,
			"output_path":     output,
			"output_prefixed": prefixOutput(output, root, true),
			"start":           profile.Start,
			"end":             profile.End,
			"file_start":      profile.FileStart,
			"file_end":        profile.FileEnd,
		})
	}
	globalOutput := cfg.GetOutputFile()
	editor := cli.Config.Editor
	if editor == "" {
		editor = editorMarkdown
	}
	editorFormat := cfg.Editors[editor]

	return map[string]any{
		"base_options": baseMap, "cli_options": cliLayer, "profiles": profiles,
		"global_prompt": cfg.Prompt, "prompts": cfg.Prompts,
		"cli_prompt": cli.Config.Prompt,
		"cli_output": cli.Config.Output, "cli_output_prefixed": prefixOutput(cli.Config.Output, root, true),
		"global_output": globalOutput, "global_output_prefixed": prefixOutput(globalOutput, root, true),
		"global_start": cfg.Start, "global_end": cfg.End,
		"editor_file_start": editorFormat.FileStart, "editor_file_end": editorFormat.FileEnd,
		"dir_prefix": cli.DirPrefix, "default_tree_depth": config.DefaultTreeDepth,
		"paths": paths, "files": files, "cli_files": cli.Config.Files,
		"target_names": cli.Config.Targets, "copy": cli.Config.Copy, "copy_explicit": cli.CopyExplicit, "explicit_output": cli.Config.Output, "stdout": cli.Stdout,
		"has_remote": hasRemoteTargets(paths, files, cli.Config.Files),
	}, nil
}

func sparseOptionsMap(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, field := range fields {
		if field == nil {
			delete(fields, key)
		}
	}
	return fields, nil
}

func prefixOutput(path, root string, enabled bool) string {
	if path == "" || !enabled || path == "-" {
		return path
	}
	base := filepath.Base(root)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return path
	}
	return filepath.Join(filepath.Dir(path), base+"_"+filepath.Base(path))
}

func mapPolicyOptions(value any) (config.Options, error) {
	data, err := json.Marshal(normalizePolicyValue(value))
	if err != nil {
		return config.Options{}, err
	}
	var options config.Options
	if err := json.Unmarshal(data, &options); err != nil {
		return config.Options{}, err
	}
	return options, nil
}
