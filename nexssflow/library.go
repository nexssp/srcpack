package nexssflow

import (
	"github.com/nexssp/flow/compiler"
	flowtemplate "github.com/nexssp/flow/extensions/template"
	"github.com/nexssp/kernel/action"

	configviews "github.com/nexssp/srcpack/bundles/config_views"
	"github.com/nexssp/srcpack/bundles/document"
	"github.com/nexssp/srcpack/bundles/operators"
	"github.com/nexssp/srcpack/bundles/terminal"
)

// Library returns srcpack's full operator surface as one library for legacy Go hosts.
func Library() action.Library { return operators.Library() }

const (
	SourceBundleID   = "srcpack_source"
	AnalysisBundleID = "srcpack_analysis"
	RenderBundleID   = "srcpack_render"
	IOBundleID       = "srcpack_io"
)

func init() {
	compiler.Register(SourceBundleID, SourceBundle)
	compiler.Register(AnalysisBundleID, AnalysisBundle)
	compiler.Register(RenderBundleID, RenderBundle)
	compiler.Register(IOBundleID, IOBundle)
	// Preserve the aggregate registration for existing external Go/NFlow hosts.
	// Srcpack's own NFlow sources require the focused bundles above.
	compiler.Register("srcpack", Bundle)
}

func SourceBundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: SourceBundleID, Libraries: []action.Library{operators.SourceLibrary()}}
}

func AnalysisBundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: AnalysisBundleID, Libraries: []action.Library{operators.AnalysisLibrary()}}
}

func RenderBundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: RenderBundleID, Libraries: []action.Library{operators.RenderLibrary()}}
}

func IOBundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: IOBundleID, Libraries: []action.Library{operators.IOLibrary()}}
}

// Bundle is a legacy aggregate retained for existing external hosts. New
// NFlow sources should require the focused capability bundles above instead.
func Bundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{ID: "srcpack", Libraries: []action.Library{
		operators.SourceLibrary(), operators.AnalysisLibrary(),
		operators.RenderLibrary(), operators.IOLibrary(),
	}}
}

// FullBundle preserves the composed mounting API for manual Go hosts. NFlow
// sources use Bundle and explicit @require declarations instead.
// FullBundle composes SRCPACK's domain libraries with independent config preset,
// document and terminal bundles. Existing hosts keep their mounting API; each
// bundle directory can be copied without an SRCPACK dependency.
func FullBundle(_ map[string]string) compiler.Bundle {
	return compiler.Bundle{
		ID:      "srcpack",
		Symbols: terminal.Bundle(nil).Symbols,
		Libraries: append(configviews.FullBundle(nil).Libraries, []action.Library{
			// Reuse the same independent bundle libraries, without reimplementing
			// document policy or requiring existing FullBundle hosts to change.
			document.Bundle(nil).Libraries[0],
			terminal.Bundle(nil).Libraries[0],
			flowtemplate.Library(),
			operators.SourceLibrary(),
			operators.AnalysisLibrary(),
			operators.RenderLibrary(),
			operators.IOLibrary(),
		}...),
	}
}
