package cliinput

import "strings"

// NormalizeFlagNames preserves the old CLI spelling convention without
// interpreting flags or values. Single-letter short flags remain case-sensitive.
func NormalizeFlagNames(args []string) []string {
	out := append([]string(nil), args...)
	for i, arg := range out {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if strings.HasPrefix(name, "--") || len(strings.TrimLeft(name, "-")) > 1 {
			name = strings.ToLower(name)
		}
		if hasValue {
			name += "=" + value
		}
		out[i] = name
	}
	return out
}
