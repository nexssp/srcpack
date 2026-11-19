package packcmd

import (
	"testing"

	"github.com/nexssp/flow/native"
	"github.com/nexssp/flow/runner"
)

func TestBundleDoesNotRegisterDoneAction(t *testing.T) {
	cfg, err := runner.BuildConfig(append(native.Bundles(), Bundle(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if _, found := cfg.Resolver.Action("packcmd.done"); found {
		t.Fatal("packcmd.done must be provided by the typed NFlow pipeline, not a Go action")
	}
}
