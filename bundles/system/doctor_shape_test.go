package systemcmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDoctorRawPreservesGenericYAMLFormsAndContext(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "nexss.srcpack.yml")
	data := `shared: &shared
  tree_depth: 3
options: *shared
targets: null
profiles:
  custom:
    prompt: false
prompts:
  disabled: false
dir:
  - src
  - path: lib
    full: [go, ts]
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := doctorRaw(context.Background(), map[string]any{"cwd": cwd, "path": path, "caller": "preserved"})
	if err != nil {
		t.Fatal(err)
	}
	if got["cwd"] != cwd || got["path"] != path || got["caller"] != "preserved" {
		t.Fatalf("request context was not preserved: %#v", got)
	}
	if got["shape_error"] != "" {
		t.Fatalf("shape_error = %v", got["shape_error"])
	}
	raw, ok := got["_doctor_yaml"].(map[string]any)
	if !ok {
		t.Fatalf("generic YAML root type = %T, want map[string]any", got["_doctor_yaml"])
	}
	if _, optionsOK := raw["options"].(map[string]any); !optionsOK {
		t.Fatalf("YAML alias was not retained as a mapping: %T", raw["options"])
	}
	if raw["targets"] != nil {
		t.Fatalf("explicit null should remain null, got %#v", raw["targets"])
	}
	profiles, ok := raw["profiles"].(map[string]any)
	if !ok {
		t.Fatalf("profiles have type %T, want map[string]any", raw["profiles"])
	}
	custom, ok := profiles["custom"].(map[string]any)
	if !ok {
		t.Fatalf("custom profile has type %T, want map[string]any", profiles["custom"])
	}
	if custom["prompt"] != false {
		t.Fatalf("FlexibleText boolean form changed: %#v", custom["prompt"])
	}
	dir, ok := raw["dir"].([]any)
	if !ok || len(dir) == 0 {
		t.Fatalf("custom dir has value %#v, want at least one entry", raw["dir"])
	}
	if dir[0] != "src" {
		t.Fatalf("custom DirEntry scalar changed: %#v", dir[0])
	}
	if !reflect.DeepEqual(got["profiles"], raw["profiles"]) || !reflect.DeepEqual(got["options"], raw["options"]) {
		t.Fatal("top-level fields were not exposed for the schema gate")
	}
}

func TestDoctorRawNormalizesEmptyAndNullDocumentsButNotScalarRoots(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "config.yml")
	for _, tc := range []struct {
		name    string
		body    string
		wantMap bool
	}{
		{name: "empty", body: "", wantMap: true},
		{name: "null", body: "null\n", wantMap: true},
		{name: "scalar", body: "false\n", wantMap: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := doctorRaw(context.Background(), map[string]any{"cwd": cwd, "path": path})
			if err != nil {
				t.Fatal(err)
			}
			_, isMap := got["_doctor_yaml"].(map[string]any)
			if isMap != tc.wantMap {
				t.Fatalf("raw root = %#v (%T), expected map=%v", got["_doctor_yaml"], got["_doctor_yaml"], tc.wantMap)
			}
			if got["cwd"] != cwd || got["path"] != path {
				t.Fatalf("context lost: %#v", got)
			}
		})
	}
}

func TestDoctorRawReportsMalformedYAMLAsShapeErrorData(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "bad.yml")
	if err := os.WriteFile(path, []byte("targets: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := doctorRaw(context.Background(), map[string]any{"cwd": cwd, "path": path})
	if err != nil {
		t.Fatal(err)
	}
	if got["cwd"] != cwd || got["path"] != path || got["shape_error"] == "" {
		t.Fatalf("expected preserved context and parse error envelope, got %#v", got)
	}
}
