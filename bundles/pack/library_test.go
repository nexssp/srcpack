package packcmd

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"testing"

	"github.com/nexssp/transport/tcli"

	"github.com/nexssp/srcpack/bundles/cliinput"
	"github.com/nexssp/srcpack/bundles/config"
)

func TestToIntValidValues(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		value any
		want  int
	}{
		{name: "int", value: 7, want: 7},
		{name: "int32", value: int32(7), want: 7},
		{name: "int64", value: int64(7), want: 7},
		{name: "float", value: 7.9, want: 7},
		{name: "json number", value: json.Number("7"), want: 7},
		{name: "string", value: "7", want: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := toInt(test.value); got != test.want {
				t.Fatalf("toInt(%#v) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}

func TestToIntInvalidAndOutOfRangeValuesUseZeroFallback(t *testing.T) {
	t.Parallel()

	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "invalid string", value: "not an integer"},
		{name: "invalid JSON number", value: json.Number("not a number")},
		{name: "JSON number overflow", value: json.Number("9223372036854775808")},
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "float above int range", value: float64(maxInt) + 1},
		{name: "float below int range", value: math.Nextafter(float64(minInt), math.Inf(-1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := toInt(test.value); got != 0 {
				t.Fatalf("toInt(%#v) = %d, want zero fallback", test.value, got)
			}
		})
	}

	if strconv.IntSize == 32 {
		if got := toInt(maxInt + 1); got != 0 {
			t.Fatalf("out-of-range int64 conversion = %d, want zero fallback", got)
		}
	}
}

func TestSelectorTargetKeyRemovesSupportedLineRanges(t *testing.T) {
	t.Parallel()

	want := selectorTargetKey("file.go", "/workspace")
	for _, selector := range []string{
		"file.go:4",
		"file.go:4-12",
		"file.go:4-",
		"file.go:-12",
	} {
		if got := selectorTargetKey(selector, "/workspace"); got != want {
			t.Errorf("selectorTargetKey(%q) = %q, want %q", selector, got, want)
		}
	}
}

func TestProfileLookupUsesBuiltinProfilesAndAliases(t *testing.T) {
	t.Parallel()
	for _, profile := range config.BuiltinProfiles {
		if !isProfile(profile.Name) || canonicalProfile(profile.Name) != profile.Name {
			t.Errorf("built-in profile %q did not resolve to itself", profile.Name)
		}
		for _, alias := range profile.Aliases {
			if !isProfile(alias) || canonicalProfile(alias) != profile.Name {
				t.Errorf("alias %q did not resolve consistently to %q", alias, profile.Name)
			}
		}
		if !containsProfileName(builtinProfileNames(), profile.Name) {
			t.Errorf("built-in profile %q missing from NFlow validation names", profile.Name)
		}
	}
}

func containsProfileName(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestResolveMaskOverridePreservesDefaultAndFollowsExplicitFlagOrder(t *testing.T) {
	t.Parallel()
	if !(config.ConfigOptions{}).ToOptions().MaskSecrets {
		t.Fatal("secret masking defaulted off")
	}

	schema := tcli.CLIRequestSchema{Name: "PackInput", Fields: []tcli.CLIFieldSpec{
		{Name: "mask", Type: "bool", Flags: []string{"mask", "mask-secrets"}},
		{Name: "no_mask", Type: "bool", Flags: []string{"no-mask"}},
	}}
	for _, test := range []struct {
		name string
		args []string
		want *bool
	}{
		{name: "unset preserves default"},
		{name: "positive flag", args: []string{"--mask"}, want: new(true)},
		{name: "positive false", args: []string{"--mask=false"}, want: new(false)},
		{name: "negative flag", args: []string{"--no-mask"}, want: new(false)},
		{name: "negative false", args: []string{"--no-mask=false"}, want: new(true)},
		{name: "positive last", args: []string{"--no-mask", "--mask"}, want: new(true)},
		{name: "negative last", args: []string{"--mask", "--no-mask"}, want: new(false)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := cliinput.WithRawArguments(context.Background(), test.args)
			ctx = cliinput.WithInputSchemas(ctx, map[string]tcli.CLIRequestSchema{"PackInput": schema})
			got, err := resolveMaskOverride(ctx, "pack", nil)
			if err != nil {
				t.Fatalf("resolveMaskOverride() error = %v", err)
			}
			if test.want == nil {
				if got != nil {
					t.Fatalf("resolveMaskOverride() = %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *test.want {
				t.Fatalf("resolveMaskOverride() = %v, want %v", got, *test.want)
			}
		})
	}
}
