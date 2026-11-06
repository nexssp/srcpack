package operators

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/nexssp/kernel/xtest/ktest"
)

type stage8TreeFixture struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Depth     int    `json:"depth"`
	Color     bool   `json:"color"`
	ShowFiles bool   `json:"show_files"`
	Output    string `json:"output"`
}

func TestStage8TreeParity(t *testing.T) {
	b, e := os.ReadFile("testdata/stage8_tree_parity.json")
	ktest.RequireNoError(t, e)
	var cases []stage8TreeFixture
	ktest.RequireNoError(t, json.Unmarshal(b, &cases))
	for _, c := range cases {
		t.Run(c.Name+"/"+strconv.Itoa(c.Depth)+"/color="+map[bool]string{true: "on", false: "off"}[c.Color]+"/files="+map[bool]string{true: "on", false: "off"}[c.ShowFiles], func(t *testing.T) {
			var got string
			if c.Kind == "context" {
				got = GenerateContextTree(stage8ParityFiles(), c.Depth, c.Color, c.ShowFiles)
			} else {
				root := stage8FullFixture(t)
				got = GenerateFullProjectTree(root, stage8ParityFiles(), c.Depth, c.Color, c.ShowFiles)
			}
			ktest.RequireEqual(t, stage8NormalizeLanguageTies(got), stage8NormalizeLanguageTies(c.Output))
		})
	}
}
