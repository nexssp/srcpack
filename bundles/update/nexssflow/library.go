package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	updatecmd "github.com/nexssp/srcpack/bundles/update"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return updatecmd.Bundle(opts)
}
