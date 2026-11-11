package config_views

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nexssp/kernel/xtest/ktest"
)

// This guards the bundle's copy-and-use contract: copied production files must
// compile independently of srcpack and be usable from an external consumer.
func TestCopiedBundleConsumer(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	ktest.RequireCondition(t, ok, "cannot locate test source")
	source := filepath.Dir(here)
	tmp := t.TempDir()
	ktest.RequireNoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/copied-config-views\n\ngo 1.27.3\n\nrequire (\n github.com/nexssp/kernel v0.36.0\n github.com/nexssp/flow v0.64.0\n gopkg.in/yaml.v3 v3.0.1\n)\n"), 0o600))
	for _, unit := range []struct{ source, destination string }{
		{source, "."},
		{filepath.Join(filepath.Dir(source), "crud"), "crud"},
	} {
		sourceRoot, err := os.OpenRoot(unit.source)
		ktest.RequireNoError(t, err)
		destinationRoot, err := os.OpenRoot(tmp)
		ktest.RequireNoError(t, err)
		ktest.RequireNoError(t, destinationRoot.MkdirAll(unit.destination, 0o755))
		ktest.RequireNoError(t, fs.WalkDir(sourceRoot.FS(), ".", func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := path.Ext(name)
			if ext != ".go" && ext != ".nflow" {
				return nil
			}
			if strings.HasSuffix(name, "_test.go") {
				return nil
			}
			dst := path.Join(unit.destination, name)
			if err := destinationRoot.MkdirAll(path.Dir(dst), 0o755); err != nil {
				return err
			}
			b, err := sourceRoot.ReadFile(name)
			if err != nil {
				return err
			}
			b = []byte(strings.ReplaceAll(string(b), "github.com/nexssp/srcpack/bundles/config_views", "example.com/copied-config-views"))
			b = []byte(strings.ReplaceAll(string(b), "github.com/nexssp/srcpack/bundles/crud", "example.com/copied-config-views/crud"))
			return destinationRoot.WriteFile(dst, b, 0o600)
		}))
		ktest.RequireNoError(t, sourceRoot.Close())
		ktest.RequireNoError(t, destinationRoot.Close())
	}
	consumer := `package config_views_test
import (
	 "context"
 "errors"
 "path/filepath"
 "testing"
 config "example.com/copied-config-views"
 "github.com/nexssp/kernel/xtest/ktest"
)
func TestConsumerOperations(t *testing.T) {
 p := filepath.Join(t.TempDir(), "settings.yml")
 ctx := context.Background()
 value := map[string]any{"nested":map[string]any{"items":[]any{"x", 7, true}}, "enabled":true}
 _, err := config.Execute(ctx, map[string]any{"operation":"save", "path":p, "name":"generic", "value":value, "app":"demo", "schema":"v1"})
 ktest.RequireNoError(t, err)
 _, err = config.Execute(ctx, map[string]any{"operation":"save", "path":p, "name":"legacy", "value":[]any{"old", "two"}, "legacy":true})
 ktest.RequireNoError(t, err)
 got, err := config.Execute(ctx, map[string]any{"operation":"get", "path":p, "name":"generic"})
 ktest.RequireNoError(t, err)
 ktest.RequireCondition(t, got["value"] != nil, "generic value missing")
 got, err = config.Execute(ctx, map[string]any{"operation":"get", "path":p, "name":"legacy", "legacy":true})
 ktest.RequireNoError(t, err)
 ktest.RequireCondition(t, got["value"] != nil, "legacy value missing")
 listing, err := config.Execute(ctx, map[string]any{"operation":"list", "path":p})
 ktest.RequireNoError(t, err)
 ktest.RequireCondition(t, len(listing["names"].([]any)) == 1, "generic listing should contain one entry: %#v", listing)
 listing, err = config.Execute(ctx, map[string]any{"operation":"list", "path":p, "legacy":true})
 ktest.RequireNoError(t, err)
 ktest.RequireCondition(t, len(listing["names"].([]any)) == 1, "legacy listing should contain one entry: %#v", listing)
 _, err = config.Execute(ctx, map[string]any{"operation":"save", "path":p, "name":"generic", "value":value, "app":"demo", "schema":"v1"})
 ktest.RequireCondition(t, errors.Is(err, config.ErrConflict), "expected conflict: %v", err)
 _, err = config.Execute(ctx, map[string]any{"operation":"save", "path":p, "name":"generic", "value":value, "app":"demo", "schema":"v1", "replace":true})
 ktest.RequireNoError(t, err)
 _, err = config.Execute(ctx, map[string]any{"operation":"delete", "path":p, "name":"generic"})
 ktest.RequireNoError(t, err)
 _, err = config.Execute(ctx, map[string]any{"operation":"delete", "path":p, "name":"legacy", "legacy":true})
 ktest.RequireNoError(t, err)
}`
	ktest.RequireNoError(t, os.WriteFile(filepath.Join(tmp, "consumer_test.go"), []byte(consumer), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	goBin, err := exec.LookPath("go")
	ktest.RequireNoError(t, err)
	cmd := exec.CommandContext(ctx, goBin, "test", "-mod=mod", "./...")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	output, err := cmd.CombinedOutput()
	ktest.RequireCondition(t, err == nil, "copied bundle go test failed: %v\n%s", err, output)
	deps := exec.CommandContext(ctx, goBin, "list", "-deps", "./...")
	deps.Dir, deps.Env = tmp, cmd.Env
	output, err = deps.CombinedOutput()
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, !strings.Contains(string(output), "github.com/nexssp/srcpack"), "copied bundle imports srcpack: %s", output)
}
