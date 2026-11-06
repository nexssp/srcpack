package cliinput

import (
	"context"
	"testing"
)

func TestHasFlag(t *testing.T) {
	ctx := WithArguments(context.Background(), []string{"--copy=false", "src", "--", "--no-copy"})
	if !HasFlag(ctx, "copy", "c") {
		t.Fatal("did not detect explicit --copy")
	}
	if HasFlag(ctx, "no-copy") {
		t.Fatal("detected a flag after --")
	}
}

func TestResolveBooleanAliasOrder(t *testing.T) {
	pair := BoolAliases{Positive: "copy", Negative: "no_copy", PositiveFlags: []string{"copy", "c"}, NegativeFlags: []string{"no-copy"}}
	for _, tc := range []struct {
		args                     []string
		positive, negative, want bool
	}{
		{[]string{"--copy", "--no-copy"}, true, true, false},
		{[]string{"--no-copy", "-c"}, true, true, true},
		{[]string{"--copy=false", "--no-copy=false"}, false, false, true},
		{[]string{"--no-copy=false", "--copy=false"}, false, false, false},
		{[]string{"--copy", "--", "--no-copy"}, true, true, true},
		{[]string{"--copy", "--prompt=--no-copy"}, true, true, true},
	} {
		input := map[string]any{"copy": tc.positive, "no_copy": tc.negative, "other": "untouched"}
		got := ResolveBoolAliases(WithArguments(context.Background(), tc.args), input, pair)
		if got["copy"] != tc.want {
			t.Errorf("args %v: got %#v", tc.args, got)
		}
		if _, ok := got["no_copy"]; ok {
			t.Fatal("negative alias not consumed")
		}
		if _, ok := input["no_copy"]; !ok {
			t.Fatal("original map mutated")
		}
	}
}

func TestNegativeFalseAndStandaloneTypedMap(t *testing.T) {
	pair := BoolAliases{Positive: "comments", Negative: "no_comments"}
	got := ResolveBoolAliases(context.Background(), map[string]any{"no_comments": false}, pair)
	if got["comments"] != true {
		t.Fatalf("negative false lost: %#v", got)
	}
	untouched := ResolveBoolAliases(context.Background(), map[string]any{"quiet": false}, pair)
	if len(untouched) != 1 || untouched["quiet"] != false {
		t.Fatalf("unexpected defaults: %#v", untouched)
	}
}
