// Package config_views adapts generic CRUD records to configuration presets
// and the historical srcpack views layout. All policy lives in NFlow.
package config_views

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/crud"
)

const ID = "config_views"

var (
	ErrConflict      = crud.ErrConflict
	ErrNotFound      = crud.ErrNotFound
	ErrCompatibility = errors.New("config_views: app/schema mismatch")
)

//go:embed config_views.nflow
var FS embed.FS

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/config_views/nexssflow", Bundle)
}

// Bundle exposes adapter errors only; CRUD is selected by its own @require.
func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: ID, Libraries: []action.Library{{Name: ID, Actions: []action.AnyAction{
		action.New("config_views.reject", reject).Description("Raise an adapter policy error").Build(),
	}}}}
}

// FullBundle preserves manual Go mounting with the CRUD storage primitives.
func FullBundle(opts map[string]string) compiler.Bundle {
	bundle := Bundle(opts)
	bundle.Libraries = append(append([]action.Library(nil), crud.Bundle(nil).Libraries...), bundle.Libraries...)
	return bundle
}

func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

func (sourceLoader) Load(from, target string) ([]byte, string, error) {
	if filepath.Base(target) == "crud.nflow" {
		return crud.Loader().Load(from, target)
	}
	if filepath.Base(target) != "config_views.nflow" {
		return nil, "", fmt.Errorf("config_views: unknown source %q", target)
	}
	data, err := FS.ReadFile("config_views.nflow")
	return data, "config_views.nflow", err
}

var executionConfig = sync.OnceValues(func() (runner.Config, error) {
	return runner.BuildConfig(append(native.Bundles(), FullBundle(nil)))
})

// Execute invokes adapter NFlow policy, never applies or executes stored data.
// crud.WithStore can select another storage implementation through ctx.
//
//nolint:contextcheck // A nil context has no parent; retain the API's background fallback.
func Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := executionConfig()
	if err != nil {
		return nil, err
	}
	ctx = compiler.WithSourceLoader(ctx, Loader())
	ex, err := runner.Execute(ctx, cfg, "@include \"config_views.nflow\"\nconfig_views.pipeline.config_views", "config-views-entry.nflow", input)
	if err != nil {
		return nil, err
	}
	out, ok := ex.Output.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config_views: expected result object, got %T", ex.Output)
	}
	return out, nil
}

func reject(_ context.Context, input map[string]any) (map[string]any, error) {
	message, _ := input["message"].(string)
	if input["code"] == "compatibility" {
		return nil, fmt.Errorf("%w: %s", ErrCompatibility, message)
	}
	return nil, fmt.Errorf("config_views: %s", message)
}
