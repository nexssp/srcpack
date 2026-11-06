// Package bundles embeds the NFlow source files shipped by srcpack's focused bundles.
// It does not register a combined Flow action bundle.
package bundles

import "embed"

// FS contains each bundle's top-level NFlow modules.
//
//go:embed */*.nflow
var FS embed.FS
