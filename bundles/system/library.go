// Package systemcmd implements srcpack's built-in system and informational commands.
package systemcmd

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"

	"github.com/nexssp/srcpack/bundles/config"
	"github.com/nexssp/srcpack/bundles/operators"
)

const (
	ID             = "srcpack_system"
	defaultVersion = "dev"
)

//go:embed *.nflow help.txt cookbook.md
var FS embed.FS

type versionKey struct{}

// WithVersion carries the host's linked version into system flows without package-global state.
func WithVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, versionKey{}, version)
}

// Version reads the version supplied by the host, falling back to the development version.
func Version(ctx context.Context) string {
	version, _ := ctx.Value(versionKey{}).(string)
	if version == "" {
		return defaultVersion
	}
	return version
}

// HelpText returns the full static command reference with the current tree-depth default.
func HelpText() string {
	data, err := FS.ReadFile("help.txt")
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(string(data), "{{TREE_DEPTH}}", strconv.Itoa(config.DefaultTreeDepth))
}

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/system", Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/system/nexssflow", Bundle)
}

func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{
		ID: ID,
		Libraries: []action.Library{{Name: ID, Sources: []action.AnyStreamAction{
			action.NewStream("system.demo.sample", demoSample),
		}, Actions: []action.AnyAction{
			action.New("system.dashboard.facts", dashboardFacts).Build(),
			action.New("system.help", help).Build(),
			action.New("system.cookbook", cookbook).Build(),
			action.New("system.version", version).Build(),
			action.New("system.profiles", profiles).Build(),
			action.New("system.profile_info", profileInfo).Build(),
			action.New("system.clean.inspect", cleanInspect).Build(),
			action.New("system.clean.report_dry_run", cleanDryRun).Build(),
			action.New("system.clean.remove", cleanRemove).Build(),
			action.New("system.doctor.context", doctorContext).Build(),
			action.New("system.doctor.facts", doctorFacts).Build(),
			action.New("system.doctor.path_stat", doctorPathStat).Build(),
			action.New("system.doctor.format", doctorFormat).Build(),
			DoctorRawAction(),
			action.New("system.health.inspect", healthInspect).Build(),
			action.New("system.demo.report", demoReport).Build(),
			action.New("system.find", find).Build(),
		}}},
	}
}

// Loader resolves this bundle's embedded source files by their namespaced path.
func Loader() compiler.SourceLoader { return sourceLoader{} }

type sourceLoader struct{}

func (sourceLoader) Load(_, target string) ([]byte, string, error) {
	name := filepath.Base(target)
	if name != "system.nflow" && name != "schema.nflow" && name != "doctor.nflow" && name != "doctor_schema.nflow" {
		return nil, "", fmt.Errorf("srcpack system: unknown source %q", target)
	}
	data, err := FS.ReadFile(name)
	return data, ID + "/" + name, err
}

func dashboardFacts(_ context.Context, _ map[string]any) (map[string]any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	projectRoot, configPath := config.FindRoot(cwd)
	cfg, err := config.Load(cwd, "")
	if err != nil {
		return nil, err
	}
	targetNames := make([]string, 0, len(cfg.TargetsMap))
	for name := range cfg.TargetsMap {
		targetNames = append(targetNames, name)
	}
	viewNames := make([]string, 0, len(cfg.Views))
	for name := range cfg.Views {
		viewNames = append(viewNames, name)
	}
	configName := ""
	if configPath != "" {
		configName = filepath.Base(configPath)
	}
	return map[string]any{
		"cwd":          cwd,
		"project_root": projectRoot,
		"config_name":  configName,
		"target_names": targetNames,
		"view_names":   viewNames,
	}, nil
}

func help(_ context.Context, _ map[string]any) (map[string]any, error) {
	return printed(HelpText()), nil
}

func cookbook(_ context.Context, _ map[string]any) (map[string]any, error) {
	content, err := FS.ReadFile("cookbook.md")
	if err != nil {
		return nil, fmt.Errorf("load srcpack cookbook: %w", err)
	}
	return printed(string(content)), nil
}

func version(ctx context.Context, _ map[string]any) (map[string]any, error) {
	return printed("srcpack " + Version(ctx) + "\n"), nil
}

func profiles(_ context.Context, _ map[string]any) (map[string]any, error) {
	var text strings.Builder
	text.WriteString("Built-in profiles:\n")
	for i := range config.BuiltinProfiles {
		profile := &config.BuiltinProfiles[i]
		fmt.Fprintf(&text, "  %-12s %s\n", profile.Name, profile.Description)
	}
	return printed(text.String()), nil
}

const systemErrorField = "error"

