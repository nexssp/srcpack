package packcmd

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/cliinput"
	"github.com/nexssp/srcpack/bundles/config"
	remote "github.com/nexssp/srcpack/bundles/github_source"
	"github.com/nexssp/srcpack/bundles/operators"
)

const (
	ID               = "srcpack_pack"
	editorMarkdown   = "markdown"
	profileWithTests = "with-tests"
)

//go:embed *.nflow
var FS embed.FS

// Invocation values are request-scoped; no command state is held globally.
type (
	invocationKey struct{}
	invocation    struct {
		command      string
		explicitPack bool
	}
)

func WithInvocation(ctx context.Context, command string, explicitPack bool, _ string) context.Context {
	return context.WithValue(ctx, invocationKey{}, invocation{command: command, explicitPack: explicitPack})
}

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/pack", Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/pack/nexssflow", Bundle)
}

func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: ID, Libraries: []action.Library{{Name: ID, Actions: []action.AnyAction{
		action.New("packcmd.input_facts", inputFacts).Build(),
		action.New("packcmd.selector_facts", selectorFacts).Build(),
		action.New("packcmd.typed_input", typedInput).Build(),
		action.New("packcmd.build_request", buildRequest).Build(),
		action.New("packcmd.git_diff", gitDiff).Build(),
		action.New("packcmd.request_maps", requestMaps).Build(),
		action.New("packcmd.materialize", materializeInput).Build(),
		action.New("packcmd.view_request", viewRequestAction).Build(),
		action.New("packcmd.statistics", statisticsAction).Build(),
		action.New("packcmd.done", doneAction).Build(),
	}}}}
}

type cliOptions struct {
	ExplicitPack   bool
	Command        string
	CommandLine    string
	Plan           bool
	JSON           bool
	Profile        string
	Cwd            string
	ConfigPath     string
	Config         operators.PackConfig
	Overrides      config.ConfigOptions
	RawSelectors   []string
	SaveView       string
	Check          bool
	DirPrefix      bool
	CopyExplicit   bool
	Stdout         bool
	OutputExplicit bool
}
type selector struct {
	Targets  []string
	Profile  string
	Prompt   string
	Excludes []string
}

// ResolveInput accepts only typed fields produced by tcli (or a test adapter); it never interprets raw argv.
func ResolveInput(ctx context.Context, fields map[string]any) (map[string]any, error) {
	return resolveInput(ctx, fields)
}

func resolveInput(ctx context.Context, in map[string]any) (map[string]any, error) {
	return runNamedPolicy(ctx, "selectors.nflow", "pipeline.pack_resolve", in)
}

