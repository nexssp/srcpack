package main

// These test-only helpers remain as a parity safety net during the CLI migration; delete them after v1.0.0.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nexssp/srcpack/bundles/config"
	"github.com/nexssp/srcpack/bundles/operators"
)

type cliOptions struct {
	ExplicitPack bool // Raw entry syntax; NFlow chooses the corresponding output default.
	Command      string
	InfoProfile  string
	Profile      string
	Cwd          string
	ConfigPath   string
	Config       operators.PackConfig
	Overrides    config.ConfigOptions
	Args         []string
	RawSelectors []string
	SaveView     string
	Check        bool
	DirPrefix    bool
}

type selector struct {
	Targets  []string
	Profile  string
	Prompt   string
	Excludes []string
}

func parseCLI(args []string) (cliOptions, error) {
	cli := cliOptions{Command: "pack", Config: operators.PackConfig{
		Editor: editorMarkdown, Comments: true, Depth: config.DefaultTreeDepth, Color: "auto",
	}}
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "help", "--help", "-h":
			cli.Command = "help"
			args = args[1:]
		case cmdVersion, "--version", "-v":
			cli.Command = cmdVersion
			args = args[1:]
		case "list", "profiles":
			cli.Command = "list"
			args = args[1:]
		case "info":
			cli.Command = "info"
			if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
				cli.InfoProfile = args[1]
				args = args[2:]
			} else {
				args = args[1:]
			}
		case "tree":
			cli.Command = "tree"
			cli.Config.TreeOnly = true
			cli.Overrides.TreeOnly = new(true)
			cli.Config.Output = filepath.Join("_srcpack", "tree.md")
			args = args[1:]
		case cmdConfig, cmdClean, cmdDoctor, "validate", cmdTypes, "type-list", cmdHealth, cmdDemo, cmdInit:
			cli.Command = normalizeMaintenanceCommand(args[0])
			cli.Args = args[1:]
			args = nil
		case cmdTarget, cmdPrompt, cmdProfile, cmdEditor, cmdView:
			cli.Command = strings.ToLower(args[0])
			cli.Args = args[1:]
			args = nil
		case cmdUpdate, "up", "run-all":
			cli.Command = cmdUpdate
			cli.Args = args[1:]
			args = nil
		case "pack", "p":
			args = args[1:]
		}
	}

	var positional []string
	forcedTargets := make(map[int]bool)
	var (
		profileFlag     string
		promptFlag      string
		explicitDense   *bool
		explicitOnlySig *bool
		explicitComment *bool
		explicitTests   *bool
		explicitNoTests *bool
	)
	flagsEnded := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if flagsEnded {
			positional = append(positional, arg)
			continue
		}
		if arg == "--" {
			flagsEnded = true
			continue
		}
		if arg == "-h" || arg == "--help" {
			cli.Command = "help"
			continue
		}
		if arg == "-v" || arg == "--version" {
			cli.Command = "version"
			continue
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		name, value, hasValue := splitOption(arg)
		normalized := normalizeOptionName(name)
		if !hasValue && optionNeedsValue(normalized) {
			if i+1 >= len(args) || args[i+1] == "--" {
				return cli, missingOptionValueError(name, normalized)
			}
			i++
			value = args[i]
			hasValue = true
		}

		switch normalized {
		// ── selection ────────────────────────────────────────────────
		case "target", "t":
			forcedTargets[len(positional)] = true
			positional = append(positional, value)
		case "file", "f":
			cli.Config.Files = append(cli.Config.Files, value)
		case "profile", "p":
			profileFlag = value
		case "prompt":
			promptFlag = value
		case "ext":
			cli.Config.Ext = value
		case "exclude", "x":
			cli.Config.Exclude = appendCSV(cli.Config.Exclude, value)

		// ── identity / context ───────────────────────────────────────
		case "cwd", "C":
			cli.Cwd = value
		case "config":
			cli.ConfigPath = value

		// ── output ───────────────────────────────────────────────────
		case "output", "o":
			cli.Config.Output = value
		case "stdout":
			cli.Config.Output = "-"
		case cmdEditor, "e", "format":
			mode := strings.ToLower(value)
			switch mode {
			case "markdown", "raw", "none", "text", "zed", "claude", "fold":
				cli.Config.Editor = mode
			default:
				return cli, fmt.Errorf("%s expects one of: markdown, raw, none, text, zed, claude, fold", name)
			}
		case "color":
			mode := strings.ToLower(value)
			if mode != "auto" && mode != "always" && mode != "never" {
				return cli, fmt.Errorf("%s expects one of: auto, always, never", name)
			}
			cli.Config.Color = mode
		case "dir-prefix", "pfx":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.DirPrefix = b
		case "check", "diff", "dry-run":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Check = b

			// ── tree ─────────────────────────────────────────────────────
		case "depth", "l", "L":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return cli, fmt.Errorf("%s expects a positive integer", name)
			}
			cli.Config.Depth = n
			cli.Overrides.TreeDepth = &n
		case "tree", "tree-only":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.TreeOnly = b
			cli.Overrides.TreeOnly = &b
		case "full-tree":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.FullTree = &b
			if b {
				cli.Overrides.Tree = &b
			}
		case "tree-depth":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return cli, fmt.Errorf("%s expects a positive integer", name)
			}
			cli.Config.Depth = n
			cli.Overrides.TreeDepth = &n
		case "tree-files":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.TreeFiles = &b

		// ── transformation ───────────────────────────────────────────
		case "max-lines":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return cli, fmt.Errorf("%s expects a positive integer", name)
			}
			cli.Config.MaxLines = n
			cli.Overrides.MaxLines = &n
		case "max-lines-head":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return cli, fmt.Errorf("%s expects a positive integer", name)
			}
			cli.Config.MaxLinesHead = n
			cli.Overrides.MaxLinesHead = &n
		case "max-lines-tail":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return cli, fmt.Errorf("%s expects a positive integer", name)
			}
			cli.Config.MaxLinesTail = n
			cli.Overrides.MaxLinesTail = &n
		case profileDense:
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			explicitDense = &b
		case "only-sig", "signatures":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			explicitOnlySig = &b
		case "comments":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			explicitComment = &b
		case "no-comments":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			explicitComment = new(!b)
		case "exported-only":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.ExportedOnly = &b
		case "trim-blank-lines":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.TrimBlankLines = &b
		case "trim-indent":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.TrimIndent = &b
		case "mask", "mask-secrets":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.Mask = b
			cli.Overrides.MaskSecrets = &b

		// ── filtering ────────────────────────────────────────────────
		case "no-tests":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			explicitNoTests = &b
		case "tests":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			explicitTests = &b
		case "no-generated":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.NoGenerated = &b
		case "no-deps":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.NoDeps = &b
		case "no-examples":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.NoExamples = &b
		case "examples":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			inverted := !b
			cli.Overrides.NoExamples = &inverted

		// ── git ──────────────────────────────────────────────────────
		case "changed", "incremental":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.Changed = b
			cli.Overrides.Changed = &b
		case "modified", "m":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.Modified = b
			cli.Overrides.Modified = &b
		case "staged":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.Staged = b
			cli.Overrides.Staged = &b
		case "diff-only":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.DiffOnly = b
			cli.Overrides.DiffOnly = &b
		case "since":
			cli.Config.Since = value
			cli.Overrides.Since = &value

		// ── headers ──────────────────────────────────────────────────
		case "no-header":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.NoHeader = &b
		case "no-prompt":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.NoPrompt = &b
		case "no-start":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.NoStart = &b
		case "no-end":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.NoEnd = &b

		// ── copy / misc ──────────────────────────────────────────────
		case "copy", "c":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.Copy = b
		case "no-copy":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.Copy = !b
		case "quiet", "q":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Config.Quiet = b
		case "cost", "roi":
			b, err := optionBool(name, value, hasValue)
			if err != nil {
				return cli, err
			}
			cli.Overrides.CostROI = &b

		case "save-view", "as":
			if value == "" {
				return cli, fmt.Errorf("--%s requires a name", normalized)
			}
			cli.SaveView = value

		default:
			return cli, fmt.Errorf("unknown option %q; run 'srcpack --help'", name)
		}
	}

	var selectorProfile string
	for index, raw := range positional {
		if !forcedTargets[index] && isBareProfile(raw) {
			profilePath := raw
			if cli.Cwd != "" && !filepath.IsAbs(raw) {
				profilePath = filepath.Join(cli.Cwd, raw)
			}
			if _, err := os.Stat(profilePath); err != nil && selectorProfile == "" {
				selectorProfile = canonicalProfile(raw)
				continue
			}
		}
		cli.RawSelectors = append(cli.RawSelectors, raw)
		parsed := parseSelector(raw)
		cli.Config.Targets = append(cli.Config.Targets, parsed.Targets...)
		if parsed.Profile != "" {
			canonical := canonicalProfile(parsed.Profile)
			if selectorProfile == "" {
				selectorProfile = canonical
			}
			if cli.Config.TargetProfiles == nil {
				cli.Config.TargetProfiles = make(map[string]string)
			}
			for _, target := range parsed.Targets {
				cli.Config.TargetProfiles[selectorTargetKey(target, cli.Cwd)] = canonical
			}
		}
		for _, target := range parsed.Targets {
			key := selectorTargetKey(target, cli.Cwd)
			if len(parsed.Excludes) > 0 {
				if cli.Config.TargetExcludes == nil {
					cli.Config.TargetExcludes = make(map[string]string)
				}
				cli.Config.TargetExcludes[key] = appendCSV(cli.Config.TargetExcludes[key], strings.Join(parsed.Excludes, ","))
			}
			if parsed.Prompt != "" {
				if cli.Config.TargetPrompts == nil {
					cli.Config.TargetPrompts = make(map[string]string)
				}
				cli.Config.TargetPrompts[key] = resolvePromptFile(parsed.Prompt, cli.Cwd)
			}
		}
	}
	if profileFlag != "" {
		selectorProfile = canonicalProfile(profileFlag)
		cli.Config.TargetProfiles = nil
	}
	if promptFlag != "" {
		cli.Config.Prompt = resolvePromptFile(promptFlag, cli.Cwd)
	}
	if selectorProfile != "" {
		if !isProfile(selectorProfile) {
			return cli, fmt.Errorf("unknown profile %q; valid profiles: arch, api, impl, source, compact, dense, with-tests", selectorProfile)
		}
		applyProfile(&cli.Config, selectorProfile)
	}
	if explicitDense != nil {
		cli.Config.Dense = *explicitDense
		cli.Config.DenseOverride = explicitDense
		cli.Overrides.Dense = explicitDense
		if *explicitDense {
			cli.Config.OnlySig = true
			cli.Config.Comments = false
			if explicitOnlySig == nil {
				cli.Config.OnlySigOverride = new(true)
				cli.Overrides.RemoveBodyFunc = new(true)
			}
			if explicitComment == nil {
				cli.Config.CommentsOverride = new(false)
				cli.Overrides.StripComments = new(true)
			}
		}
	}
	if explicitOnlySig != nil {
		cli.Config.OnlySig = *explicitOnlySig
		cli.Config.OnlySigOverride = explicitOnlySig
		cli.Overrides.RemoveBodyFunc = explicitOnlySig
	}
	if explicitComment != nil {
		cli.Config.Comments = *explicitComment
		cli.Config.CommentsOverride = explicitComment
		strip := !*explicitComment
		cli.Overrides.StripComments = &strip
	}
	if explicitNoTests != nil {
		cli.Config.NoTests = *explicitNoTests
		cli.Config.NoTestsOverride = explicitNoTests
		cli.Overrides.NoTests = explicitNoTests
	}
	if explicitTests != nil {
		cli.Config.Tests = *explicitTests
		if *explicitTests {
			cli.Config.NoTests = false
			cli.Config.NoTestsOverride = new(false)
			cli.Overrides.NoTests = new(false)
		}
	}
	if cli.Config.Dense && explicitOnlySig == nil {
		cli.Config.OnlySig = true
	}
	if cli.Config.Output == "" && cli.Config.TreeOnly {
		cli.Config.Output = filepath.Join("_srcpack", "tree.md")
	}
	cli.Profile = selectorProfile
	return cli, nil
}

