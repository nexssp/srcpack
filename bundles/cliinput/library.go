// Package cliinput preserves literal commas across tcli's CSV list binding.
// It does not parse flags: tcli remains the sole CLI parser and validator.
package cliinput

import (
	"context"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"
)

const ID = "srcpack_cli"

type (
	invocationKey struct{}
	invocation    struct {
		command      string
		explicitPack bool
		marker       string
	}
)

// ProtectCommas encodes literal commas before tcli binds list fields. A marker
// absent from every argument makes the transformation lossless, even when an
// argument itself contains the marker normally used by this adapter.
func ProtectCommas(args []string) ([]string, string) {
	stem := "\ue000srcpack"
	marker := stem + "\ue001"
	for {
		found := false
		for _, arg := range args {
			if strings.Contains(arg, marker) {
				found = true
				break
			}
		}
		if !found {
			break
		}
		stem += "x"
		marker = stem + "\ue001"
	}
	out := make([]string, len(args))
	encoder := strings.NewReplacer(",", marker+"c", " ", marker+"s", "\t", marker+"t", "\n", marker+"n", "\r", marker+"r")
	for i, arg := range args {
		out[i] = encoder.Replace(arg)
		if arg == "-" {
			out[i] = marker + "d"
		}
		if i > 0 && arg == "help" {
			// tcli reserves a naked help token anywhere in argv. Positional
			// values and prompt text must still be representable literally.
			out[i] = marker + "h"
		}
	}
	return out, marker
}

// WithInvocation carries syntax facts for one execution, never global state.
func WithInvocation(ctx context.Context, command string, explicitPack bool, marker string) context.Context {
	return context.WithValue(ctx, invocationKey{}, invocation{command, explicitPack, marker})
}

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/cliinput", Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/cliinput/nexssflow", Bundle)
}

func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: ID, WrapPipeline: WrapInputSchemas, Libraries: []action.Library{{Name: ID, Actions: []action.AnyAction{
		action.New("cli.restore", Restore).Build(),
		action.New("cli.print", Print).Build(),
		action.New("cli.yaml", YAML).Build(),
		action.New("cli.finish", Finish).Build(),
	}}}}
}

// Restore copies a parsed request, restoring original scalar/list strings.
// Only the host's syntax facts are added; domain and output decisions belong
// to the command pipelines.
func Restore(ctx context.Context, in map[string]any) (map[string]any, error) {
	inv, ok := ctx.Value(invocationKey{}).(invocation)
	out := make(map[string]any, len(in)+2)
	derived, _ := in["_derived_request"].(bool)
	for key, value := range in {
		if derived {
			// Requests produced inside a pipeline never passed through the CLI
			// encoder; decoding their literal data would corrupt it.
			out[key] = value
		} else {
			out[key] = restore(value, inv.marker)
		}
	}
	if ok && !derived {
		if _, present := out["_command"]; !present {
			out["_command"] = inv.command
		}
		if _, present := out["_explicit_pack"]; !present {
			out["_explicit_pack"] = inv.explicitPack
		}
	}
	return out, nil
}

func restore(value any, marker string) any {
	if marker == "" {
		return value
	}
	switch v := value.(type) {
	case string:
		return restoreString(v, marker)
	case []string:
		out := make([]string, len(v))
		for i, s := range v {
			out[i] = restoreString(s, marker)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = restore(item, marker)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = restore(item, marker)
		}
		return out
	default:
		return value
	}
}

func restoreString(value, marker string) string {
	if marker == "" {
		return value
	}
	return strings.NewReplacer(marker+"c", ",", marker+"s", " ", marker+"t", "\t", marker+"n", "\n", marker+"r", "\r", marker+"d", "-", marker+"h", "help").Replace(value)
}