func typedInput(ctx context.Context, envelope map[string]any) (map[string]any, error) {
	envelope = policyObject(envelope)
	in, _ := envelope["input"].(map[string]any)
	selection, _ := envelope["selection"].(map[string]any)
	normalized, _ := envelope["normalized"].(map[string]any)
	overrides, _ := envelope["overrides"].(map[string]any)
	cli := cliOptions{Command: stringField(normalized, "command", "pack"), ExplicitPack: boolField(normalized, "explicit_pack"), Config: operators.PackConfig{Editor: editorMarkdown}}
	cli.CommandLine = cliinput.CommandLine(ctx)
	cli.Cwd = stringField(in, "cwd", "")
	cli.ConfigPath = stringField(in, "config_path", "")
	cli.SaveView = stringField(in, "save_view", "")
	cli.Check = boolField(in, "check")
	cli.Plan = boolField(in, "plan")
	cli.JSON = boolField(in, "json")
	cli.DirPrefix = boolField(in, "dir_prefix")
	// Direct mapping preserves tcli's typed arrays, including selector strings whose commas are protected upstream.
	cli.Config.Files = stringSliceField(in["files"])
	cli.Config.Ext = stringField(in, "ext", "")
	cli.Config.Output = stringField(normalized, "output", "")
	cli.Stdout = boolField(in, "stdout")
	cli.OutputExplicit = cliinput.HasFlag(ctx, "output", "o")
	if cli.Stdout && cli.OutputExplicit && cli.Config.Output != "" && cli.Config.Output != "-" {
		return nil, errors.New("--stdout conflicts with -o FILE; use either --stdout or -o - for stdout, or -o FILE to save")
	}
	cli.Config.Copy = boolField(in, "copy")
	cli.CopyExplicit = boolField(in, "copy_explicit")
	cli.Config.Quiet = boolField(in, "quiet")
	cli.Config.Color = stringField(in, "color", "auto")
	cli.Config.Mask = boolField(in, "mask")
	cli.Config.IncludeConfig = boolField(in, "include_config")
	cli.Config.Changed = boolField(in, "changed")
	cli.Config.Modified = boolField(in, "modified")
	cli.Config.Staged = boolField(in, "staged")
	cli.Config.DiffOnly = boolField(in, "diff_only")
	cli.Config.Since = stringField(in, "since", "")
	cli.Config.MaxLines = intField(in, "max_lines")
	cli.Config.MaxLinesHead = intField(in, "max_lines_head")
	cli.Config.MaxLinesTail = intField(in, "max_lines_tail")
	cli.Config.Exclude = stringField(in, "exclude", "")
	cli.Config.Prompt = stringField(in, "prompt", "")
	cli.Config.Editor = stringField(in, "editor", editorMarkdown)
	maskOverride, err := resolveMaskOverride(ctx, cli.Command, in)
	if err != nil {
		return nil, err
	}
	if maskOverride != nil {
		cli.Config.Mask = *maskOverride
		cli.Overrides.MaskSecrets = maskOverride
	}
	// Targets and positional selectors are combined below, preserving argv order.
	cli.Config.Tests = boolField(in, "tests")
	cli.Config.NoTests = boolField(in, "no_tests")
	cli.Config.NoGenerated = boolField(in, "no_generated")
	cli.Config.NoDeps = boolField(in, "no_deps")
	cli.Config.NoExamples = boolField(in, "no_examples")
	cli.Config.Comments = boolField(normalized, "comments")
	cli.Config.OnlySig = boolField(in, "only_sig")
	cli.Config.Dense = boolField(in, "dense")
	cli.Config.ExportedOnly = boolField(in, "exported_only")
	cli.Config.TrimIndent = boolField(in, "trim_indent")
	cli.Config.TrimBlankLines = boolField(in, "trim_blank_lines")
	cli.Config.TreeOnly = boolField(overrides, "tree_only")
	cli.Config.FullTree = boolField(in, "full_tree")
	cli.Config.Depth = intField(normalized, "depth")
	cli.Config.CostROI = boolField(in, "cost")
	cli.Config.NoHeader = boolField(in, "no_header")
	cli.Config.NoPrompt = boolField(in, "no_prompt")
	cli.Config.NoStart = boolField(in, "no_start")
	cli.Config.NoEnd = boolField(in, "no_end")
	cli.Profile = stringField(in, "profile", "")

	// Pointer overrides are set on key presence, not truthiness, so an explicit false defeats profile/config defaults.
	setBool := func(key string, dst **bool) {
		if v, ok := overrides[key].(bool); ok {
			*dst = new(v)
		}
	}
	setInt := func(key string, dst **int) {
		if v, ok := overrides[key]; ok && v != nil {
			n := toInt(v)
			*dst = &n
		}
	}
	setBool("dense", &cli.Overrides.Dense)
	setBool("only_sig", &cli.Overrides.RemoveBodyFunc)
	setBool("strip_comments", &cli.Overrides.StripComments)
	setBool("no_tests", &cli.Overrides.NoTests)
	setBool("no_generated", &cli.Overrides.NoGenerated)
	setBool("no_deps", &cli.Overrides.NoDeps)
	setBool("no_examples", &cli.Overrides.NoExamples)
	setBool("changed", &cli.Overrides.Changed)
	setBool("modified", &cli.Overrides.Modified)
	setBool("staged", &cli.Overrides.Staged)
	setBool("diff_only", &cli.Overrides.DiffOnly)
	setBool("tree_only", &cli.Overrides.TreeOnly)
	setBool("full_tree", &cli.Overrides.FullTree)
	setBool("tree_files", &cli.Overrides.TreeFiles)
	setBool("exported_only", &cli.Overrides.ExportedOnly)
	setBool("trim_indent", &cli.Overrides.TrimIndent)
	setBool("trim_blank_lines", &cli.Overrides.TrimBlankLines)
	setBool("no_header", &cli.Overrides.NoHeader)
	setBool("no_prompt", &cli.Overrides.NoPrompt)
	setBool("no_start", &cli.Overrides.NoStart)
	setBool("no_end", &cli.Overrides.NoEnd)
	setBool("cost", &cli.Overrides.CostROI)
	setInt("max_lines", &cli.Overrides.MaxLines)
	setInt("max_lines_head", &cli.Overrides.MaxLinesHead)
	setInt("max_lines_tail", &cli.Overrides.MaxLinesTail)
	setInt("tree_depth", &cli.Overrides.TreeDepth)
	if v, ok := in["since"].(string); ok {
		cli.Overrides.Since = &v
	}
	if v, ok := in["only_sig"].(bool); ok {
		cli.Config.OnlySigOverride = &v
	}
	if v, ok := in["dense"].(bool); ok {
		cli.Config.DenseOverride = &v
	}
	if v, ok := in["comments"].(bool); ok {
		cli.Config.CommentsOverride = &v
	}
	if v, ok := in["no_tests"].(bool); ok {
		cli.Config.NoTestsOverride = &v
	}

	cli.Profile = stringField(envelope, "profile_ref", "")
	selected, _ := selection["selectors"].([]any)
	for _, value := range selected {
		item, _ := value.(map[string]any)
		cli.RawSelectors = append(cli.RawSelectors, stringField(item, "raw", ""))
		cli.Config.Targets = append(cli.Config.Targets, stringSliceField(item["targets"])...)
	}
	cli.Config.TargetProfiles = stringMapField(envelope["target_profiles"])
	cli.Config.TargetPrompts = stringMapField(envelope["target_prompts"])
	cli.Config.TargetExcludes = stringMapField(envelope["target_excludes"])

	plan, err := prepareResolution(cli)
	if err != nil {
		return nil, err
	}
	requestedOutput := stringField(in, "_output_path", "")
	if plan.Facts == nil {
		plan.Facts = make(map[string]any)
	}
	plan.Facts["requested_output"] = requestedOutput
	return map[string]any{"host": &packRequestHost{CLI: cli, Started: time.Now(), Plan: plan}, "facts": plan.Facts, "requested_output": requestedOutput}, nil
}

