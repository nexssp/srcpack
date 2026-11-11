package nflows

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	flowcli "github.com/nexssp/flow/cli"
	"github.com/nexssp/flow/compiler"
	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
	"github.com/nexssp/kernel/action"
	tclinexssflow "github.com/nexssp/transport/tcli/nexssflow"

	_ "github.com/nexssp/srcpack/bundles/cliinput"
	_ "github.com/nexssp/srcpack/bundles/config_cli"
	_ "github.com/nexssp/srcpack/bundles/config_views"
	_ "github.com/nexssp/srcpack/bundles/crud"
	_ "github.com/nexssp/srcpack/bundles/document"
	_ "github.com/nexssp/srcpack/bundles/init"
	_ "github.com/nexssp/srcpack/bundles/manage"
	_ "github.com/nexssp/srcpack/bundles/pack"
	_ "github.com/nexssp/srcpack/bundles/system"
	_ "github.com/nexssp/srcpack/bundles/terminal"
	_ "github.com/nexssp/srcpack/bundles/update"
	"github.com/nexssp/srcpack/nexssflow"
)

type acceptanceRecordSourceConfig struct {
	Items []map[string]any `json:"items"`
}

func acceptanceRecordSource() action.AnyStreamAction {
	return action.NewStream("srcpack_test.records", func(_ context.Context, cfg acceptanceRecordSourceConfig) (iter.Seq2[map[string]any, error], error) {
		return func(yield func(map[string]any, error) bool) {
			for _, item := range cfg.Items {
				if !yield(item, nil) {
					return
				}
			}
		}, nil
	})
}

func srcpackAcceptanceBundle() compiler.Bundle {
	bundle := compiler.Bundle{ID: "srcpack"}
	for _, component := range []compiler.Bundle{
		nexssflow.SourceBundle(nil),
		nexssflow.AnalysisBundle(nil),
		nexssflow.RenderBundle(nil),
		nexssflow.IOBundle(nil),
	} {
		bundle.Libraries = append(bundle.Libraries, component.Libraries...)
	}
	return bundle
}

// TestFlowAcceptance runs the checked-in .nflow acceptance fixtures through
// Flow's test-only @test runner, with srcpack's real host bundles mounted.
func TestFlowAcceptance(t *testing.T) {
	fixtureRoot, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "nexss.srcpack.yml"), []byte("views:\n  saved: [src#arch]\n  existing: [elsewhere#source]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "existing"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "dense"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "prompt.txt"), []byte("file prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	bundles := []compiler.Bundle{
		{ID: "srcpack_nflow_test_sources", Libraries: []action.Library{{Name: "srcpack_test", Sources: []action.AnyStreamAction{acceptanceRecordSource()}}}},
		srcpackAcceptanceBundle(),
		tclinexssflow.RoutingOnlyBundle(),
	}
	if code := flowcli.RunWithBundlesForRequirements([]string{"test", fixtureRoot}, bundles, nil); code != 0 {
		t.Fatalf("Flow acceptance runner returned exit code %d", code)
	}
}

// TestFlowFilesystemDownload verifies fs.download through NFlow and Flow's
// standard bundle set, using a local server so the test never depends on the
// public network.
func TestFlowFilesystemDownload(t *testing.T) {
	const payload = "srcpack filesystem download fixture"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asset" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = fmt.Fprint(w, payload)
	}))
	defer server.Close()

	outputDir := t.TempDir()
	cfg, err := runner.BuildConfig(native.Bundles())
	if err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`fs.download @{
  output_dir: %q,
  files: [{ url: %q, path: "assets/payload.txt", expected_bytes: %d }],
  receipt_path: ".download-receipt.json",
  provenance: { project: "srcpack", revision: "integration-test" }
}`, outputDir, server.URL+"/asset", len(payload))
	if _, err := runner.Execute(t.Context(), cfg, source, "fs_download_test.nflow", nil); err != nil {
		t.Fatalf("run fs.download NFlow: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(outputDir, "assets", "payload.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("downloaded payload = %q, want %q", got, payload)
	}
	receipt, err := os.ReadFile(filepath.Join(outputDir, ".download-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"payload.txt", "srcpack", "integration-test"} {
		if !strings.Contains(string(receipt), want) {
			t.Errorf("download receipt %q does not contain %q", receipt, want)
		}
	}
}