func profileInfo(_ context.Context, in map[string]any) (map[string]any, error) {
	name, _ := in["name"].(string)
	canonical := config.CanonicalProfileName(name)
	var description string
	var ok bool
	for i := range config.BuiltinProfiles {
		profile := &config.BuiltinProfiles[i]
		if profile.Name == canonical {
			description, ok = profile.Description, true
			break
		}
	}
	if !ok {
		return map[string]any{systemErrorField: fmt.Sprintf("unknown profile %q; run 'srcpack list' to see built-in profiles", canonical)}, nil
	}
	return printed(fmt.Sprintf("%s: %s\n", canonical, description)), nil
}

func cleanInspect(_ context.Context, in map[string]any) (map[string]any, error) {
	out := clone(in)
	dir, _ := in["path"].(string)
	out["path"] = dir
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			out["count"] = 0
			out["error"] = ""
			return out, nil
		}
		out["error"] = err.Error()
		return out, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		out["error"] = err.Error()
		return out, nil
	}
	if !info.IsDir() {
		out["error"] = fmt.Sprintf("output path %q is not a directory", dir)
		return out, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		out["error"] = err.Error()
		return out, nil
	}
	defer root.Close()
	count := 0
	err = filepath.WalkDir(dir, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		} // WalkDir never follows symlinked directories.
		if generated(entry.Name()) {
			count++
		}
		return nil
	})
	if err != nil {
		out["error"] = err.Error()
		return out, nil
	}
	out["count"], out["error"] = count, ""
	return out, nil
}

func cleanDryRun(_ context.Context, in map[string]any) (map[string]any, error) {
	count, _ := in["count"].(int)
	return printed(fmt.Sprintf("[DRY-RUN] %d generated file(s) would be removed\n", count)), nil
}

func cleanRemove(_ context.Context, in map[string]any) (map[string]any, error) {
	dir, _ := in["path"].(string)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	count := 0
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !generated(entry.Name()) {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if removeErr := root.Remove(rel); removeErr != nil {
			return removeErr
		}
		count++
		return nil
	})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return printed("Nothing to clean\n"), nil
	}
	_ = os.Remove(dir) // Compatibility: remove the now-empty output directory, if possible.
	return printed(fmt.Sprintf("🧹 Cleaned %d generated file(s)\n", count)), nil
}

func generated(name string) bool {
	return strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".xml")
}

func healthInspect(ctx context.Context, _ map[string]any) (map[string]any, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	_, path := config.FindRoot(cwd)
	valid := false
	if path != "" {
		if _, err := config.Load(cwd, path); err == nil {
			valid = true
		}
	}
	return map[string]any{"version": Version(ctx), "config_present": path != "", "config_valid": valid, "working_dir": cwd}, nil
}

func demoSample(_ context.Context, _ map[string]any) (iter.Seq2[operators.ParsedFile, error], error) {
	files := []operators.ParsedFile{
		{
			Path: "demo/example.go", RelPath: "demo/example.go", Lang: "go",
			Content: []byte("package example\n\n// Add returns the sum of two ints.\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc internal() {}\n"),
		},
		{
			Path: "demo/example.ts", RelPath: "demo/example.ts", Lang: "typescript",
			Content: []byte("export function greet(name: string): string {\n  return name;\n}\n"),
		},
		{
			Path: "demo/Component.tsx", RelPath: "demo/Component.tsx", Lang: "typescript",
			Content: []byte("export interface Props { label: string }\nexport const Component = (props: Props) => <button>{props.label}</button>;\n"),
		},
		{
			Path: "demo/example.rs", RelPath: "demo/example.rs", Lang: "rust",
			Content: []byte("pub fn add(a: i32, b: i32) -> i32 {\n    a + b\n}\n"),
		},
		{
			Path: "demo/example.php", RelPath: "demo/example.php", Lang: "php",
			Content: []byte("<?php\nnamespace Demo;\nfunction greet(string $name): string { return 'Hello ' . $name; }\n"),
		},
		{
			Path: "demo/example.py", RelPath: "demo/example.py", Lang: "python",
			Content: []byte("def greet(name: str) -> str:\n    return f'Hello {name}'\n"),
		},
	}
	return func(yield func(operators.ParsedFile, error) bool) {
		for i := range files {
			if !yield(files[i], nil) {
				return
			}
		}
	}, nil
}

func demoReport(_ context.Context, in any) (map[string]any, error) {
	items, _ := in.([]any)
	var b strings.Builder
	for _, item := range items {
		file, ok := item.(operators.ParsedFile)
		if !ok {
			return nil, fmt.Errorf("demo: expected parsed file, got %T", item)
		}
		fmt.Fprintf(&b, "### %s [%s]\n", file.RelPath, file.Lang)
		if len(file.Content) > 0 {
			b.Write(file.Content)
		} else {
			for _, snippet := range file.Snippets {
				b.WriteString(snippet)
				b.WriteByte('\n')
			}
		}
		b.WriteByte('\n')
	}
	return printed(b.String()), nil
}

func printed(content string) map[string]any {
	return map[string]any{"content": content, "stream": "stdout"}
}

func clone(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+4)
	maps.Copy(out, in)
	return out
}