func normalizeMaintenanceCommand(name string) string {
	switch strings.ToLower(name) {
	case "validate":
		return "doctor"
	case "type-list":
		return "types"
	}
	return strings.ToLower(name)
}

// ── helpers ──────────────────────────────────────────────────────────

func splitOption(arg string) (name, value string, hasValue bool) {
	return strings.Cut(arg, "=")
}

func normalizeOptionName(name string) string {
	trimmed := strings.TrimLeft(name, "-")
	if strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "--") && len(trimmed) == 1 {
		return trimmed
	}
	return strings.ToLower(trimmed)
}

func optionNeedsValue(name string) bool {
	switch name {
	case "target", "t", "file", "f", "profile", "p", "prompt",
		"output", "o", "editor", "e", "format",
		"ext", "exclude", "x", "cwd", "C", "config",
		"depth", "l", "L", "max-lines", "max-lines-head", "max-lines-tail",
		"tree-depth", "since", "color", "save-view", "as":
		return true
	}
	return false
}

func missingOptionValueError(name, normalized string) error {
	switch normalized {
	case "editor", "e", "format":
		return fmt.Errorf("option %s requires a value (valid editor formats: markdown, raw, none, text, zed, claude, fold)", name)
	case "color":
		return fmt.Errorf("option %s requires a value (valid color modes: auto, always, never)", name)
	}
	return fmt.Errorf("option %s requires a value; run 'srcpack --help' for valid choices", name)
}

