package packcmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/nexssp/flow/compiler"

	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
)

func runNamedPolicy(ctx context.Context, filename, entry string, payload map[string]any) (map[string]any, error) {
	ctx = compiler.WithSourceLoader(ctx, packPolicyLoader{})
	source, err := FS.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	bundles := append(native.Bundles(), Bundle(nil))
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		return nil, err
	}
	if entry != "" {
		source = append(source, []byte("\n"+entry+"\n")...)
	}
	ex, err := runner.Execute(ctx, cfg, string(source), filename, payload)
	if err != nil {
		return nil, err
	}
	out, ok := ex.Output.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("pack policy %s returned %T", filename, ex.Output)
	}
	return policyObject(out), nil
}

// normalizePolicyValue bridges expression maps (whose keys have type any) to
// JSON/typed Go objects. It performs no fallback, overlay or domain decision.
func normalizePolicyValue(value any) any {
	switch values := value.(type) {
	case map[any]any:
		out := make(map[string]any, len(values))
		for key, v := range values {
			if text, ok := key.(string); ok {
				out[text] = normalizePolicyValue(v)
			}
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(values))
		for key, v := range values {
			out[key] = normalizePolicyValue(v)
		}
		return out
	case []any:
		out := make([]any, len(values))
		for i, v := range values {
			out[i] = normalizePolicyValue(v)
		}
		return out
	default:
		return value
	}
}

func policyObject(value any) map[string]any {
	out, _ := normalizePolicyValue(value).(map[string]any)
	return out
}

// packPolicyLoader exposes only this command's explicitly named policy include.
type packPolicyLoader struct{}

func (packPolicyLoader) Load(_, target string) ([]byte, string, error) {
	if filepath.Base(target) != "resolution.nflow" {
		return nil, "", fmt.Errorf("unknown pack policy include %q", target)
	}
	data, err := FS.ReadFile("resolution.nflow")
	return data, "bundles/pack/resolution.nflow", err
}
