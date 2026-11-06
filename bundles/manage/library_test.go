package managecmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	tcliflow "github.com/nexssp/transport/tcli/nexssflow"

	"github.com/nexssp/srcpack/bundles/cliinput"
	configviews "github.com/nexssp/srcpack/bundles/config_views"
	terminalfmt "github.com/nexssp/srcpack/bundles/terminal"
)

func TestManageAtomicTargetAndPromptStorage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, configFile)
	if _, err := addTarget(context.Background(), map[string]any{"name": "api", "paths": []string{"src", "test"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := addPrompt(context.Background(), map[string]any{"name": "brief", "text": "Be concise"}); err != nil {
		t.Fatal(err)
	}
	if _, err := addTarget(context.Background(), map[string]any{"name": "api", "paths": []string{"other"}}); err == nil || !strings.Contains(err.Error(), `target "api" already exists`) {
		t.Fatalf("expected duplicate target error, got %v", err)
	}
	if _, err := deletePrompt(context.Background(), map[string]any{"name": "missing"}); err == nil || !strings.Contains(err.Error(), `prompt "missing" not found`) {
		t.Fatalf("expected missing prompt error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "api:") || !strings.Contains(string(data), "brief: Be concise") {
		t.Fatalf("atomic storage contents missing entries:\n%s", data)
	}
	if _, err := deleteTarget(context.Background(), map[string]any{"name": "api"}); err != nil {
		t.Fatal(err)
	}
}

const configFile = "nexss.srcpack.yml"

func TestManagePipelinesExecute(t *testing.T) {
	bundles := append(native.Bundles(), cliinput.Bundle(nil), terminalfmt.Bundle(nil), configviews.FullBundle(nil), tcliflow.RoutingOnlyBundle(), Bundle(nil))
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		t.Fatalf("build flow config: %v", err)
	}
	ctx := compiler.WithSourceLoader(context.Background(), Loader())
	for _, command := range []string{"target", "prompt", "profile", "editor", "view"} {
		source := "@include \"bundles/manage/manage.nflow\"\nmanage.pipeline." + command
		if _, err := runner.Execute(ctx, cfg, source, "manage-flow-test.nflow", map[string]any{"sub": "list", "args": []string{}}); err != nil {
			t.Errorf("execute %s pipeline: %v", command, err)
		}
	}
}

func TestManageNFlowPreflightsMutations(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(configFile, []byte("targets:\n  existing:\n    paths: [src]\nprompts:\n  existing: Saved\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	bundles := append(native.Bundles(), cliinput.Bundle(nil), terminalfmt.Bundle(nil), configviews.FullBundle(nil), tcliflow.RoutingOnlyBundle(), Bundle(nil))
	cfg, err := runner.BuildConfig(bundles)
	if err != nil {
		t.Fatalf("build flow config: %v", err)
	}
	ctx := compiler.WithSourceLoader(context.Background(), Loader())
	tests := []struct {
		command string
		input   map[string]any
		wantErr string
	}{
		{"target", map[string]any{"sub": "add", "args": []string{"existing", "other"}}, `target "existing" already exists`},
		{"target", map[string]any{"sub": "delete", "args": []string{"missing"}}, `target "missing" not found`},
		{"prompt", map[string]any{"sub": "add", "args": []string{"existing", "again"}}, `prompt "existing" already exists`},
		{"prompt", map[string]any{"sub": "delete", "args": []string{"missing"}}, `prompt "missing" not found`},
	}
	for _, test := range tests {
		t.Run(test.command+"/"+test.wantErr, func(t *testing.T) {
			source := "@include \"bundles/manage/manage.nflow\"\nmanage.pipeline." + test.command
			if _, err := runner.Execute(ctx, cfg, source, "manage-preflight-test.nflow", test.input); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected NFlow error containing %q, got %v", test.wantErr, err)
			}
		})
	}
	if _, err := runner.Execute(ctx, cfg, "@include \"bundles/manage/manage.nflow\"\nmanage.pipeline.target", "manage-default-test.nflow", map[string]any{}); err != nil {
		t.Fatalf("target pipeline should default an omitted subcommand to list: %v", err)
	}
}
