package packcmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nexssp/srcpack/bundles/config"
	remote "github.com/nexssp/srcpack/bundles/github_source"
	"github.com/nexssp/srcpack/bundles/operators"
)

func TestRemoteSelectorGroup(t *testing.T) {
	got := parseSelector("github:nexssp/flow,nexssp/kernel#sources")
	want := selector{Targets: []string{"github:nexssp/flow", "github:nexssp/kernel"}, Profile: "source"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	got = parseSelector("github:nexssp/flow//compiler?ref=feature%2Fbranch~*_test.go#arch@explain, please")
	want = selector{Targets: []string{"github:nexssp/flow//compiler?ref=feature%2Fbranch"}, Excludes: []string{"*_test.go"}, Profile: "arch", Prompt: "explain, please"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if key := selectorTargetKey(got.Targets[0], "/workspace"); key != got.Targets[0] {
		t.Fatalf("locator became local path: %s", key)
	}
	if got := config.CanonicalProfileName("sources"); got != "source" {
		t.Fatalf("alias: %s", got)
	}
}

func TestRemoteResolveMixedTargetsWithoutNetwork(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("folder", 0o700); err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"selectors": []string{"github:nexssp/flow,nexssp/kernel#sources", "folder#arch"}}
	got, err := ResolveInput(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	var cfg operators.PackConfig
	if err := decode(got["pack"], &cfg); err != nil {
		t.Fatal(err)
	}
	want := []string{"github:nexssp/flow", "github:nexssp/kernel", "folder"}
	if !reflect.DeepEqual(cfg.Targets, want) {
		t.Fatalf("targets: %#v", cfg.Targets)
	}
	if cfg.TargetProfiles[want[0]] != "source" || cfg.TargetProfiles[want[1]] != "source" || cfg.TargetProfiles[selectorTargetKey("folder", "")] != "arch" {
		t.Fatalf("profiles: %#v", cfg.TargetProfiles)
	}
}

func TestRemoteViewRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	raw := "github:nexssp/flow,nexssp/kernel//action?ref=refs%2Ftags%2Fv0.35.0~*_test.go#sources@explain, please"
	got, err := viewRequestAction(context.Background(), map[string]any{"selectors": []string{raw, "folder#arch"}, "save_view": "remote"})
	if err != nil {
		t.Fatal(err)
	}
	saved, ok := got["value"].([]string)
	if !ok || len(saved) != 2 {
		t.Fatalf("saved value has type %T, want two strings", got["value"])
	}
	wantRemote := "github:nexssp/flow,github:nexssp/kernel//action?ref=refs%2Ftags%2Fv0.35.0~*_test.go#source@explain, please"
	if saved[0] != wantRemote {
		t.Fatalf("saved remote: %s", saved[0])
	}
	if restored := resolveViewSelector(saved[0], root); restored != wantRemote {
		t.Fatalf("replayed remote: %s", restored)
	}
	if restored := resolveViewSelector(saved[1], root); !strings.HasPrefix(restored, filepath.Join(root, "folder")) {
		t.Fatalf("local replay: %s", restored)
	}
}

func TestRemoteMaterializePreservesMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "compiler")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := "github:nexssp/flow//compiler?ref=v0.35.0"
	commit := strings.Repeat("a", 40)
	ctx, cleanup := WithRemoteSession(context.Background())
	defer cleanup()
	session, ok := ctx.Value(remoteSessionKey{}).(*remoteSession)
	if !ok {
		t.Fatal("remote session was not attached to context")
	}
	session.results[raw] = remote.Result{Path: path, Root: root, Repository: "https://github.com/nexssp/flow.git", Commit: commit}
	cfg := operators.PackConfig{Targets: []string{raw, "local"}, TargetProfiles: map[string]string{raw: "arch"}, TargetPrompts: map[string]string{raw: "explain"}, TargetExcludes: map[string]string{raw: "*_test.go"}}
	in := map[string]any{"pack": cfg, "render": map[string]any{"dirs": raw}, "io": map[string]any{"quiet": true}}
	got, err := materializeInput(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var out operators.PackConfig
	if err := decode(got["pack"], &out); err != nil {
		t.Fatal(err)
	}
	if out.Targets[0] != path || out.Targets[1] != "local" || out.TargetProfiles[path] != "arch" || out.TargetPrompts[path] != "explain" || out.TargetExcludes[path] != "*_test.go" {
		t.Fatalf("materialized: %#v", out)
	}
	if out.TargetRoots[path] != root || out.TargetLabels[path] != "github/nexssp/flow@"+commit {
		t.Fatalf("roots/labels: %#v", out)
	}
	if got["remote_sources"] == nil {
		t.Fatal("missing provenance")
	}
	render, ok := got["render"].(map[string]any)
	if !ok {
		t.Fatalf("render settings have type %T, want map[string]any", got["render"])
	}
	if render["dirs"] != raw {
		t.Fatal("render label became a temporary path")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.get(ctx, raw); err == nil {
		t.Fatal("closed session accepted a request")
	}
}

func TestRemoteSessionRequiredAndLocalPassThrough(t *testing.T) {
	local := map[string]any{"pack": operators.PackConfig{Targets: []string{"."}}}
	if got, err := materializeInput(context.Background(), local); err != nil || !reflect.DeepEqual(got, local) {
		t.Fatalf("local: %v %v", got, err)
	}
	_, err := materializeInput(context.Background(), map[string]any{"pack": operators.PackConfig{Targets: []string{"github:nexssp/flow"}}})
	if err == nil || !strings.Contains(err.Error(), "WithRemoteSession") {
		t.Fatalf("missing scope error: %v", err)
	}
}

func TestRemoteLocalGitFlagsRejectedBeforeIO(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, flag := range []string{"changed", "modified", "staged", "since", "diff_only", "full_tree"} {
		in := map[string]any{"selectors": []string{"github:nexssp/flow"}, flag: true}
		if flag == "since" {
			in[flag] = "HEAD~1"
		}
		_, err := ResolveInput(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "remote sources") {
			t.Fatalf("%s: %v", flag, err)
		}
	}
}

func TestRemoteConfiguredTargetExpansion(t *testing.T) {
	cfg := config.Config{TargetsMap: map[string]config.TargetGroup{"deps": {Paths: []string{"github:nexssp/flow//compiler?ref=v0.35.0"}, Files: []string{"github:nexssp/kernel//go.mod"}}}}
	paths, files, err := expandTargets([]string{"deps"}, cfg)
	if err != nil || len(paths) != 1 || len(files) != 1 || !remote.IsRemote(paths[0]) || !remote.IsRemote(files[0]) {
		t.Fatalf("%v %v %v", paths, files, err)
	}
}
