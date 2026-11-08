package operators

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

func extractGoSymbolSpans(filename string, source []byte, exportedOnly bool, baseLine int) []SymbolSpan {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, source, parser.ParseComments)
	if err != nil {
		return nil
	}
	if baseLine < 1 {
		baseLine = 1
	}

	var spans []SymbolSpan
	for _, decl := range file.Decls {
		spans = append(spans, extractGoDeclSpans(fset, file, decl, exportedOnly, baseLine)...)
	}
	return spans
}

func extractGoDeclSpans(
	fset *token.FileSet,
	file *ast.File,
	decl ast.Decl,
	exportedOnly bool,
	baseLine int,
) []SymbolSpan {
	switch decl := decl.(type) {
	case *ast.GenDecl:
		return extractGoGenDeclSpans(fset, decl, exportedOnly, baseLine)
	case *ast.FuncDecl:
		span, ok := extractGoFuncSpan(fset, file.Name.Name, decl, exportedOnly, baseLine)
		if ok {
			return []SymbolSpan{span}
		}
	}
	return nil
}

func extractGoGenDeclSpans(fset *token.FileSet, decl *ast.GenDecl, exportedOnly bool, baseLine int) []SymbolSpan {
	if decl.Tok == token.IMPORT {
		return nil
	}
	var spans []SymbolSpan
	for _, spec := range decl.Specs {
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			if !exportedOnly || ast.IsExported(spec.Name.Name) {
				if span, ok := newSymbolSpan(fset, spec.Name.Name, "type", "", spec.Pos(), spec.End(), baseLine); ok {
					spans = append(spans, span)
				}
			}
		case *ast.ValueSpec:
			for _, name := range spec.Names {
				if exportedOnly && !ast.IsExported(name.Name) {
					continue
				}
				if span, ok := newSymbolSpan(fset, name.Name, decl.Tok.String(), "", spec.Pos(), spec.End(), baseLine); ok {
					spans = append(spans, span)
				}
			}
		}
	}
	return spans
}

func extractGoFuncSpan(
	fset *token.FileSet,
	packageName string,
	decl *ast.FuncDecl,
	exportedOnly bool,
	baseLine int,
) (SymbolSpan, bool) {
	isMain := packageName == "main" && decl.Name.Name == "main"
	if exportedOnly && !ast.IsExported(decl.Name.Name) && !isMain {
		return SymbolSpan{}, false
	}
	kind, receiver := "func", ""
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		kind = "method"
		var b bytes.Buffer
		if err := format.Node(&b, fset, decl.Recv.List[0].Type); err == nil {
			receiver = b.String()
		}
	}
	return newSymbolSpan(fset, decl.Name.Name, kind, receiver, decl.Type.Func, decl.End(), baseLine)
}

func newSymbolSpan(
	fset *token.FileSet,
	name, kind, receiver string,
	start, end token.Pos,
	baseLine int,
) (SymbolSpan, bool) {
	startLine := fset.PositionFor(start, false).Line + baseLine - 1
	endLine := fset.PositionFor(end, false).Line + baseLine - 1
	if name == "" || startLine < 1 || endLine < startLine {
		return SymbolSpan{}, false
	}
	return SymbolSpan{
		Name: name, Kind: kind, Receiver: receiver,
		StartLine: startLine, EndLine: endLine,
	}, true
}
