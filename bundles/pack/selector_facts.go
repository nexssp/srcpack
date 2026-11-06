package packcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nexssp/srcpack/bundles/cliinput"
	"github.com/nexssp/srcpack/bundles/config"
	remote "github.com/nexssp/srcpack/bundles/github_source"
)

// inputFacts performs effects and reports syntax/provenance facts only. View
// expansion and path/profile precedence are chosen in selectors.nflow.
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
	ordered := cliinput.OrderedFieldValues(ctx, "PackInput", "targets", "selectors")
	if len(ordered) == 0 {
		for _, v := range stringSliceField(in["targets"]) {
			ordered = append(ordered, cliinput.FieldValue{Field: "targets", Value: v})
		}
		for _, v := range selectors {
			ordered = append(ordered, cliinput.FieldValue{Field: "selectors", Value: v})
		}
	}
	facts := make([]any, 0, len(ordered))
	for _, item := range ordered {
		_, statErr := os.Stat(item.Value)
		values, hasView := cfg.Views[item.Value]
		view := make([]any, 0, len(values))
		for _, raw := range values {
			view = append(view, map[string]any{"raw": resolveViewSelector(raw, root), "forced": false})
		}
		facts = append(facts, map[string]any{"raw": item.Value, "forced": item.Field == "targets", "path_exists": statErr == nil, "has_view": hasView, "view": view})
	}
	inv, _ := ctx.Value(invocationKey{}).(invocation)
	return map[string]any{"input": in, "cli_bool_flags": boolOverrides, "ordered": facts, "profile": canonicalProfile(stringField(in, "profile", "")), "context_command": inv.command, "context_explicit": inv.explicitPack, "tree_output": filepath.Join("_srcpack", "tree.md"), "default_depth": config.DefaultTreeDepth}, nil
}

// selectorFacts parses srcpack's grammar and reports filesystem facts. It does
// not choose whether a token is a profile, a literal path or a saved view.
func selectorFacts(_ context.Context, in map[string]any) (map[string]any, error) {
	selected, _ := in["selected"].([]any)
	facts := make([]any, 0, len(selected))
	for _, value := range selected {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("selector fact has type %T", value)
		}
		raw := stringField(item, "raw", "")
		parsed := parseSelector(raw)
		for _, target := range parsed.Targets {
			if remote.IsRemote(target) {
				if _, err := remote.Parse(target); err != nil {
					return nil, err
				}
			}
		}
		_, statErr := os.Stat(raw)
		bindings := make([]any, 0, len(parsed.Targets))
		input, _ := in["input"].(map[string]any)
		cwd := stringField(input, "cwd", "")
		prompt := parsed.Prompt
		if prompt != "" {
			prompt = resolvePromptFile(prompt, cwd)
		}
		for _, target := range parsed.Targets {
			bindings = append(bindings, map[string]any{"key": selectorTargetKey(target, cwd), "profile": parsed.Profile, "has_prompt": parsed.Prompt != "", "prompt": prompt, "excludes": parsed.Excludes})
		}
		facts = append(facts, map[string]any{"bindings": bindings, "targets": parsed.Targets, "raw": raw, "forced": boolField(item, "forced"), "path_exists": statErr == nil, "bare_profile": isBareProfile(raw), "canonical": canonicalProfile(raw), "profile": parsed.Profile})
	}
	in["candidates"] = facts
	return in, nil
}
