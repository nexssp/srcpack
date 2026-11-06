package cliinput

import (
	"context"
	"reflect"
	"testing"

	flowschema "github.com/nexssp/flow/extensions/schema"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/transport/tcli"
)

func TestCommandLinePreservesArgumentsAndQuotesUnsafeWords(t *testing.T) {
	ctx := WithCommandLine(context.Background(), "srcpack", []string{"pack", "--prompt", "review API's behavior", "--output", "out.md"})
	got := CommandLine(ctx)
	want := "srcpack pack --prompt 'review API'\\''s behavior' --output out.md"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCommandLineEscapesControlCharacters(t *testing.T) {
	got := CommandLine(WithCommandLine(context.Background(), "srcpack", []string{"--prompt", "first\nsecond\x1b[31m"}))
	want := "srcpack --prompt 'first\\u000Asecond\\u001B[31m'"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCommandLineMissingContextIsEmpty(t *testing.T) {
	if got := CommandLine(context.Background()); got != "" {
		t.Fatalf("got %q, want empty command line", got)
	}
}

func TestOrderedFieldValuesPreservesInterleavedPositionalsAndFlags(t *testing.T) {
	schema := tcli.CLIRequestSchema{Name: "Input", Fields: []tcli.CLIFieldSpec{
		{Name: "selectors", Type: "[]string", Positional: true},
		{Name: "targets", Type: "[]string", Flags: []string{"target", "t"}},
		{Name: "prompt", Type: "string", Flags: []string{"prompt"}},
	}}
	args := []string{"before", "--target", "flagged", "after", "--target=last"}
	got := OrderedFieldValues(WithInputSchemas(WithRawArguments(context.Background(), args), map[string]tcli.CLIRequestSchema{"Input": schema}), "Input", "selectors", "targets")
	want := []FieldValue{
		{Field: "selectors", Value: "before", Index: 0},
		{Field: "targets", Value: "flagged", Index: 2},
		{Field: "selectors", Value: "after", Index: 3},
		{Field: "targets", Value: "last", Index: 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestOrderedFieldValuesHonorsLexicalBoundaries(t *testing.T) {
	schema := tcli.CLIRequestSchema{Name: "Input", Fields: []tcli.CLIFieldSpec{
		{Name: "targets", Type: "[]string", Flags: []string{"target", "t"}},
		{Name: "prompt", Type: "string", Flags: []string{"prompt"}},
		{Name: "quiet", Type: "bool", Flags: []string{"quiet", "q"}},
		{Name: "selectors", Type: "[]string", Positional: true},
	}}
	args := []string{
		"--prompt=--target", "--target", "same", "--t", "comma,name", "-t=with space",
		"--quiet", "false", "--", "after --target, and # @ ~ literals",
	}
	ctx := WithInputSchemas(WithRawArguments(context.Background(), args), map[string]tcli.CLIRequestSchema{"Input": schema})
	got := OrderedFieldValues(ctx, "Input", "targets", "prompt", "quiet", "selectors")
	want := []FieldValue{
		{Field: "prompt", Value: "--target", Index: 0},
		{Field: "targets", Value: "same", Index: 2},
		{Field: "targets", Value: "comma,name", Index: 4},
		{Field: "targets", Value: "with space", Index: 5},
		{Field: "quiet", Value: "false", Index: 7},
		{Field: "selectors", Value: "after --target, and # @ ~ literals", Index: 9},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestOrderedFieldValuesEmptyEqualsConsumesFollowingToken(t *testing.T) {
	schema := tcli.CLIRequestSchema{Name: "Input", Fields: []tcli.CLIFieldSpec{
		{Name: "prompt", Type: "string", Flags: []string{"prompt"}},
		{Name: "target", Type: "string", Flags: []string{"target"}},
	}}
	ctx := WithInputSchemas(WithRawArguments(context.Background(), []string{"--prompt=", "literal", "--target=inline"}), map[string]tcli.CLIRequestSchema{"Input": schema})
	got := OrderedFieldValues(ctx, "Input", "prompt", "target")
	want := []FieldValue{{Field: "prompt", Value: "literal", Index: 1}, {Field: "target", Value: "inline", Index: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestOrderedFieldValuesRequiresBothContextInputsAndSelectsFields(t *testing.T) {
	schema := tcli.CLIRequestSchema{Name: "Input", Fields: []tcli.CLIFieldSpec{
		{Name: "target", Type: "string", Flags: []string{"target"}},
		{Name: "prompt", Type: "string", Flags: []string{"prompt"}},
	}}
	if got := OrderedFieldValues(context.Background(), "Input", "target"); got != nil {
		t.Fatalf("without context got %#v, want nil", got)
	}
	ctx := WithRawArguments(context.Background(), []string{"--prompt=skip", "--target", "keep"})
	if got := OrderedFieldValues(ctx, "Input", "target"); got != nil {
		t.Fatalf("without schema context got %#v, want nil", got)
	}
	ctx = WithInputSchemas(ctx, map[string]tcli.CLIRequestSchema{"Input": schema})
	got := OrderedFieldValues(ctx, "Input", "target")
	want := []FieldValue{{Field: "target", Value: "keep", Index: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWrapInputSchemasAttachesOnlyCLIRequestSchemas(t *testing.T) {
	meta := map[string]any{"schemas": []flowschema.Schema{{
		Name: "Input",
		Fields: []flowschema.Field{
			{Name: "Target", JSONName: "target", Type: "[]string", Tags: map[string]string{"cli": "target,t"}},
			{Name: "Note", JSONName: "note", Type: "string"},
		},
	}}}
	inner := action.New("inner", func(ctx context.Context, req map[string]any) (map[string]any, error) {
		got := OrderedFieldValues(ctx, "Input", "target")
		want := []FieldValue{{Field: "target", Value: "one", Index: 2}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("inside wrapped action: got %#v, want %#v", got, want)
		}
		return req, nil
	}).Build()
	wrapped, err := WrapInputSchemas(meta, inner)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRawArguments(context.Background(), []string{"before", "--t", "one"})
	if _, err := wrapped.DoAny(ctx, map[string]any{}); err != nil {
		t.Fatal(err)
	}
}
