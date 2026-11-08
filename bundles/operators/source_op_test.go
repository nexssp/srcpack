package operators_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"

	"github.com/nexssp/srcpack/bundles/operators"
)

func TestSource_Guardrails_FileSize(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "small.go", "package a\n")
	writeFile(t, dir, "huge.go", make([]byte, 128))

	t.Chdir(dir)

	guardrails := operators.NewGuardrails(64, 0)
	err := guardrails.CheckFile(filepath.Join(dir, "huge.go"), mustStat(t, filepath.Join(dir, "huge.go")))
	ktest.RequireErrorContains(t, err, "exceeds maximum allowed size")
}

func TestSource_Guardrails_FileCount(t *testing.T) {
	guardrails := operators.NewGuardrails(0, 2)
	info := mustStat(t, mustWriteTemp(t))

	ktest.RequireNoError(t, guardrails.CheckFile("a.go", info))
	ktest.RequireNoError(t, guardrails.CheckFile("b.go", info))
	err := guardrails.CheckFile("c.go", info)
	ktest.RequireErrorContains(t, err, "scan runaway prevented")
}

func TestSource_OmittedFilesDoNotConsumeFileCountLimit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package a\n")
	writeFile(t, dir, "one_test.go", "package a\n")
	writeFile(t, dir, "two_test.go", "package a\n")
	t.Chdir(dir)

	collected := collectSource(t, operators.PackConfig{
		Targets:      []string{"."},
		NoTests:      true,
		MaxFilesScan: 1,
	})
	if len(collected) != 3 {
		t.Fatalf("source emitted %d records, want one included file and two omitted markers", len(collected))
	}
	var included, omitted int
	for _, file := range collected {
		if file.Omitted {
			omitted++
		} else {
			included++
		}
	}
	if included != 1 || omitted != 2 {
		t.Fatalf("source emitted included=%d omitted=%d, want included=1 omitted=2", included, omitted)
	}
}

func TestSource_ExcludesGeneratedDepsExamples(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package a\n")
	writeFile(t, dir, "service_generated.go", "package a\n")
	writeFile(t, dir, "vendor/lib.go", "package lib\n")
	writeFile(t, dir, "examples/demo.go", "package demo\n")
	writeFile(t, dir, "service_test.go", "package a\n")

	t.Chdir(dir)

	collected := collectSource(t, operators.PackConfig{
		Targets:     []string{"."},
		NoTests:     true,
		NoGenerated: true,
		NoDeps:      true,
		NoExamples:  true,
	})

	wantIncluded := map[string]bool{"main.go": true}
	for _, item := range collected {
		if item.Omitted {
			continue
		}
		delete(wantIncluded, item.RelPath)
	}
	if len(wantIncluded) != 0 {
		t.Fatalf("expected exactly main.go included; missing %v", wantIncluded)
	}

	excludedReasons := make(map[string]string)
	for _, item := range collected {
		if item.Omitted {
			excludedReasons[item.RelPath] = item.OmitReason
		}
	}
	ktest.RequireEqual(t, excludedReasons["service_test.go"], "test")
	ktest.RequireEqual(t, excludedReasons["service_generated.go"], "generated")
	ktest.RequireEqual(t, excludedReasons["examples/demo.go"], "examples")
	// vendor/ is not tree-visible, so it must not appear as an omission.
	if _, present := excludedReasons["vendor/lib.go"]; present {
		t.Fatal("deps files must not be surfaced as omission markers")
	}
}

func TestSource_ExtensionsFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n")
	writeFile(t, dir, "b.ts", "export {}\n")

	t.Chdir(dir)

	collected := collectSource(t, operators.PackConfig{
		Targets: []string{"."},
		Ext:     "go",
	})

	var included []string
	for _, item := range collected {
		if item.Omitted {
			continue
		}
		included = append(included, item.RelPath)
	}
	ktest.RequireEqual(t, included, []string{"a.go"})
}

func TestSource_LineRangeSelectors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "snippet.go", "one\ntwo\nthree\nfour\nfive\n")
	t.Chdir(dir)

	for _, test := range []struct {
		selector string
		want     string
	}{
		{selector: "snippet.go:2-3", want: "two\nthree"},
		{selector: "snippet.go:3-", want: "three\nfour\nfive\n"},
		{selector: "snippet.go:-2", want: "one\ntwo"},
		{selector: "snippet.go:4", want: "four"},
	} {
		t.Run(test.selector, func(t *testing.T) {
			collected := collectSource(t, operators.PackConfig{Targets: []string{test.selector}})
			if len(collected) != 1 {
				t.Fatalf("source emitted %d records, want one", len(collected))
			}
			if got := string(collected[0].Content); got != test.want {
				t.Fatalf("source content = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSource_PreservesOriginalLineRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sample.txt", "one\ntwo\nthree\nfour\nfive\n")
	t.Chdir(dir)

	collected := collectSource(t, operators.PackConfig{Targets: []string{"sample.txt:2-4"}})
	if len(collected) != 1 {
		t.Fatalf("source emitted %d records, want one", len(collected))
	}
	file := collected[0]
	ktest.RequireEqual(t, file.SourceStartLine, 2)
	ktest.RequireEqual(t, file.SourceEndLine, 4)
	ktest.RequireEqual(t, string(file.Content), "two\nthree\nfour")
}

func TestSource_ExcludesSensitiveFilesEvenWhenExplicit(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		".env", ".env.production", ".npmrc", "credentials.json",
		"service-account.json", "private.pem", "safe.yml",
	} {
		writeFile(t, dir, name, "key: value\n")
	}
	t.Chdir(dir)

	collected := collectSource(t, operators.PackConfig{
		Targets:       []string{".env", ".env.production", ".npmrc", "credentials.json", "service-account.json", "private.pem", "safe.yml"},
		IncludeConfig: true,
	})
	var included []string
	for _, file := range collected {
		included = append(included, file.RelPath)
	}
	ktest.RequireEqual(t, included, []string{"safe.yml"})
}

// ── helpers ──────────────────────────────────────────────────────────

func collectSource(t *testing.T, cfg operators.PackConfig) []operators.ParsedFile {
	t.Helper()
	stream := operators.SourceOperator()
	seq, err := stream.DoStreamAny(context.Background(), cfg)
	ktest.RequireNoError(t, err)

	var out []operators.ParsedFile
	seq(func(item any, itemErr error) bool {
		if itemErr != nil {
			t.Fatalf("stream error: %v", itemErr)
		}
		file, ok := item.(operators.ParsedFile)
		if !ok {
			t.Fatalf("unexpected item type %T", item)
		}
		out = append(out, file)
		return true
	})
	return out
}

func writeFile(t *testing.T, dir, rel string, content any) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var data []byte
	switch v := content.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func mustWriteTemp(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