// resolveMaskOverride merges the positive and negative mask flags by argv
// order. Explicit values are respected, including --no-mask=false.
func resolveMaskOverride(ctx context.Context, command string, input map[string]any) (*bool, error) {
	schemaName := "PackInput"
	if command == "tree" {
		schemaName = "TreeInput"
	}
	ordered := cliinput.OrderedFieldValues(ctx, schemaName, "mask", "no_mask")
	var result *bool
	for _, occurrence := range ordered {
		enabled, err := strconv.ParseBool(occurrence.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value %q: %w", occurrence.Field, occurrence.Value, err)
		}
		if occurrence.Field == "no_mask" {
			enabled = !enabled
		}
		result = &enabled
	}
	if result != nil {
		return result, nil
	}

	// Non-argv callers provide typed fields but no lexical provenance. Resolve
	// each explicitly supplied value; when both exist, the negative field wins.
	if enabled, ok := input["mask"].(bool); ok {
		result = &enabled
	}
	if disabled, ok := input["no_mask"].(bool); ok {
		enabled := !disabled
		result = &enabled
	}
	return result, nil
}

// packRequestHost carries typed objects through the policy projection. NFlow
// never needs to serialize or reinterpret configuration-store implementation types.
type packRequestHost struct {
	CLI     cliOptions
	Started time.Time
	Plan    resolutionPlan
}

func buildRequest(ctx context.Context, envelope map[string]any) (map[string]any, error) {
	host, ok := envelope["host"].(*packRequestHost)
	if !ok {
		return nil, errors.New("packcmd.build_request: missing typed host facts")
	}
	cli, started := host.CLI, host.Started
	policy := policyObject(envelope["policy"])
	policy["diff_content"] = stringField(envelope, "diff_content", "")
	run, err := finishResolution(ctx, cli, started, host.Plan, policy)
	if err != nil {
		return nil, err
	}
	packMap, err := jsonObject(run.Pack)
	if err != nil {
		return nil, err
	}
	statsMap, err := jsonObject(run.Stats)
	if err != nil {
		return nil, err
	}
	renderMap, err := jsonObject(run.Render)
	if err != nil {
		return nil, err
	}
	check := cli.Check
	outputPath := run.OutputPath
	if cli.Stdout {
		outputPath = "-"
	}
	io := map[string]any{"explicit_pack": cli.ExplicitPack, "check": check, "plan": cli.Plan, "json": cli.JSON, "copy": boolField(policy, "copy"), "preview": cli.Config.Preview, "explicit_copy": cli.Config.Copy && cli.CopyExplicit, "quiet": cli.Config.Quiet, "started_at": started.UnixNano(), "output_path": outputPath, "explicit_output": cli.Config.Output, "output_explicit": cli.OutputExplicit, "config_path": cli.ConfigPath, "stdout": cli.Stdout, "color": cli.Config.Color}
	return map[string]any{"pack": packMap, "stats": statsMap, "render": renderMap, "io": io, "save_view": cli.SaveView, "selectors": cli.RawSelectors, "command": cli.Command, "requested_output": stringField(envelope, "requested_output", "")}, nil
}

