package terminal

import (
	"context"
	"fmt"
	"sync"

	"github.com/nexssp/flow/compiler"
	flowtemplate "github.com/nexssp/flow/extensions/template"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
)

// Summary renders supplied facts without terminal detection, clocks or writes.
// The caller owns those effects and decides where to send the result.
func Summary(ctx context.Context, input map[string]any) (string, error) {
	return render(ctx, "summary", input)
}

// Tree renders flattened tree facts. Traversal, filtering, aggregation and
// discovery are supplied by the host; the bundle does not read files.
func Tree(ctx context.Context, input map[string]any) (string, error) {
	return render(ctx, "tree", input)
}

func render(ctx context.Context, kind string, input map[string]any) (string, error) {
	cfg, err := formatterConfig()
	if err != nil {
		return "", err
	}
	ctx = compiler.WithSourceLoader(ctx, Loader())
	execution, err := runner.Execute(ctx, cfg, "@include \""+kind+".nflow\"\n"+kind+".pipeline.terminal_"+kind, "terminal-"+kind+".nflow", input)
	if err != nil {
		return "", err
	}
	out, ok := execution.Output.(map[string]any)
	if !ok {
		return "", fmt.Errorf("terminal.%s: expected object, got %T", kind, execution.Output)
	}
	content, ok := out["content"].(string)
	if !ok {
		return "", fmt.Errorf("terminal.%s: expected string content, got %T", kind, out["content"])
	}
	return content, nil
}

var formatterConfig = sync.OnceValues(func() (runner.Config, error) {
	return runner.BuildConfig(append(native.Bundles(), flowtemplate.Bundle(nil), Bundle(nil)))
})
