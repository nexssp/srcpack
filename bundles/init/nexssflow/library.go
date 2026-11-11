package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	initcmd "github.com/nexssp/srcpack/bundles/init"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return initcmd.Bundle(opts)
}