func statisticsAction(ctx context.Context, in map[string]any) (map[string]any, error) {
	values, ok := in["records"].([]any)
	if !ok {
		if m, yes := in["records"].([]map[string]any); yes {
			values = make([]any, len(m))
			for i := range m {
				values[i] = m[i]
			}
		} else {
			return nil, fmt.Errorf("packcmd.statistics: records must be array, got %T", in["records"])
		}
	}
	var cfg operators.StatsCollectConfig
	if err := decode(in["config"], &cfg); err != nil {
		return nil, err
	}
	seq := func(yield func(any, error) bool) {
		for _, v := range values {
			if !yield(v, nil) {
				return
			}
		}
	}
	out := make([]any, 0, 1)
	for v, err := range operators.StatsCollectWithContext(ctx, cfg)(iter.Seq2[any, error](seq)) {
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return map[string]any{"records": out}, nil
}

func viewRequestAction(_ context.Context, in map[string]any) (map[string]any, error) {
	selectors := stringSliceField(in["selectors"])
	if len(selectors) == 0 {
		return nil, errors.New("--save-view requires at least one selector")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, path := config.FindRoot(cwd)
	if path == "" {
		path = filepath.Join(root, config.ConfigFileName)
	}
	normalized := make([]string, 0, len(selectors))
	for _, raw := range selectors {
		sel := parseSelector(raw)
		for i, target := range sel.Targets {
			if remote.IsRemote(target) {
				continue
			}
			abs := target
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(cwd, abs)
			}
			rel, err := filepath.Rel(root, filepath.Clean(abs))
			if err != nil {
				return nil, err
			}
			sel.Targets[i] = filepath.ToSlash(rel)
		}
		normalized = append(normalized, rebuildSelector(sel, sel.Targets))
	}
	return map[string]any{"operation": "save", "path": path, "name": stringField(in, "save_view", ""), "value": normalized, "replace": true, "legacy": true}, nil
}
func doneAction(context.Context, map[string]any) (any, error) { return nil, nil }
func jsonObject(v any) (map[string]any, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	m := map[string]any{}
	e = json.Unmarshal(b, &m)
	return m, e
}

func decode(v, dst any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, dst)
}

func stringField(m map[string]any, k, def string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return def
}
func boolField(m map[string]any, k string) bool { v, _ := m[k].(bool); return v }
func intField(m map[string]any, k string) int   { return toInt(m[k]) }
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		if int64(int(n)) != n {
			return 0
		}
		return int(n)
	case float64:
		intLimit := math.Ldexp(1, strconv.IntSize-1)
		if math.IsNaN(n) || math.IsInf(n, 0) || n < -intLimit || n >= intLimit {
			return 0
		}
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil || int64(int(i)) != i {
			return 0
		}
		return int(i)
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0
		}
		return i
	}
	return 0
}

func stringSliceField(v any) []string {
	switch a := v.(type) {
	case []string:
		return append([]string(nil), a...)
	case []any:
		r := make([]string, 0, len(a))
		for _, x := range a {
			if s, ok := x.(string); ok {
				r = append(r, s)
			}
		}
		return r
	case string:
		if a != "" {
			return []string{a}
		}
	}
	return nil
}

func selectorTargetKey(target, cwd string) string {
	if remote.IsRemote(target) {
		return target
	}
	if path, start, _ := operators.ParseLineRange(target); start > 0 {
		target = path
	}
	if cwd != "" && !filepath.IsAbs(target) {
		if base, e := filepath.Abs(cwd); e == nil {
			target = filepath.Join(base, target)
		}
	}
	if abs, e := filepath.Abs(target); e == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(target)
}

func resolvePromptFile(value, cwd string) string {
	p := value
	if cwd != "" && !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	if b, e := os.ReadFile(p); e == nil {
		return string(b)
	}
	return value
}

