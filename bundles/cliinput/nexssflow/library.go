package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	srcpackcliinput "github.com/nexssp/srcpack/bundles/cliinput"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return srcpackcliinput.Bundle(opts)
}
