package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	"github.com/nexssp/srcpack/bundles/terminal"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return terminal.Bundle(opts)
}
