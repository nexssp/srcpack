package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nexssp/srcpack/bundles/config"
)

// preprocessViews expands any positional argument that names a saved
// view into the view's selector strings.
//
// Paths stored in a view are project-root-relative; they are rewritten
// to absolute paths here so the rest of the pipeline (which resolves
// targets relative to cwd) receives unambiguous paths regardless of
// where the user ran srcpack.
//
// A path with the same name as a view wins — a real directory is never
// silently reinterpreted as a view. Flags and their values are left
// untouched.
func preprocessViews(args []string, cfg config.Config) []string {
	if len(cfg.Views) == 0 {
		return args
	}
	cwd, err := os.Getwd()
	if err != nil {
		return args
	}
	projectRoot, _ := config.FindRoot(cwd)

	out := make([]string, 0, len(args))
	flagsEnded := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if flagsEnded {
			out = append(out, expandViewArg(arg, cfg, projectRoot)...)
			continue
		}
		if arg == "--" {
			flagsEnded = true
			out = append(out, arg)
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			out = append(out, arg)
			name, _, hasValue := splitOption(arg)
			if !hasValue && optionNeedsValue(normalizeOptionName(name)) && i+1 < len(args) && args[i+1] != "--" {
				i++
				out = append(out, args[i])
			}
			continue
		}
		out = append(out, expandViewArg(arg, cfg, projectRoot)...)
	}
	return out
}

func expandViewArg(arg string, cfg config.Config, projectRoot string) []string {
	selectors, ok := cfg.Views[arg]
	if !ok {
		return []string{arg}
	}
	if _, err := os.Stat(arg); err == nil {
		return []string{arg}
	}
	resolved := make([]string, len(selectors))
	for i, raw := range selectors {
		resolved[i] = resolveViewSelector(raw, projectRoot)
	}
	return resolved
}

// resolveViewSelector rewrites every path slot in raw from
// project-root-relative to absolute, so downstream code (which
// resolves relative paths against cwd) sees the same file regardless
// of where the user ran srcpack.
func resolveViewSelector(raw, projectRoot string) string {
	if projectRoot == "" {
		return raw
	}
	sel := parseSelector(raw)
	if len(sel.Targets) == 0 {
		return raw
	}
	targets := make([]string, len(sel.Targets))
	for i, t := range sel.Targets {
		if filepath.IsAbs(t) {
			targets[i] = t
			continue
		}
		targets[i] = filepath.Join(projectRoot, t)
	}
	return rebuildSelector(sel, targets)
}

// saveView stores the current raw selectors in nexss.srcpack.yml with
// every path slot normalised to be relative to the project root. A
// view saved from a subdirectory therefore remains usable from the
// repository root and from any other subdirectory, and the paths
// travel with the repository instead of pinning to one machine.
func saveView(ctx context.Context, name string, selectors []string) error {
	if len(selectors) == 0 {
		return errors.New("--save-view requires at least one selector")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	projectRoot, _ := config.FindRoot(cwd)

	normalized := make([]string, 0, len(selectors))
	for _, raw := range selectors {
		n, normErr := normalizeSelector(raw, cwd, projectRoot)
		if normErr != nil {
			return fmt.Errorf("save-view: %w", normErr)
		}
		normalized = append(normalized, n)
	}

	_, err = legacyViewOperation(ctx, "save", name, normalized)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "✓ Saved view %q (%d selector(s))\n", name, len(normalized))
	return nil
}

// normalizeSelector rewrites every path slot in raw so it is relative
// to projectRoot. The profile, exclude, and prompt slots are preserved
// verbatim.
func normalizeSelector(raw, cwd, projectRoot string) (string, error) {
	if projectRoot == "" {
		return raw, nil
	}
	sel := parseSelector(raw)
	if len(sel.Targets) == 0 {
		return raw, nil
	}
	targets := make([]string, len(sel.Targets))
	for i, t := range sel.Targets {
		abs := t
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		rel, err := filepath.Rel(projectRoot, filepath.Clean(abs))
		if err != nil {
			return "", err
		}
		targets[i] = filepath.ToSlash(rel)
	}
	return rebuildSelector(sel, targets), nil
}

// rebuildSelector reassembles a selector string from a parsed selector
// and a new list of targets. Slots that were empty in the original stay
// empty. The order matches parseSelector: paths~excludes#profile@prompt.
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
