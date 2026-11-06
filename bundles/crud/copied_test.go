package crud

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestCopiedBundleConsumer(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	ktest.RequireCondition(t, ok, "cannot locate test source")
	source := filepath.Dir(here)
	tmp := t.TempDir()
	root, err := os.OpenRoot(tmp)
	ktest.RequireNoError(t, err)
	defer root.Close()
	mod := `module example.com/copied-crud

go 1.27.3

require (
	 github.com/nexssp/kernel v0.36.0
	 github.com/nexssp/flow v0.64.0
	 gopkg.in/yaml.v3 v3.0.1
)
`
	ktest.RequireNoError(t, root.WriteFile("go.mod", []byte(mod), 0o600))
	// Run the complete nonrecursive CRUD test suite inside the independent module,
	// including memory storage, all four operations, replace and YAML failures.
	for _, name := range []string{"library.go", "crud.nflow", "library_test.go"} {
		b, err := os.ReadFile(filepath.Join(source, name))
		ktest.RequireNoError(t, err)
		ktest.RequireNoError(t, root.WriteFile(name, b, 0o600))
	}
	consumer := `package crud_test
import (
 "context"
 "path/filepath"
 "testing"
 crud "example.com/copied-crud"
 "github.com/nexssp/kernel/xtest/ktest"
)
func TestExternalConsumer(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"records.yml")
 value:=map[string]any{"ordinary":[]any{true,1,"data"}}
 _,err:=crud.Execute(context.Background(),map[string]any{"operation":"save","path":path,"name":"demo","value":value})
 ktest.RequireNoError(t,err)
 got,err:=crud.Execute(context.Background(),map[string]any{"operation":"get","path":path,"name":"demo"})
 ktest.RequireNoError(t,err)
 ktest.RequireEqual(t,got["value"],any(value))
}`
	ktest.RequireNoError(t, root.WriteFile("consumer_test.go", []byte(consumer), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	goBin, err := exec.LookPath("go")
	ktest.RequireNoError(t, err)
	cmd := exec.CommandContext(ctx, goBin, "test", "-mod=mod", "./...")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	ktest.RequireCondition(t, err == nil, "copied consumer tests: %v\n%s", err, out)
	deps := exec.CommandContext(ctx, goBin, "list", "-deps", "./...")
	deps.Dir, deps.Env = tmp, cmd.Env
	out, err = deps.CombinedOutput()
	ktest.RequireNoError(t, err)
	ktest.RequireCondition(t, !strings.Contains(string(out), "github.com/nexssp/srcpack"), "copied bundle imports srcpack: %s", out)
}
