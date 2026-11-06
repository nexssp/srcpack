// Package updatecmd contains the modular NFlow implementation of srcpack update.
package updatecmd

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/config"

	// Register the CLI input actions referenced by the update NFlow source.
	_ "github.com/nexssp/srcpack/bundles/cliinput"
	// Register the pack actions referenced by the update NFlow source.
	_ "github.com/nexssp/srcpack/bundles/pack"
)

const ID = "srcpack_update"

// FS embeds the update command's NFlow entrypoint and input schema.
//
//go:embed *.nflow
var FS embed.FS

func init() { compiler.Register(ID, Bundle) }

// Bundle registers only update-specific actions and command fixtures.
func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{
		ID: ID,
		Libraries: []action.Library{{Name: ID, Actions: []action.AnyAction{
			action.New("updatecmd.targets", targets).Build(),
		}}},
		Fixtures: FS,
	}
}

// Loader exposes the embedded command sources for hosts that use a composite
// source loader instead of the compiler bundle fixture loader.
func Loader() compiler.SourceLoader { return updateSourceLoader{} }

type updateSourceLoader struct{}

func (updateSourceLoader) Load(_, target string) ([]byte, string, error) {
	name := filepath.Base(target)
	data, err := fs.ReadFile(FS, name)
	return data, name, err
}

// targets loads configuration names and filesystem context. Update's selector,
// fallback, output-name, and result-joining policies are projected in NFlow.
func targets(_ context.Context, in map[string]any) (map[string]any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("update: get working directory: %w", err)
	}
	cfg, err := config.Load(cwd, "")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(cfg.TargetsMap))
	for name := range cfg.TargetsMap {
		names = append(names, name)
	}

	out := maps.Clone(in)
	if out == nil {
		out = make(map[string]any)
	}
	out["target_names"] = names
	out["cwd"] = cwd
	out["output_prefix"] = filepath.Join(cwd, "_srcpack") + string(os.PathSeparator)
	out["check"], _ = in["check"].(bool)
	out["quiet"], _ = in["quiet"].(bool)
	out["profile"], _ = in["profile"].(string)
	out["profile_arg"], _ = in["profile_arg"].(string)
	return out, nil
}