func optionBool(name, value string, hasValue bool) (bool, error) {
	if !hasValue {
		return true, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s expects true or false", name)
	}
	return b, nil
}

func appendCSV(current, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return current
	}
	if current == "" {
		return value
	}
	return current + "," + value
}

func selectorTargetKey(target, cwd string) string {
	if idx := strings.LastIndex(target, ":"); idx > 1 {
		parts := strings.Split(target[idx+1:], "-")
		validRange := len(parts) == 1 || len(parts) == 2
		for _, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 {
				validRange = false
				break
			}
		}
		if validRange {
			target = target[:idx]
		}
	}
	if cwd != "" && !filepath.IsAbs(target) {
		if base, err := filepath.Abs(cwd); err == nil {
			target = filepath.Join(base, target)
		}
	}
	if abs, err := filepath.Abs(target); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(target)
}

func resolvePromptFile(value, cwd string) string {
	path := value
	if cwd != "" && !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}
	return value
}

func parseSelector(raw string) selector {
	result := selector{}
	value := strings.TrimSpace(raw)
	if at := strings.Index(value, "@"); at >= 0 {
		result.Prompt = strings.TrimSpace(value[at+1:])
		value = strings.TrimSpace(value[:at])
	}
	if hash := strings.Index(value, "#"); hash >= 0 {
		left := strings.TrimSpace(value[:hash])
		right := strings.TrimSpace(value[hash+1:])
		if isProfile(canonicalProfile(left)) && !isProfile(canonicalProfile(right)) {
			result.Profile, value = canonicalProfile(left), right
		} else {
			result.Profile, value = canonicalProfile(right), left
		}
	}
	if mark := strings.Index(value, "~"); mark >= 0 {
		for exclusion := range strings.SplitSeq(value[mark+1:], ",") {
			if exclusion = strings.TrimSpace(exclusion); exclusion != "" {
				result.Excludes = append(result.Excludes, exclusion)
			}
		}
		value = value[:mark]
	}
	for target := range strings.SplitSeq(value, ",") {
		if target = strings.TrimSpace(target); target != "" {
			result.Targets = append(result.Targets, target)
		}
	}
	return result
}

func isBareProfile(value string) bool {
	return !strings.ContainsAny(value, ",#~@") && isProfile(canonicalProfile(value))
}

func canonicalProfile(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "architecture", "architecture-map", "signatures":
		return "arch"
	case "implementation", "full":
		return "impl"
	case "withtests", "tests":
		return profileWithTests
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func isProfile(name string) bool {
	switch canonicalProfile(name) {
	case "arch", "api", "impl", "source", "compact", "dense", "with-tests":
		return true
	}
	return false
}

func applyProfile(cfg *operators.PackConfig, profile string) {
	switch canonicalProfile(profile) {
	case "arch", "api", "dense":
		cfg.OnlySig = true
		cfg.Dense = true
		cfg.Comments = false
	case "compact":
		cfg.Dense = true
		cfg.Comments = false
	case "impl", "source":
		cfg.OnlySig = false
		cfg.Dense = false
		cfg.Comments = true
	case "with-tests":
		cfg.Tests = true
		cfg.NoTests = false
	}
}
