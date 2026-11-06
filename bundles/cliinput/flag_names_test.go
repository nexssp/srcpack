package cliinput

import (
	"reflect"
	"testing"
)

func TestNormalizeFlagNames(t *testing.T) {
	args := []string{"pack", "--CoMmEnTs=false", "-OUTPUT=Mixed.Case", "-L", "2", "-C", "Project", "--PROMPT=--COPY", "--", "--COPY"}
	want := []string{"pack", "--comments=false", "-output=Mixed.Case", "-L", "2", "-C", "Project", "--prompt=--COPY", "--", "--COPY"}
	got := NormalizeFlagNames(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if args[1] != "--CoMmEnTs=false" {
		t.Fatal("input mutated")
	}
}
