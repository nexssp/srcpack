package cliinput

import (
	"context"
	"errors"
	"io"
	"maps"
	"os"

	"gopkg.in/yaml.v3"
)

// Print is an exact text-output tap, not a formatter or command dispatcher.
// Flow decides what to print, where, and whether to continue or fail afterward.
func Print(_ context.Context, in map[string]any) (map[string]any, error) {
	content, ok := in["content"].(string)
	if !ok {
		return nil, errors.New("cli.print requires string content")
	}
	var out io.Writer = os.Stdout
	if stream, _ := in["stream"].(string); stream != "" && stream != "stdout" {
		if stream != "stderr" {
			return nil, errors.New("cli.print stream must be stdout or stderr")
		}
		out = os.Stderr
	}
	_, err := io.WriteString(out, content)
	return in, err
}

// YAML serializes only the explicitly selected value and preserves the envelope.
func YAML(_ context.Context, in map[string]any) (map[string]any, error) {
	value, ok := in["value"]
	if !ok {
		return nil, errors.New("cli.yaml requires value")
	}
	body, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(in)+1)
	maps.Copy(out, in)
	out["content"] = string(body)
	return out, nil
}

// Finish prevents the terminal transport from serializing an internal envelope.
func Finish(_ context.Context, _ any) (any, error) { return nil, nil }
