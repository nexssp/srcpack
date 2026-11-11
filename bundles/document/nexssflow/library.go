package nexssflow

import (
	"github.com/nexssp/flow/compiler"

	"github.com/nexssp/srcpack/bundles/document"
)

func Bundle(opts map[string]string) compiler.Bundle {
	return document.Bundle(opts)
}
