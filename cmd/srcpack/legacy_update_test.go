package main

import (
	"context"
	"path/filepath"
	"time"
)

type updateResult struct {
	Target string
	File   string
	Status string
	Err    error
}

func runOneUpdate(
	ctx context.Context,
	cwd, targetName, profileName string,
	check, quiet bool,
	startedAt time.Time,
) updateResult {
	res := updateResult{Target: targetName, Status: "FAILED"}

	defaults, err := parseCLI(nil)
	if err != nil {
		res.Err = err
		return res
	}

	sub := defaults
	sub.Profile = profileName
	if targetName != "" {
		sub.Config.Targets = []string{targetName}
	}
	sub.Config.Quiet = quiet
	sub.Check = check

	run, err := resolve(ctx, sub, startedAt)
	if err != nil {
		res.Err = err
		return res
	}

	effectiveProfile := profileName
	if effectiveProfile == "" {
		effectiveProfile = "default"
	}
	if targetName != "" {
		run.OutputPath = filepath.Join(cwd, "_srcpack", targetName+"_"+effectiveProfile+".md")
	} else {
		run.OutputPath = filepath.Join(cwd, "_srcpack", effectiveProfile+".md")
	}
	run.Stats.Output = run.OutputPath
	run.Render.OutputFile = run.OutputPath

	if _, err := executeRootPipeline(ctx, sub, run); err != nil {
		res.Err = err
		return res
	}

	res.File = run.OutputPath
	if check {
		res.Status = "CHECKED"
	} else {
		res.Status = "WROTE"
	}
	return res
}
