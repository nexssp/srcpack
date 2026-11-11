package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	managecmd "github.com/nexssp/srcpack/bundles/manage"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return managecmd.Bundle(opts)
}
