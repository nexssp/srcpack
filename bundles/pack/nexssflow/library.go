package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	packcmd "github.com/nexssp/srcpack/bundles/pack"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return packcmd.Bundle(opts)
}
