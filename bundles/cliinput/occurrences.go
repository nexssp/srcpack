package cliinput

import (
	"context"
	"maps"
	"slices"
	"strings"
)

type argumentOrderKey struct{}

// WithArguments preserves only lexical flag occurrence order. tcli still does
// all binding, type conversion and validation. This compatibility shim can be
// removed when tcli exposes occurrence provenance in its parse result.
// A flag-looking string cannot be a separate string flag value in tcli; such
// literal values use --flag=value. Positionals after -- are never inspected.
func WithArguments(ctx context.Context, args []string) context.Context {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue
		}
		name, _, _ := strings.Cut(arg, "=")
		names = append(names, strings.TrimLeft(name, "-"))
	}
	return context.WithValue(ctx, argumentOrderKey{}, names)
}

// HasFlag reports whether argv contains any of the named flags before `--`.
func HasFlag(ctx context.Context, aliases ...string) bool {
	names, _ := ctx.Value(argumentOrderKey{}).([]string)
	for _, name := range names {
		if slices.Contains(aliases, name) {
			return true
		}
	}
	return false
}

// BoolAliases declares inverse spellings for one already-bound boolean value.
// These are application metadata, not built-in meanings of a `no-` prefix.
type BoolAliases struct {
	Positive, Negative           string
	PositiveFlags, NegativeFlags []string
}

// ResolveBoolAliases returns a copy containing one positive field per pair.
// It interprets only typed bool values supplied by tcli, never raw flag values.
func ResolveBoolAliases(ctx context.Context, in map[string]any, pairs ...BoolAliases) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	names, _ := ctx.Value(argumentOrderKey{}).([]string)
	for _, pair := range pairs {
		positive, posOK := in[pair.Positive].(bool)
		negative, negOK := in[pair.Negative].(bool)
		if !posOK && !negOK {
			continue
		}
		// For direct typed-map callers without argv provenance, the negative field
		// takes precedence if both are supplied. Ordinary calls should supply one.
		chooseNegative := negOK
		for _, name := range names {
			for _, alias := range pair.PositiveFlags {
				if name == alias {
					chooseNegative = false
				}
			}
			for _, alias := range pair.NegativeFlags {
				if name == alias {
					chooseNegative = true
				}
			}
		}
		if chooseNegative && negOK {
			out[pair.Positive] = !negative
		} else if posOK {
			out[pair.Positive] = positive
		}
		delete(out, pair.Negative)
	}
	return out
}
