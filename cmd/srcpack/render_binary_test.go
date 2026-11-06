package main

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestProductionPackBinaryBytesWithLegacyCRCleanup(t *testing.T) {
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"nul_invalid_utf8", []byte("begin\x00middle\xff\xfeend")},
		{"all_bytes", allBytes},
		{"newline", []byte("${PROFILE_NAME}\x00\xff\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := materializeWorkspace(t, map[string]string{"binary.txt": string(tc.raw)})
			t.Chdir(root)
			err := execute(context.Background(), []string{"pack", "--file", "binary.txt", "--profile", "source", "--editor", "raw", "--no-header", "--no-prompt", "--output", "out.md", "--quiet"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("out.md")
			if err != nil {
				t.Fatal(err)
			}
			// The preexisting parser/clean.go cleanup removes CR before rendering.
			// This test preserves that CLI contract; the isolated rendering gate
			// separately proves that CR and every other byte survive NFlow itself.
			want := bytes.ReplaceAll(tc.raw, []byte{'\r'}, nil)
			if len(want) > 0 && want[len(want)-1] != '\n' {
				want = append(want, '\n')
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("production CLI byte mismatch: want %x, got %x", want, got)
			}
			input, err := os.ReadFile("binary.txt")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(input, tc.raw) {
				t.Fatal("production pack changed source file")
			}
		})
	}
}
