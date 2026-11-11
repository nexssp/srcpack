package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	configviews "github.com/nexssp/srcpack/bundles/config_views"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return configviews.Bundle(opts)
}
