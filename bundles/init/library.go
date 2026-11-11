package initcmd

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/kernel/xfs"

	// Register the CLI input bundle used by the init NFlow pipeline.
	_ "github.com/nexssp/srcpack/bundles/cliinput"
	"github.com/nexssp/srcpack/bundles/config"
)

const ID = "srcpack_init"

// FS contains this command's pipeline and request schema.
//
//go:embed *.nflow
var FS embed.FS

func init() {
	compiler.Register(ID, Bundle)
	compiler.Register("github.com/nexssp/srcpack/bundles/init/nexssflow", Bundle)
}

func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{
		ID: ID,
		Libraries: []action.Library{{
			Name: ID,
			Actions: []action.AnyAction{
				action.New("init.inspect", inspectWorkspace).Build(),
				action.New("init.clean_generated", cleanGeneratedDir).Build(),
				action.New("init.preview_clean_generated", previewCleanGeneratedDir).Build(),
				action.New("init.generate_targets", generateTargets).Build(),
				action.New("init.generate_default", generateDefaultTemplate).Build(),
				action.New("init.write_atomic", writeFilesAtomic).Build(),
				action.New("init.preview", previewChanges).Build(),
				action.New("init.report_success", reportSuccess).Build(),
			},
		}},
	}
}

func inspectWorkspace(_ context.Context, in map[string]any) (map[string]any, error) {
	out := clone(in)
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	out["cwd"] = cwd

	configPath := config.FindInDir(cwd)
	if configPath == "" {
		configPath = filepath.Join(cwd, config.ConfigFileName)
	}
	ignorePath := filepath.Join(cwd, "nexss.srcpack.ignore")
	out["config_path"] = configPath
	out["config_name"] = filepath.Base(configPath)
	out["ignore_path"] = ignorePath
	_, statErr := os.Stat(configPath)
	out["config_exists"] = statErr == nil
	return out, nil
}

func generateTargets(_ context.Context, in map[string]any) (map[string]any, error) {
	out := clone(in)
	cwd, _ := in["cwd"].(string)
	body, err := buildTargetsStarter(cwd, stringSlice(in["dirs"]))
	if err != nil {
		return nil, err
	}
	out["config_body"] = string(body)
	out["ignore_body"] = string(config.EmbeddedIgnore())
	return out, nil
}

func generateDefaultTemplate(_ context.Context, in map[string]any) (map[string]any, error) {
	out := clone(in)
	out["config_body"] = starterConfig
	out["ignore_body"] = string(config.EmbeddedIgnore())
	return out, nil
}

func cleanGeneratedDir(_ context.Context, in map[string]any) (map[string]any, error) {
	out := clone(in)
	cwd, err := initWorkingDir(in)
	if err != nil {
		return nil, err
	}
	count, err := cleanGeneratedFiles(filepath.Join(cwd, "_srcpack"), false)
	if err != nil {
		return nil, fmt.Errorf("clean _srcpack: %w", err)
	}
	out["cleaned_count"] = count
	if count > 0 {
		fmt.Printf("🧹 Removed %d generated file(s)\n", count)
	}
	return out, nil
}

func previewCleanGeneratedDir(_ context.Context, in map[string]any) (map[string]any, error) {
	out := clone(in)
	cwd, err := initWorkingDir(in)
	if err != nil {
		return nil, err
	}
	count, err := cleanGeneratedFiles(filepath.Join(cwd, "_srcpack"), true)
	if err != nil {
		return nil, fmt.Errorf("clean _srcpack: %w", err)
	}
	out["cleaned_count"] = count
	fmt.Printf("[DRY-RUN] would remove %d generated file(s) from _srcpack/\n", count)
	return out, nil
}

func initWorkingDir(in map[string]any) (string, error) {
	cwd, _ := in["cwd"].(string)
	if cwd != "" {
		return cwd, nil
	}
	return os.Getwd()
}

