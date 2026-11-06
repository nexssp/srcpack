package main

// These adapters exist only for historical regression fixtures. None of the
// former administrative command implementations is linked into the executable.
import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nexssp/srcpack/bundles/config"
	systemcmd "github.com/nexssp/srcpack/bundles/system"
)

func runEntrypoint(ctx context.Context, args []string) (bool, error) {
	return runCommandEntrypoint(ctx, args)
}

func runView(args []string) error {
	return execute(context.Background(), append([]string{"view"}, args...))
}
func printHelp(out io.Writer) { _, _ = fmt.Fprint(out, systemcmd.HelpText()) }
func configPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	root, path := config.FindRoot(cwd)
	if path != "" {
		return path
	}
	return filepath.Join(root, config.ConfigFileName)
}
