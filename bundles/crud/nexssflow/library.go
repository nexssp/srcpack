package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	"github.com/nexssp/srcpack/bundles/crud"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return crud.Bundle(opts)
}