func parseSelector(raw string) selector {
	r := selector{}
	v := strings.TrimSpace(raw)
	if at := strings.Index(v, "@"); at >= 0 {
		r.Prompt = strings.TrimSpace(v[at+1:])
		v = strings.TrimSpace(v[:at])
	}
	if h := strings.Index(v, "#"); h >= 0 {
		l, rr := strings.TrimSpace(v[:h]), strings.TrimSpace(v[h+1:])
		var value string
		if isProfile(canonicalProfile(l)) && !isProfile(canonicalProfile(rr)) {
			r.Profile, value = canonicalProfile(l), rr
		} else {
			r.Profile, value = canonicalProfile(rr), l
		}
		v = value
	}
	if m := strings.Index(v, "~"); m >= 0 {
		for x := range strings.SplitSeq(v[m+1:], ",") {
			if x = strings.TrimSpace(x); x != "" {
				r.Excludes = append(r.Excludes, x)
			}
		}
		v = v[:m]
	}
	provider := ""
	if remote.IsRemote(v) {
		provider = "github:"
	}
	for x := range strings.SplitSeq(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			if provider != "" && !remote.IsRemote(x) {
				x = provider + x
			}
			r.Targets = append(r.Targets, x)
		}
	}
	return r
}

func isBareProfile(v string) bool {
	return !strings.ContainsAny(v, ",#~@") && isProfile(canonicalProfile(v))
}

func canonicalProfile(name string) string {
	return strings.ToLower(config.CanonicalProfileName(name))
}

func isProfile(name string) bool {
	_, ok := config.FindBuiltinProfile(name)
	return ok
}

func builtinProfileNames() []any {
	names := make([]any, 0, len(config.BuiltinProfiles))
	for i := range config.BuiltinProfiles {
		names = append(names, config.BuiltinProfiles[i].Name)
	}
	return names
}

func resolveViewSelector(raw, root string) string {
	if root == "" {
		return raw
	}
	sel := parseSelector(raw)
	if len(sel.Targets) == 0 {
		return raw
	}
	for i, t := range sel.Targets {
		if !remote.IsRemote(t) && !filepath.IsAbs(t) {
			sel.Targets[i] = filepath.Join(root, t)
		}
	}
	return rebuildSelector(sel, sel.Targets)
}

func rebuildSelector(sel selector, targets []string) string {
	var b strings.Builder
	b.WriteString(strings.Join(targets, ","))
	if len(sel.Excludes) > 0 {
		b.WriteString("~")
		b.WriteString(strings.Join(sel.Excludes, ","))
	}
	if sel.Profile != "" {
		b.WriteString("#")
		b.WriteString(sel.Profile)
	}
	if sel.Prompt != "" {
		b.WriteString("@")
		b.WriteString(sel.Prompt)
	}
	return b.String()
}

// stringMapField bridges expression maps to typed configuration, preserving nil
// for an absent/empty optional map. It does not choose winning entries.
func stringMapField(value any) map[string]string {
	var out map[string]string
	switch values := normalizePolicyValue(value).(type) {
	case map[string]any:
		for k, v := range values {
			if text, ok := v.(string); ok {
				if out == nil {
					out = map[string]string{}
				}
				out[k] = text
			}
		}
	case map[string]string:
		for k, v := range values {
			if out == nil {
				out = map[string]string{}
			}
			out[k] = v
		}
	}
	return out
}

func requestMaps(_ context.Context, in map[string]any) (map[string]any, error) {
	return policyObject(in), nil
}

// gitDiff performs exactly the effect selected by the caller's NFlow branch.
func gitDiff(ctx context.Context, in map[string]any) (map[string]any, error) {
	host, ok := in["host"].(*packRequestHost)
	if !ok {
		return nil, errors.New("packcmd.git_diff: missing typed host facts")
	}
	if host.CLI.Plan {
		in["diff_content"] = ""
		return in, nil
	}
	policy := policyObject(in["policy"])
	opts, err := mapPolicyOptions(policy["options"])
	if err != nil {
		return nil, err
	}
	patch, err := operators.GitUnifiedDiff(ctx, host.Plan.WorkspaceRoot, operators.GitFilter{Since: opts.Since, Modified: opts.Modified || opts.Changed, Staged: opts.Staged})
	if err != nil {
		return nil, err
	}
	in["diff_content"] = patch
	return in, nil
}
