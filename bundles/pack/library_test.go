package packcmd

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
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
