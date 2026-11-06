package main

import (
	"context"
	"errors"
	"fmt"

	configviews "github.com/nexssp/srcpack/bundles/config_views"
)

// legacyViewOperation is deliberately only a storage adapter. Selector parsing,
// normalization and positional expansion stay with srcpack. The explicit replace
// policy preserves the historical --save-view overwrite behavior; new bundle
// callers must choose their own policy.
func legacyViewOperation(ctx context.Context, operation, name string, value any) (map[string]any, error) {
	result, err := configviews.Execute(ctx, map[string]any{
		"operation": operation, "path": configPath(), "name": name,
		"value": value, "replace": true, "legacy": true,
	})
	if errors.Is(err, configviews.ErrNotFound) {
		return nil, fmt.Errorf("view %q not found", name)
	}
	return result, err
}
