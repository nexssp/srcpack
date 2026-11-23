package packcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nexssp/srcpack/bundles/cliinput"
	"github.com/nexssp/srcpack/bundles/config"
	remote "github.com/nexssp/srcpack/bundles/github_source"
)

// inputFacts performs filesystem checks and reports selector/profile metadata.
// Selector grammar and precedence decisions are implemented in selectors.nflow.
func inputFacts(ctx context.Context, in map[string]any) (map[string]any, error) {
	if boolField(in, "_derived_request") {
		ctx = cliinput.WithRawArguments(ctx, nil)
	}
	copyExplicit := cliinput.HasFlag(ctx, "copy", "c", "no-copy") || boolField(in, "copy") || boolField(in, "no_copy")
	in = cliinput.ResolveBoolAliases(ctx, in,
		cliinput.BoolAliases{Positive: "comments", Negative: "no_comments", PositiveFlags: []string{"comments"}, NegativeFlags: []string{"no-comments"}},
		cliinput.BoolAliases{Positive: "copy", Negative: "no_copy", PositiveFlags: []string{"copy", "c"}, NegativeFlags: []string{"no-copy"}},
		cliinput.BoolAliases{Positive: "examples", Negative: "no_examples", PositiveFlags: []string{"examples"}, NegativeFlags: []string{"no-examples"}},
	)
	boolOverrides := make(map[string]any, 3)
	for _, key := range []string{"comments", "copy", "examples"} {
		if value, ok := in[key].(bool); ok {
			boolOverrides[key] = value
		}
	}
	in["copy_explicit"] = copyExplicit
	if cwd := stringField(in, "cwd", ""); cwd != "" {
		absolute, err := filepath.Abs(cwd)
		if err != nil {
			return nil, err
		}
		if err := os.Chdir(absolute); err != nil {
			return nil, fmt.Errorf("change directory to %q: %w", absolute, err)
		}
		in["cwd"] = absolute
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, _ := config.FindRoot(cwd)
	cfg, err := config.Load(root, stringField(in, "config_path", ""))
	if err != nil {
		return nil, err
	}
	selectors := stringSliceField(in["selectors"])
	if len(selectors) == 0 {
		selectors = stringSliceField(in["positional"])
	}
	ordered := cliinput.OrderedFieldValues(ctx, "PackInput", targetsField, "selectors")
	if len(ordered) == 0 {
		for _, v := range stringSliceField(in[targetsField]) {
			ordered = append(ordered, cliinput.FieldValue{Field: targetsField, Value: v})
		}
		for _, v := range selectors {
			ordered = append(ordered, cliinput.FieldValue{Field: "selectors", Value: v})
		}
	}
	facts := make([]any, 0, len(ordered))
	for _, item := range ordered {
		_, statErr := os.Stat(item.Value)
		values, hasView := cfg.Views[item.Value]
		// Flow's projection runtime reserves the top-level "raw" key for its source body.
		view := make([]any, 0, len(values))
		for _, raw := range values {
			view = append(view, map[string]any{"selector_text": raw, "forced": false, "view_root": root})
		}
		facts = append(facts, map[string]any{"selector_text": item.Value, "forced": item.Field == targetsField, "path_exists": statErr == nil, "has_view": hasView, "view": view})
	}
	inv, _ := ctx.Value(invocationKey{}).(invocation)
	return map[string]any{
		"input": in, "cli_bool_flags": boolOverrides, "ordered": facts,
		profileField: stringField(in, profileField, ""), "profile_aliases": builtinProfileAliases(),
		"valid_profiles": builtinProfileNames(), "context_command": inv.command,
		"context_explicit": inv.explicitPack, "tree_output": filepath.Join("_srcpack", "tree.md"),
		"default_depth": config.DefaultTreeDepth,
	}, nil
}

// selectorFacts accepts selectors already parsed by selectors.nflow. Go owns
// only filesystem checks, local path normalization, and remote-source validation.
func selectorFacts(_ context.Context, in map[string]any) (map[string]any, error) {
	selected, _ := in["selected"].([]any)
	facts := make([]any, 0, len(selected))
	input, _ := in["input"].(map[string]any)
	cwd := stringField(input, "cwd", "")
	for _, value := range selected {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("selector fact has type %T", value)
		}
		raw := stringField(item, "raw", "")
		targets := stringSliceField(item[targetsField])
		if len(targets) > 0 && remote.IsRemote(targets[0]) {
			for i := 1; i < len(targets); i++ {
				if !remote.IsRemote(targets[i]) {
					targets[i] = "github:" + targets[i]
				}
			}
		}
		viewRoot := stringField(item, "view_root", "")
		for i, target := range targets {
			if viewRoot != "" && !remote.IsRemote(target) && !filepath.IsAbs(target) {
				targets[i] = filepath.Join(viewRoot, target)
			}
			if remote.IsRemote(targets[i]) {
				if _, err := remote.Parse(targets[i]); err != nil {
					return nil, err
				}
			}
		}
		_, statErr := os.Stat(raw)
		profile := stringField(item, profileField, "")
		prompt := stringField(item, promptField, "")
		if prompt != "" {
			prompt = resolvePromptFile(prompt, cwd)
		}
		excludes := stringSliceField(item["excludes"])
		bindings := make([]any, 0, len(targets))
		for _, target := range targets {
			key := selectorTargetKey(target, cwd)
			bindings = append(bindings, map[string]any{"key": key, profileField: profile, "has_prompt": stringField(item, promptField, "") != "", promptField: prompt, "excludes": excludes})
		}
		facts = append(facts, map[string]any{
			"bindings": bindings, targetsField: targets, "excludes": excludes, promptField: stringField(item, promptField, ""),
			"raw": raw, "forced": boolField(item, "forced"), "path_exists": statErr == nil,
			"bare_profile": boolField(item, "bare_profile"), "canonical": stringField(item, "canonical", ""),
			profileField: profile,
		})
	}
	in["candidates"] = facts
	return in, nil
}

func builtinProfileAliases() []any {
	aliases := make([]any, 0)
	for i := range config.BuiltinProfiles {
		profile := &config.BuiltinProfiles[i]
		canonical := strings.ToLower(strings.TrimSpace(profile.Name))
		aliases = append(aliases, []any{canonical, canonical})
		for _, alias := range profile.Aliases {
			aliases = append(aliases, []any{strings.ToLower(strings.TrimSpace(alias)), canonical})
		}
	}
	return aliases
}
