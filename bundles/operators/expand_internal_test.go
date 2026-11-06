package operators

import (
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

func TestExpandPlaceholders_Basic(t *testing.T) {
	t.Parallel()

	lookup := func(key string) (string, bool) {
		switch key {
		case "NAME":
			return "server", true
		case "PORT":
			return "8080", true
		default:
			return "", false
		}
	}

	got := expandPlaceholders("host=${NAME} port=${PORT}", lookup)
	ktest.RequireEqual(t, got, "host=server port=8080")
}

func TestExpandPlaceholders_PreservesUnknown(t *testing.T) {
	t.Parallel()

	lookup := func(key string) (string, bool) {
		if key == "KNOWN" {
			return "yes", true
		}
		return "", false
	}

	got := expandPlaceholders("known=${KNOWN} unknown=${MISSING}", lookup)
	ktest.RequireEqual(t, got, "known=yes unknown=${MISSING}")
}

func TestExpandPlaceholders_NoDollarReturnsSameString(t *testing.T) {
	t.Parallel()

	input := "no placeholders here"
	got := expandPlaceholders(input, func(string) (string, bool) { return "", false })
	ktest.RequireEqual(t, got, input)
}

func TestExpandPlaceholders_UnclosedBrace(t *testing.T) {
	t.Parallel()

	input := "before ${UNCLOSED after"
	got := expandPlaceholders(input, func(string) (string, bool) { return "", false })
	ktest.RequireEqual(t, got, input)
}

func TestExpandPlaceholders_EmptyValue(t *testing.T) {
	t.Parallel()

	lookup := func(key string) (string, bool) {
		if key == "EMPTY" {
			return "", true
		}
		return "", false
	}

	got := expandPlaceholders("a=${EMPTY}b", lookup)
	ktest.RequireEqual(t, got, "a=b")
}

func TestExpandPlaceholders_AdjacentTokens(t *testing.T) {
	t.Parallel()

	lookup := func(key string) (string, bool) {
		switch key {
		case "A":
			return "1", true
		case "B":
			return "2", true
		default:
			return "", false
		}
	}

	got := expandPlaceholders("${A}${B}", lookup)
	ktest.RequireEqual(t, got, "12")
}
