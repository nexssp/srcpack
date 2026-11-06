package cliinput

import (
	"context"
	"reflect"
	"testing"

	"github.com/nexssp/transport/tcli"
)

func TestLiteralHelpPayloadRoundTrip(t *testing.T) {
	args, marker := ProtectCommas([]string{"prompt", "add", "greeting", "help"})
	if args[3] == "help" {
		t.Fatal("transport would intercept literal prompt value")
	}
	schema := tcli.CLIRequestSchema{Name: "ManageInput", Fields: []tcli.CLIFieldSpec{{Name: "sub", Type: "string", Positional: true}, {Name: "args", Type: "[]string", Positional: true}}}
	parsed, err := tcli.ParseCLIRequest(args[1:], schema)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Restore(WithInvocation(context.Background(), "prompt", false, marker), parsed)
	if err != nil || !reflect.DeepEqual(out["args"], []string{"greeting", "help"}) {
		t.Fatalf("literal payload lost: %#v %v", out, err)
	}
}

func TestYAMLPreservesEnvelopeAndFinishSuppressesInternalData(t *testing.T) {
	input := map[string]any{"value": map[string]any{"status": "ok"}, "passed": true}
	out, err := YAML(context.Background(), input)
	if err != nil || out["content"] != "status: ok\n" || out["passed"] != true {
		t.Fatalf("bad YAML: %#v %v", out, err)
	}
	if _, exists := input["content"]; exists {
		t.Fatal("input mutated")
	}
	result, err := Finish(context.Background(), out)
	if err != nil || result != nil {
		t.Fatal("terminal envelope leaked")
	}
	if _, err := YAML(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing value accepted")
	}
	if _, err := Print(context.Background(), map[string]any{"content": 1}); err == nil {
		t.Fatal("non-text accepted")
	}
	if _, err := Print(context.Background(), map[string]any{"content": "", "stream": "file"}); err == nil {
		t.Fatal("unknown stream accepted")
	}
}
