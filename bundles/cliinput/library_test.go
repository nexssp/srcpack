package cliinput

import (
	"context"
	"reflect"
	"testing"

	"github.com/nexssp/transport/tcli"
)

func TestProtectCommasRoundTripThroughTCLI(t *testing.T) {
	schema := tcli.CLIRequestSchema{Name: "Input", Fields: []tcli.CLIFieldSpec{
		{Name: "selectors", Type: "[]string", Positional: true},
		{Name: "targets", Type: "[]string", Flags: []string{"target", "t"}},
		{Name: "ext", Type: "string", Flags: []string{"ext"}},
		{Name: "prompt", Type: "string", Flags: []string{"prompt"}},
	}}
	args := []string{"a,b#arch", "c#source@explain x,y", "--target", "d,e#impl", "--ext=go,rs", "--prompt", "literal \ue000srcpack\ue001, comma"}
	protected, marker := ProtectCommas(args)
	parsed, err := tcli.ParseCLIRequest(protected, schema)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Restore(WithInvocation(context.Background(), "pack", true, marker), parsed)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"selectors": []string{"a,b#arch", "c#source@explain x,y"}, "targets": []string{"d,e#impl"}, "ext": "go,rs", "prompt": "literal \ue000srcpack\ue001, comma", "_command": "pack", "_explicit_pack": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestRestoreDoesNotMutateOrOverrideExplicitFacts(t *testing.T) {
	input := map[string]any{"_command": "pack", "_explicit_pack": false, "selectors": []string{"aMARKcb"}, "check": false}
	got, err := Restore(WithInvocation(context.Background(), "update", true, "MARK"), input)
	if err != nil {
		t.Fatal(err)
	}
	if got["_command"] != "pack" || got["_explicit_pack"] != false || got["check"] != false {
		t.Fatalf("facts changed: %#v", got)
	}
	selectors, ok := got["selectors"].([]string)
	if !ok {
		t.Fatalf("restored selectors have type %T, want []string", got["selectors"])
	}
	original, ok := input["selectors"].([]string)
	if !ok {
		t.Fatalf("input selectors have type %T, want []string", input["selectors"])
	}
	if len(selectors) != 1 || len(original) != 1 {
		t.Fatalf("selector lengths = %d and %d, want 1", len(selectors), len(original))
	}
	if selectors[0] != "a,b" || original[0] != "aMARKcb" {
		t.Fatal("restore mutated request or lost comma")
	}
}

func TestStandaloneRestoreLeavesFieldsAlone(t *testing.T) {
	input := map[string]any{"dirs": []string{"a,b"}}
	got, err := Restore(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, got) {
		t.Fatalf("got %#v", got)
	}
}

func TestStdoutDashAndEndOfFlags(t *testing.T) {
	args := []string{"-o", "-", "--", "-dash,file.go"}
	protected, marker := ProtectCommas(args)
	if protected[2] != "--" {
		t.Fatal("end-of-flags marker changed")
	}
	schema := tcli.CLIRequestSchema{Name: "Input", Fields: []tcli.CLIFieldSpec{
		{Name: "output", Type: "string", Flags: []string{"o"}},
		{Name: "selectors", Type: "[]string", Positional: true},
	}}
	parsed, err := tcli.ParseCLIRequest(protected, schema)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(WithInvocation(context.Background(), "pack", true, marker), parsed)
	if err != nil {
		t.Fatal(err)
	}
	if restored["output"] != "-" || !reflect.DeepEqual(restored["selectors"], []string{"-dash,file.go"}) {
		t.Fatalf("lost values: %#v", restored)
	}
}

func TestDerivedRequestDoesNotDecodeTransportMarkers(t *testing.T) {
	input := map[string]any{"_derived_request": true, "_command": "pack", "selectors": []string{"literalMARKcname"}}
	got, err := Restore(WithInvocation(context.Background(), "update", true, "MARK"), input)
	if err != nil || !reflect.DeepEqual(got, input) {
		t.Fatalf("derived request was decoded or overlaid: %#v %v", got, err)
	}
}
