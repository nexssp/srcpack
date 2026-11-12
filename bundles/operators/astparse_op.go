package operators

import (
	"bytes"
	"iter"
	"strings"
	"sync"

	"github.com/nexssp/kernel/action"

	"github.com/nexssp/lang/extract"
)

type ASTConfig struct {
	OnlySig        bool `json:"only_sig"         cli:"only-sig"`
	Dense          bool `json:"dense"            cli:"dense"`
	Comments       bool `json:"comments"         cli:"comments"`
	NeedFuncCount  bool `json:"need_func_count"`
	Mask           bool `json:"mask"             cli:"mask"`
	IncludeConfig  bool `json:"include_config"   cli:"include-config"`
	MaxLines       int  `json:"max_lines"        cli:"max-lines"`
	MaxLinesHead   int  `json:"max_lines_head"`
	MaxLinesTail   int  `json:"max_lines_tail"`
	ExportedOnly   bool `json:"exported_only"`
	TrimIndent     bool `json:"trim_indent"`
	TrimBlankLines bool `json:"trim_blank_lines"`
}

var maskBufPool = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, 16*1024)) },
}

//nolint:gocyclo // per-file parse with distinct recovery paths; extraction adds params without clarity.
func ASTParse(cfg ASTConfig) action.StreamOp[ParsedFile, ParsedFile] {
	return func(up iter.Seq2[ParsedFile, error]) iter.Seq2[ParsedFile, error] {
		return func(yield func(ParsedFile, error) bool) {
			for file, err := range up {
				if err != nil {
					yield(ParsedFile{}, err)
					return
				}
				if file.Omitted {
					continue
				}

				fileOptions := ParseOptions(cfg)
				if file.ParseOptionsSet {
					fileOptions = file.ParseOptions
				}
				if file.SourceStartLine < 1 {
					file.SourceStartLine = 1
				}
				if file.SourceEndLine < file.SourceStartLine {
					file.SourceEndLine = file.SourceStartLine + sourceLineCount(file.Content) - 1
				}
				if file.Lines < 1 {
					file.Lines = bytes.Count(file.Content, []byte{'\n'}) + 1
				}

				if fileOptions.IncludeConfig && isConfigContextFile(sourceContextPath(file)) {
					sourceContent := file.Content
					if fileOptions.Mask {
						file.Content = maskSourceBytes(file.Content)
					}
					file.LineSpans = sourceLineSpans(sourceContent, file.Content, file.SourceStartLine, true)
					file.Snippets = nil
					file.FuncCount = 0
					file.ParseOptions = fileOptions
					file.ParseOptionsSet = true
					if !yield(file, nil) {
						return
					}
					continue
				}

				passThrough := !fileOptions.OnlySig &&
					!fileOptions.Dense &&
					fileOptions.Comments &&
					!fileOptions.Mask &&
					fileOptions.MaxLines <= 0 &&
					!fileOptions.TrimIndent &&
					!fileOptions.TrimBlankLines &&
					!fileOptions.ExportedOnly
				if passThrough || len(file.Content) == 0 {
					file.LineSpans = sourceLineSpans(file.Content, file.Content, file.SourceStartLine, true)
					if passThrough && len(file.Content) > 0 && file.FuncCount == 0 && (cfg.NeedFuncCount || fileOptions.NeedFuncCount) {
						// Parse only for metadata; retain the original bytes on this fast path.
						if result, parseErr := extract.Parse(file.Path, file.Content, file.Lang, extract.Options{}); parseErr == nil {
							file.FuncCount = result.FuncCount
						}
					}
					if !yield(file, nil) {
						return
					}
					continue
				}

				opts := extract.Options{
					RemoveBodyFunc: fileOptions.OnlySig,
					StripComments:  !fileOptions.Comments,
					MaxLines:       fileOptions.MaxLines,
					MaxLinesHead:   fileOptions.MaxLinesHead,
					MaxLinesTail:   fileOptions.MaxLinesTail,
					ExportedOnly:   fileOptions.ExportedOnly,
					TrimIndent:     fileOptions.TrimIndent,
					TrimBlankLines: fileOptions.TrimBlankLines,
				}

				res, parseErr := extract.Parse(file.Path, file.Content, file.Lang, opts)
				if parseErr != nil {
					// Parser failed: unsupported language, malformed source.
					// Fall back to raw content instead of silently dropping
					// the file from the pack.
					res = extract.Result{Content: string(file.Content)}
				}
				if parseErr == nil && fileOptions.OnlySig && file.Lang == "python" {
					res.Snippets = normalizePythonSignatures(res.Snippets)
				}
				var symbolSpans []SymbolSpan
				if fileOptions.OnlySig && file.Lang == "go" {
					symbolSpans = extractGoSymbolSpans(file.Path, file.Content, fileOptions.ExportedOnly, file.SourceStartLine)
				}

				var finalContent []byte
				var finalSnippets []string

				if fileOptions.OnlySig {
					switch {
					case len(res.Snippets) > 0:
						if fileOptions.Dense {
							finalContent = []byte(DensifySnippets(file.Lang, res.Snippets))
						} else {
							finalSnippets = res.Snippets
						}
					case res.Content != "":
						finalContent = []byte(res.Content)
					case fileOptions.MaxLines > 0 && bytes.Count(file.Content, []byte{'\n'}) >= fileOptions.MaxLines:
						finalContent = []byte(extract.Truncate(file.Content, opts, nil))
					default:
						// Signature extraction yielded nothing and the
						// parser returned no content — the file has no
						// exported symbols, or the parser does not
						// support its language. Drop the file. Do NOT
						// fall back to full source; that silently leaks
						// bodies into a signature pack.
						finalContent = nil
					}
				} else {
					var text string
					if opts.MaxLines > 0 && bytes.Count(file.Content, []byte{'\n'})+1 > opts.MaxLines {
						text = extract.Truncate(file.Content, opts, nil)
					} else {
						text = extract.ProcessContent(string(file.Content), opts)
					}
					if fileOptions.Dense {
						text = DensifyBody(text)
					}
					finalContent = []byte(text)
				}

				if fileOptions.Mask {
					buf, ok := maskBufPool.Get().(*bytes.Buffer)
					if !ok {
						buf = bytes.NewBuffer(make([]byte, 0, 16*1024))
					}
					buf.Reset()

					if len(finalContent) > 0 {
						MaskSecretsFast(finalContent, buf)
						finalContent = bytes.Clone(buf.Bytes())
						buf.Reset()
					}

					if len(finalSnippets) > 0 {
						for i, snip := range finalSnippets {
							MaskSecretsFast([]byte(snip), buf)
							finalSnippets[i] = buf.String()
							buf.Reset()
						}
					}

					maskBufPool.Put(buf)
				}

				var lineSpans []SourceLineSpan
				if fileOptions.OnlySig && len(symbolSpans) > 0 {
					if fileOptions.Dense {
						lineSpans = sourceLineSpansForSnippets(res.Snippets, symbolSpans, file.Lang, true)
					} else {
						lineSpans = sourceLineSpansForSnippets(finalSnippets, symbolSpans, file.Lang, false)
					}
				} else {
					preservesPositions := !fileOptions.OnlySig && !fileOptions.Dense && fileOptions.Comments &&
						fileOptions.MaxLines <= 0 && !fileOptions.TrimIndent && !fileOptions.TrimBlankLines
					lineSpans = sourceLineSpans(file.Content, finalContent, file.SourceStartLine, preservesPositions)
				}

				item := ParsedFile{
					FileMeta:        file.FileMeta,
					Content:         finalContent,
					Snippets:        finalSnippets,
					FuncCount:       res.FuncCount,
					Lines:           bytes.Count(file.Content, []byte{'\n'}) + 1,
					ExpandTree:      file.ExpandTree,
					SourceStartLine: file.SourceStartLine,
					SourceEndLine:   file.SourceEndLine,
					SymbolSpans:     symbolSpans,
					LineSpans:       lineSpans,
					Hash:            file.Hash,
					Profile:         file.Profile,
					Prompt:          file.Prompt,
					ParseOptions:    fileOptions,
					ParseOptionsSet: true,
				}

				if !yield(item, nil) {
					return
				}
			}
		}
	}
}

func normalizePythonSignatures(snippets []string) []string {
	normalized := make([]string, len(snippets))
	for i, snippet := range snippets {
		lines := strings.Split(snippet, "\n")
		for j, line := range lines {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasSuffix(trimmed, " {}"):
				lines[j] = indent + strings.TrimSuffix(trimmed, " {}") + " ..."
			case strings.HasSuffix(trimmed, "::"):
				lines[j] = indent + strings.TrimSuffix(trimmed, ":")
			}
		}
		normalized[i] = strings.Join(lines, "\n")
	}
	return normalized
}

func maskSourceBytes(source []byte) []byte {
	buf, ok := maskBufPool.Get().(*bytes.Buffer)
	if !ok {
		buf = bytes.NewBuffer(make([]byte, 0, 16*1024))
	}
	buf.Reset()
	MaskSecretsFast(source, buf)
	masked := bytes.Clone(buf.Bytes())
	buf.Reset()
	maskBufPool.Put(buf)
	return masked
}

func ASTParseOperator() action.NamedOperator {
	return action.NewOperator("ast.parse", ASTParse)
}