// cleanGeneratedFiles removes only generated document/temp artifacts. It
// deliberately does not recursively remove the output tree or user files.
func cleanGeneratedFiles(outputDir string, dryRun bool) (int, error) {
	info, err := os.Stat(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("output path %q is not a directory", outputDir)
	}
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	count := 0
	walkErr := filepath.WalkDir(outputDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".tmp") && !strings.HasSuffix(name, ".xml") {
			return nil
		}
		if !dryRun {
			rel, relErr := filepath.Rel(outputDir, path)
			if relErr != nil {
				return relErr
			}
			if removeErr := root.Remove(rel); removeErr != nil {
				return removeErr
			}
		}
		count++
		return nil
	})
	if walkErr != nil {
		return count, walkErr
	}
	// Match the previous behavior where possible, but only remove the root
	// directory when it is empty; any remaining user data keeps it in place.
	if !dryRun && count > 0 {
		_ = os.Remove(outputDir)
	}
	return count, nil
}

func writeFilesAtomic(_ context.Context, in map[string]any) (map[string]any, error) {
	configPath, _ := in["config_path"].(string)
	configBody, _ := in["config_body"].(string)
	ignorePath, _ := in["ignore_path"].(string)
	ignoreBody, _ := in["ignore_body"].(string)
	if configPath == "" || ignorePath == "" {
		return nil, errors.New("init paths are missing; run init.inspect first")
	}
	if err := xfs.WriteFileAtomic(configPath, []byte(configBody), 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", configPath, err)
	}
	fmt.Printf("✓ Wrote %s\n", configPath)

	if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
		if writeErr := xfs.WriteFileAtomic(ignorePath, []byte(ignoreBody), 0o600); writeErr != nil {
			return nil, fmt.Errorf("write %s: %w", ignorePath, writeErr)
		}
		fmt.Printf("✓ Wrote %s\n", ignorePath)
	} else if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", ignorePath, err)
	}
	return in, nil
}

func previewChanges(_ context.Context, in map[string]any) (map[string]any, error) {
	configPath, _ := in["config_path"].(string)
	configBody, _ := in["config_body"].(string)
	ignorePath, _ := in["ignore_path"].(string)
	fmt.Printf("[DRY-RUN] would write %s (%d bytes)\n", configPath, len(configBody))
	if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
		fmt.Printf("[DRY-RUN] would create %s\n", ignorePath)
	} else if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", ignorePath, err)
	}
	return in, nil
}

// Returning a nil payload keeps the transport from rendering an implementation
// map after the human-readable success message.
func reportSuccess(_ context.Context, in map[string]any) (any, error) {
	dryRun, _ := in["dry_run"].(bool)
	if !dryRun {
		fmt.Println("\nNext steps:")
		fmt.Println("  srcpack arch              # pack signatures, copy to clipboard")
		fmt.Println("  srcpack impl -t my-target # pack full source for a named target")
		fmt.Println("  srcpack doctor            # validate the config")
	}
	return nil, nil
}

const starterConfig = `# nexss.srcpack.yml — srcpack configuration.
#
# Options and prompts declared here overlay the built-in defaults.
# Run 'srcpack doctor' after editing to validate.
# Full reference: docs/COMMANDS.md
options:
  strip_comments: true
  remove_body_func: true
  no_tests: true
  no_generated: true
  no_deps: true
# Named targets group directories and files so you can pack one
# subsystem at a time. Reference them with: srcpack <profile> -t <name>
#
# targets:
#   backend:
#     dir:
#       - cmd
#       - internal
#     files:
#       - main.go
#
#   frontend:
#     dir:
#       - web/src
`

func buildTargetsStarter(cwd string, dirs []string) ([]byte, error) {
	type entry struct{ name, path string }
	entries := make([]entry, 0, len(dirs))
	for _, raw := range dirs {
		abs := raw
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, raw)
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("init: %q: %w", raw, err)
		}
		name := strings.TrimSuffix(filepath.ToSlash(raw), "/")
		name = strings.ReplaceAll(name, "/", "_")
		entries = append(entries, entry{name: name, path: filepath.ToSlash(raw)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	var b strings.Builder
	b.WriteString("# nexss.srcpack.yml — generated by 'srcpack init targets'\n\n")
	b.WriteString("options:\n")
	b.WriteString("  strip_comments: true\n")
	b.WriteString("  remove_body_func: true\n")
	b.WriteString("  no_tests: true\n")
	b.WriteString("  no_generated: true\n")
	b.WriteString("  no_deps: true\n\n")
	b.WriteString("targets:\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "  %s:\n    dir:\n      - %s\n", e.name, e.path)
	}
	return []byte(b.String()), nil
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, fmt.Sprint(item))
		}
		return result
	default:
		return nil
	}
}

func clone(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+8)
	maps.Copy(out, in)
	return out
}
