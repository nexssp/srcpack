package packcmd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	remote "github.com/nexssp/srcpack/bundles/github_source"
	"github.com/nexssp/srcpack/bundles/operators"
)

type remoteSessionKey struct{}

type remoteSession struct {
	mu      sync.Mutex
	results map[string]remote.Result
	closed  bool
}

// WithRemoteSession scopes downloaded source trees to one runner invocation.
// The host must defer the returned cleanup on both success and failure.
// Keeping this in the host also covers errors in downstream parsing/rendering.
func WithRemoteSession(ctx context.Context) (context.Context, func() error) {
	s := &remoteSession{results: make(map[string]remote.Result)}
	return context.WithValue(ctx, remoteSessionKey{}, s), s.close
}

func (s *remoteSession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var first error
	for key, result := range s.results {
		if err := result.Close(); err != nil && first == nil {
			first = err
		}
		delete(s.results, key)
	}
	return first
}

func (s *remoteSession) get(ctx context.Context, raw string) (remote.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return remote.Result{}, errors.New("remote source session is closed")
	}
	if result, ok := s.results[raw]; ok {
		return result, nil
	}
	result, err := remote.Materialize(ctx, raw)
	if err != nil {
		return remote.Result{}, err
	}
	s.results[raw] = result
	return result, nil
}

// materializeInput runs only in pack_execute, never while saving a view.
// It accepts/reserializes the typed pack configuration rather than re-parsing
// command-line flags. Source selectors remain application-level data.
func materializeInput(ctx context.Context, in map[string]any) (map[string]any, error) {
	var cfg operators.PackConfig
	if err := decode(in["pack"], &cfg); err != nil {
		return nil, fmt.Errorf("materialize pack config: %w", err)
	}
	if !hasRemoteTargets(cfg.Targets, cfg.Files) {
		return in, nil
	}
	if err := validateRemoteOptions(cfg); err != nil {
		return nil, err
	}
	if stats, ok := in["stats"].(map[string]any); ok && boolField(stats, "full_tree") {
		return nil, errors.New("remote sources do not support --full-tree; use --tree to show the selected source tree")
	}
	session, ok := ctx.Value(remoteSessionKey{}).(*remoteSession)
	if !ok {
		return nil, errors.New("remote sources require a scoped session: wrap the runner context with packcmd.WithRemoteSession and defer its cleanup")
	}
	if cfg.TargetRoots == nil {
		cfg.TargetRoots = make(map[string]string)
	}
	if cfg.TargetLabels == nil {
		cfg.TargetLabels = make(map[string]string)
	}
	var provenance []any
	materialize := func(items []string) error {
		for i, raw := range items {
			if !remote.IsRemote(raw) {
				continue
			}
			result, err := session.get(ctx, raw)
			if err != nil {
				return fmt.Errorf("source %q: %w", raw, err)
			}
			spec, err := remote.Parse(raw)
			if err != nil {
				return err
			}
			items[i] = result.Path
			cfg.TargetRoots[result.Path] = result.Root
			cfg.TargetLabels[result.Path] = "github/" + spec.Repo + "@" + result.Commit
			for _, values := range []map[string]string{cfg.TargetProfiles, cfg.TargetExcludes, cfg.TargetPrompts} {
				if value, ok := values[raw]; ok {
					values[result.Path] = value
					delete(values, raw)
				}
			}
			provenance = append(provenance, map[string]any{"source": raw, "repository": result.Repository, "commit": result.Commit})
		}
		return nil
	}
	if err := materialize(cfg.Targets); err != nil {
		return nil, err
	}
	if err := materialize(cfg.Files); err != nil {
		return nil, err
	}
	pack, err := jsonObject(cfg)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(in)+1)
	maps.Copy(out, in)
	out["pack"] = pack
	out["remote_sources"] = provenance
	return out, nil
}

func hasRemoteTargets(groups ...[]string) bool {
	for _, group := range groups {
		if slices.ContainsFunc(group, remote.IsRemote) {
			return true
		}
	}
	return false
}

func validateRemoteOptions(cfg operators.PackConfig) error {
	if cfg.Changed || cfg.Modified || cfg.Staged || cfg.Since != "" || cfg.DiffOnly {
		return errors.New("remote sources cannot be combined with --changed, --modified, --staged, --since or --diff-only; these flags describe the local workspace")
	}
	return validateRemoteSpecs(cfg.Targets, cfg.Files)
}

// validateRemoteSpecs checks locator grammar only. The NFlow resolver chooses
// valid flag combinations; the materialization adapter repeats its invariants
// before performing any network I/O, including for direct Go callers.
func validateRemoteSpecs(groups ...[]string) error {
	// Selectors reserve #, ~ and @ for app-level syntax. Ref/path values
	// containing those characters must use percent escapes in the remote spec.
	for _, group := range groups {
		for _, raw := range group {
			if remote.IsRemote(raw) {
				if strings.ContainsAny(raw, "#~@") {
					return fmt.Errorf("remote source %q contains an unescaped selector delimiter", raw)
				}
				if _, err := remote.Parse(raw); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
