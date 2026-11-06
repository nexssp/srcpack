package systemcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nexssp/srcpack/bundles/config"
	remote "github.com/nexssp/srcpack/bundles/github_source"
)

const doctorErrorField = "error"

// doctorContext supplies OS path facts. NFlow chooses the missing-config result.
func doctorContext(_ context.Context, _ map[string]any) (map[string]any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	_, path := config.FindRoot(cwd)
	return map[string]any{"cwd": cwd, "path": path}, nil
}

// doctorFacts retains YAML decoding/merging and graph/locator parsing in their
// shared libraries. It does not stat target paths or choose audit findings.
func doctorFacts(_ context.Context, in map[string]any) (map[string]any, error) {
	cwd, _ := in["cwd"].(string)
	path, _ := in["path"].(string)
	out := clone(in)
	out["load_error"] = ""
	cfg, err := config.Load(cwd, path)
	if err != nil {
		out["load_error"] = err.Error()
		//nolint:nilerr // The doctor report consumes load_error as an audit finding.
		return out, nil
	}
	out["config_file"] = filepath.Base(path)
	base := filepath.Dir(path)
	targets := make(map[string]any, len(cfg.TargetsMap))
	for name := range cfg.TargetsMap {
		tg := cfg.TargetsMap[name]
		rawPaths := append([]string{}, tg.Paths...)
		for _, entry := range tg.Dir {
			rawPaths = append(rawPaths, entry.Path)
		}
		rawPaths = append(rawPaths, tg.Files...)
		paths := make([]any, 0, len(rawPaths))
		for _, raw := range rawPaths {
			abs := raw
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(base, abs)
			}
			isRemote, remoteError := remote.IsRemote(raw), ""
			if isRemote {
				if _, err := remote.Parse(raw); err != nil {
					remoteError = err.Error()
				}
			}
			paths = append(paths, map[string]any{"name": name, "source_text": raw, "path": abs, "slash_path": filepath.ToSlash(raw), "remote": isRemote, "remote_error": remoteError})
		}
		refs := make([]any, 0, len(tg.Targets))
		for _, sub := range tg.Targets {
			_, exists := cfg.TargetsMap[sub]
			refs = append(refs, map[string]any{"name": sub, "defined": exists})
		}
		targets[name] = map[string]any{"name": name, "paths": paths, "refs": refs, "cycle": config.TargetHasCycle(name, cfg.TargetsMap)}
	}
	profiles := make([]any, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		p := cfg.Profiles[name]
		_, exists := cfg.Prompts[p.Prompt.Value]
		profiles = append(profiles, map[string]any{"name": name, "prompt": p.Prompt.Value, "prompt_set": p.Prompt.Set, "prompt_disabled": p.Prompt.Disabled, "prompt_defined": exists, "prompt_quoted": fmt.Sprintf("%q", p.Prompt.Value)})
	}
	out["targets"], out["profiles"] = targets, profiles
	return out, nil
}

// doctorPathStat supplies only filesystem facts; NFlow decides how they affect the audit.
func doctorPathStat(ctx context.Context, in map[string]any) (map[string]any, error) {
	path, _ := in["path"].(string)
	if path == "" {
		return nil, errors.New("doctor path stat: path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{"path": path, "exists": false, "is_dir": false, "size": int64(0)}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "exists": true, "is_dir": info.IsDir(), "size": info.Size()}, nil
}

// doctorFormat only renders findings and status already selected by NFlow.
func doctorFormat(_ context.Context, in map[string]any) (map[string]any, error) {
	report := config.ValidationReport{}
	report.Passed, _ = in["passed"].(bool)
	report.ConfigFile, _ = in["path"].(string)
	rows, _ := in["issues"].([]any)
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("doctor: expected projected issue, got %T", raw)
		}
		severity, _ := row["severity"].(string)
		category, _ := row["category"].(string)
		message, _ := row["message"].(string)
		report.Issues = append(report.Issues, config.ValidationIssue{Severity: config.IssueSeverity(severity), Category: category, Message: message})
	}
	return map[string]any{"passed": report.Passed, doctorErrorField: in[doctorErrorField], "report": report.String()}, nil
}
