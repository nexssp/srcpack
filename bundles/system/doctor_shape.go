package systemcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/nexssp/kernel/action"
	"gopkg.in/yaml.v3"

	"github.com/nexssp/srcpack/bundles/config"
)

// DoctorRawAction returns the raw-config action used by doctor_shape(). The host
// registers it with the name system.doctor.raw; it is intentionally separate
// from Bundle so command wiring can opt into the hybrid doctor flow explicitly.
func DoctorRawAction() action.AnyAction {
	return action.New("system.doctor.raw", doctorRaw).Build()
}

// doctorRaw reads YAML into generic values, exposing only supported fields for
// Flow's shallow @schema shape checks. config.Load remains authoritative for
// known fields, nested types, custom YAML forms, and semantic decoding.
func doctorRaw(_ context.Context, input map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(input)+10)
	maps.Copy(out, input)
	out["shape_error"] = ""

	cwd, _ := input["cwd"].(string)
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			out["shape_error"] = fmt.Sprintf("resolve current directory: %v", err)
			return out, nil
		}
	}
	path, _ := input["path"].(string)
	readPath := path
	if readPath == "" {
		_, readPath = config.FindRoot(cwd)
	}
	if readPath == "" {
		// The caller normally handles missing-config discovery before this macro.
		// Keep the empty document shape-neutral if invoked independently.
		out["_doctor_yaml"] = map[string]any{}
		return out, nil
	}
	if !filepath.IsAbs(readPath) {
		readPath = filepath.Join(cwd, readPath)
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		out["shape_error"] = fmt.Sprintf("read config %s: %v", readPath, err)
		return out, nil
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		if errors.Is(err, io.EOF) {
			decoded = map[string]any{}
		} else {
			out["shape_error"] = fmt.Sprintf("parse config %s: %v", readPath, err)
			return out, nil
		}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			out["shape_error"] = fmt.Sprintf("parse config %s: multiple YAML documents are not supported", readPath)
		} else {
			out["shape_error"] = fmt.Sprintf("parse config %s: %v", readPath, err)
		}
		return out, nil
	}

	// yaml.v3's struct decoder accepts an empty/null document as a zero value.
	// Normalize only these two values; never turn a non-map root into an empty
	// object, because that would hide the raw shape error from @schema.
	if decoded == nil {
		decoded = map[string]any{}
	}
	decoded = doctorSchemaValue(decoded)
	out["_doctor_yaml"] = decoded
	configMap, ok := decoded.(map[string]any)
	if !ok {
		return out, nil
	}
	for _, key := range []string{
		"targets", "profiles", "prompts", "editors", "options", "views", "dir",
		"files", "includes", "include_profiles", "prompt", "start", "end", "summary",
	} {
		if value, present := configMap[key]; present {
			out[key] = value
		}
	}
	return out, nil
}

// YAML's generic decoder uses map[any]any for scalar non-string keys, while
// its typed map[string] decoder accepts those keys. This bridge exposes their
// object shape to @schema without replacing any wrong scalar/list shape.
// The original file, not this representation, is still decoded by config.Load.
func doctorSchemaValue(value any) any {
	switch object := value.(type) {
	case map[any]any:
		out := make(map[string]any, len(object))
		for key, item := range object {
			out[fmt.Sprint(key)] = doctorSchemaValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(object))
		for key, item := range object {
			out[key] = doctorSchemaValue(item)
		}
		return out
	case []any:
		out := make([]any, len(object))
		for i, item := range object {
			out[i] = doctorSchemaValue(item)
		}
		return out
	default:
		return value
	}
}
