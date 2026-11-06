package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/nflows"
)

func TestDemoProductionFoldAndDSLExtension(t *testing.T) {
	root := t.TempDir()
	if err := writeFixture(root); err != nil {
		t.Fatal(err)
	}
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeDemo(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	var rawBytes int
	for _, name := range []string{"hello.go", "math.go"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		rawBytes += len(data)
	}
	// These are parser snippets, before renderer headings/fences. Café checks
	// that Expr aggregation counts UTF-8 bytes rather than Unicode characters.
	packedBytes := len("func Café() string {}") + len("func Add(a, b int) int {}")
	for key, want := range map[string]int{
		"fileCount": 2, "funcCount": 2, "packedBytes": packedBytes, "rawBytes": rawBytes,
	} {
		if got := number(t, result.Baseline[key]); got != float64(want) {
			t.Errorf("baseline %s = %v, want %d", key, got, want)
		}
		if !reflect.DeepEqual(result.Extended[key], result.Baseline[key]) {
			t.Errorf("extension changed %s: %v -> %v", key, result.Baseline[key], result.Extended[key])
		}
	}
	if got := number(t, result.Extended["packedBytesPerFile"]); got != float64(packedBytes)/2 {
		t.Errorf("packedBytesPerFile = %v, want %v", got, float64(packedBytes)/2)
	}
	if got := number(t, result.Extended["nonContributingRecords"]); got != 0 {
		t.Errorf("nonContributingRecords = %v, want 0 (AST drops omitted markers)", got)
	}
	for _, want := range []string{"hello.go", "math.go", "func Café() string {}", "func Add(a, b int) int {}"} {
		if !strings.Contains(result.Context, want) {
			t.Errorf("context missing %q: %s", want, result.Context)
		}
	}
	for _, unwanted := range []string{"TestHello", "hello_test.go", "return", "greeting"} {
		if strings.Contains(result.Context, unwanted) {
			t.Errorf("context contains excluded text %q: %s", unwanted, result.Context)
		}
	}
	after, err := os.Getwd()
	if err != nil || before != after {
		t.Errorf("demo changed working directory: %q -> %q (%v)", before, after, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 3 {
		t.Errorf("demo wrote unexpected artifacts into fixture: %v (%v)", entries, err)
	}
}

func TestDemoEmptyAndExcludedInputs(t *testing.T) {
	for _, onlyExcluded := range []bool{false, true} {
		name := "empty workspace"
		if onlyExcluded {
			name = "excluded tests only"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if onlyExcluded {
				if err := os.WriteFile(filepath.Join(root, "only_test.go"), []byte("package demo\nfunc TestOnly() {}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := executeDemo(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range result.Baseline {
				if got := number(t, value); got != 0 {
					t.Errorf("empty baseline %s = %v, want 0", key, got)
				}
			}
			for key, value := range result.Extended {
				if got := number(t, value); got != 0 {
					t.Errorf("empty extension %s = %v, want 0", key, got)
				}
			}
			if strings.TrimSpace(result.Context) != "" {
				t.Errorf("empty context = %q", result.Context)
			}
		})
	}
}

func TestDemoSourceKeepsProductionFoldAndRemovesSideEffects(t *testing.T) {
	pack, err := nflows.FS.ReadFile("pack.nflow")
	if err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(string(pack), "    -> stats.collect @{\n")
	head = strings.Replace(head, "    -> workspace.save_state:enabled=$save_state\n", "", 1)
	head = strings.Replace(head, "@macro pack_body(save_state) {", "@pipeline pack", 1)
	for _, mode := range []string{"baseline", "extended", "render", "extended-render"} {
		source, err := demoSource(string(pack), mode)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(source, head) {
			t.Errorf("%s modified production source/schema/fold", mode)
		}
		for _, unwanted := range []string{"out.file", "clipboard.write", "workspace.save_state"} {
			if strings.Contains(source, unwanted) {
				t.Errorf("%s includes unwanted side effect %s", mode, unwanted)
			}
		}
	}
	if _, err := demoSource("changed upstream pipeline", "baseline"); err == nil {
		t.Fatal("pipeline drift should fail explicitly")
	}
	if _, err := demoSource(string(pack), "unknown"); err == nil {
		t.Fatal("unknown mode should fail explicitly")
	}
}

func TestDemoConsole(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Production fold", "report.nflow", "packedBytesPerFile", "renderer", "never uses the clipboard"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("console missing %q", want)
		}
	}
}

func number(t *testing.T, value any) float64 {
	t.Helper()
	switch value := value.(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	default:
		t.Fatalf("unexpected numeric type %T", value)
		return 0
	}
}
