package operators

import "github.com/nexssp/kernel/action"

// SourceLibrary returns the stream source that scans a workspace.
func SourceLibrary() action.Library {
	return action.Library{
		Name:    "srcpack.source",
		Sources: []action.AnyStreamAction{SourceOperator()},
		Actions: GitActions(),
	}
}

// AnalysisLibrary returns the AST extractor and SRCPACK's stats adapters.
// They operate on the file stream produced by srcpack.source and never write
// to disk; pack.batch only materializes records for the downstream Flow fold.
func AnalysisLibrary() action.Library {
	return action.Library{
		Name: "srcpack.analysis",
		Operators: []action.NamedOperator{
			ASTParseOperator(),
			PackStatsInputOperator(),
			PackBatchOperator(),
			StatsCollectOperator(),
		},
	}
}

// RenderLibrary exposes the domain adapter for the document NFlow bundle.
// srcpack.render remains a compatibility operator for external/legacy flows.
func RenderLibrary() action.Library {
	return action.Library{
		Name:      "srcpack.render",
		Operators: []action.NamedOperator{RenderMarkdownOperator()},
		Actions:   []action.AnyAction{JoinRenderedAction(), DocumentInputAction()},
	}
}

// IOLibrary returns the write-side operators: state cache, stdout
// sink, and clipboard.
func IOLibrary() action.Library {
	return ioLibraryWithClipboardWriter(writeSystemClipboard)
}

func ioLibraryWithClipboardWriter(writeClipboard func(string) error) action.Library {
	return action.Library{
		Name: "srcpack.io",
		Operators: []action.NamedOperator{
			SaveStateOperator(),
			StdoutOperator(),
			clipboardOperatorWithWriter(writeClipboard),
		},
		Actions: outputActionsWithClipboardWriter(writeClipboard),
	}
}

// Library returns every srcpack library in one value. Kept for callers
// that want the whole surface without enumerating each subsystem.
func Library() action.Library {
	source := SourceLibrary()
	analysis := AnalysisLibrary()
	render := RenderLibrary()
	io := IOLibrary()

	return action.Library{
		Name:    "srcpack",
		Sources: source.Sources,
		Operators: append(
			append(append([]action.NamedOperator{}, analysis.Operators...), render.Operators...),
			io.Operators...,
		),
		Actions: append(
			append(append([]action.AnyAction{}, render.Actions...), source.Actions...),
			io.Actions...,
		),
	}
}
