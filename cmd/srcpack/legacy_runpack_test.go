package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

func runPack(ctx context.Context, cli cliOptions) error {
	startedAt := time.Now()

	if cli.SaveView != "" {
		return saveView(ctx, cli.SaveView, cli.RawSelectors)
	}

	if cli.Cwd != "" {
		if chdirErr := os.Chdir(cli.Cwd); chdirErr != nil {
			return fmt.Errorf("change directory to %q: %w", cli.Cwd, chdirErr)
		}
	}

	run, err := resolve(ctx, cli, startedAt)
	if err != nil {
		return err
	}
	// Compatibility display labels only; the output candidate stays unchanged.
	if cli.Config.Copy && cli.Config.Output == "" {
		run.Stats.Output = "(clipboard only)"
		if cli.ExplicitPack && !cli.Check {
			run.Render.OutputFile = ""
		}
	} else if cli.ExplicitPack && !cli.Check && cli.Config.Output == "" && run.ConfigPath == "" {
		run.Stats.Output = "(not set)"
		run.Render.OutputFile = ""
	}
	_, err = executeRootPipeline(ctx, cli, run)
	return err
}
