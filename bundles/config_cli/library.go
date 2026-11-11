// Package configcmd implements the srcpack config CLI command.
package configcmd

import (
	"context"
	"embed"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"

	"gopkg.in/yaml.v3"

	_ "github.com/nexssp/srcpack/bundles/cliinput" // Register the CLI input bundle used by this command's NFlow pipeline.
	"github.com/nexssp/srcpack/bundles/config"
)

const ID = "srcpack_config"

//go:embed *.nflow
var FS embed.FS

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/config_cli", Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/config_cli/nexssflow", Bundle)
}

func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{
		ID: ID,
		Libraries: []action.Library{{
			Name: ID,
			Actions: []action.AnyAction{
				action.New("configcmd.inspect", inspect).Build(),
				action.New("configcmd.path", printPath).Build(),
				action.New("configcmd.show", show).Build(),
				action.New("configcmd.validate", validate).Build(),
			},
		}},
	}
}

// Loader resolves the command's embedded Flow sources.
func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

func (sourceLoader) Load(_, target string) ([]byte, string, error) {
	name := filepath.Base(target)
	switch name {
	case "config.nflow", "schema.nflow":
		data, err := FS.ReadFile(name)
		return data, "srcpack_config/" + name, err
	default:
		return nil, "", fmt.Errorf("srcpack config: unknown source %q", target)
	}
}

// inspect returns filesystem facts; defaults and missing-config policy live in NFlow.
func inspect(_ context.Context, in map[string]any) (map[string]any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	_, path := config.FindRoot(cwd)
	out := clone(in)
	out["cwd"], out["path"] = cwd, path
	return out, nil
}

func printPath(_ context.Context, in map[string]any) (any, error) {
	text, _ := in["text"].(string)
	_, err := fmt.Fprintln(os.Stdout, text)
	return nil, err
}

func show(_ context.Context, _ map[string]any) (any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cwd, "")
	if err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	_, err = os.Stdout.Write(data)
	return nil, err
}

// validate reads and reports validation facts. Flow selects the failure message
// and decides whether a report is appropriate for a missing configuration.
func validate(_ context.Context, in map[string]any) (map[string]any, error) {
	cwd, _ := in["cwd"].(string)
	path, _ := in["path"].(string)
	report := config.Validate(cwd, path)
	fmt.Fprint(os.Stderr, report)
	out := clone(in)
	out["passed"] = report.Passed
	return out, nil
}

func clone(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+2)
	maps.Copy(out, in)
	return out
}
