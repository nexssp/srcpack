package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	systemcmd "github.com/nexssp/srcpack/bundles/system"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return systemcmd.Bundle(opts)
}
