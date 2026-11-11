package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	configcmd "github.com/nexssp/srcpack/bundles/config_cli"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return configcmd.Bundle(opts)
}
