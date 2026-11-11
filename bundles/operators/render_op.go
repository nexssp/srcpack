package operators

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strconv"
	"strings"

	"github.com/nexssp/flow/extensions/fs"
	"github.com/nexssp/kernel/action"
)

// RenderConfig controls srcpack.render. Fields with no JSON tag left
// in the struct are intentionally absent: this operator receives its
// values from CLI modifiers, and modifier values are strings. Bool
// fields use bare modifiers (:no_header); string fields use :k="v".
type RenderConfig struct {
	Editor string `json:"editor"`
	Prompt string `json:"prompt"`

	Start     string `json:"start"`
	End       string `json:"end"`
	FileStart string `json:"file_start"`
	FileEnd   string `json:"file_end"`

	ProfileName string `json:"profile_name"`
	Targets     string `json:"targets"`
	Dirs        string `json:"dirs"`
	OutputFile  string `json:"output_file"`
	PromptName  string `json:"prompt_name"`
	GitCommit   string `json:"git_commit"`
	GitTag      string `json:"git_tag"`
	GitBranch   string `json:"git_branch"`
	CurrentDate string `json:"current_date"`
	Directory   string `json:"directory"`
	DiffContent string `json:"diff_content"`

	NoHeader  bool   `json:"no_header"`
	NoPrompt  bool   `json:"no_prompt"`
	NoStart   bool   `json:"no_start"`
	NoEnd     bool   `json:"no_end"`
	Tree      bool   `json:"tree"`
	FullTree  bool   `json:"full_tree"`
	TreeFiles bool   `json:"tree_files"`
	TreeDepth string `json:"tree_depth"`
}

// renderConfiguredTree is shared by the legacy renderer and document adapter.
// Generators resolve omitted or invalid depth through config.ResolveTreeDepth.
func renderConfiguredTree(cfg RenderConfig, files []ParsedFile) (string, error) {
	return renderConfiguredTreeContext(context.Background(), cfg, files)
}

func renderConfiguredTreeContext(ctx context.Context, cfg RenderConfig, files []ParsedFile) (string, error) {
	depth, err := strconv.Atoi(cfg.TreeDepth)
	if err != nil {
		depth = 0
	}
	switch {
	case cfg.FullTree:
		return generateFullProjectTreeContext(ctx, cfg.Directory, files, depth, false, cfg.TreeFiles)
	case cfg.Tree:
		return generateContextTreeContext(ctx, files, depth, false, cfg.TreeFiles)
	default:
		return "", nil
	}
}

func RenderMarkdown(cfg RenderConfig) action.StreamOp[any, fs.FileContent] {
	editor := canonicalEditor(cfg.Editor)

	return func(up iter.Seq2[any, error]) iter.Seq2[fs.FileContent, error] {
		return func(yield func(fs.FileContent, error) bool) {
			var records []any
			for record, err := range up {
				if err != nil {
					yield(fs.FileContent{}, err)
					return
				}
				records = append(records, record)
			}

			files, stats, err := collectRenderRecords(records)
			if err != nil {
				yield(fs.FileContent{}, err)
				return
			}
			emitRenderedMarkdown(cfg, editor, files, stats, yield)
		}
	}
}

// collectRenderRecords preserves the old stream/envelope input contract while
// allowing the production NFlow formatter to share only the domain adaptation.
func collectRenderRecords(records []any) ([]ParsedFile, PackStats, error) {
	if len(records) > 0 && isPackStatsEnvelope(records[0]) {
		if len(records) != 1 {
			return nil, PackStats{}, fmt.Errorf("srcpack.render expected one NFlow pack envelope, got %d items", len(records))
		}
		envelope, err := unpackPackStatsEnvelope(records[0])
		return envelope.Files, envelope.Stats, err
	}
	files, err := parsedFilesFromRecords(records)
	if err != nil {
		return nil, PackStats{}, err
	}
	stats, err := foldPackStats(files)
	return files, stats, err
}

//nolint:gocyclo // ordered emission of header/tree/files/footer; each branch toggles one section.
func emitRenderedMarkdown(
	cfg RenderConfig,
	editor string,
	files []ParsedFile,
	stats PackStats,
	yield func(fs.FileContent, error) bool,
) {
	tree, err := renderConfiguredTree(cfg, files)
	if err != nil {
		yield(fs.FileContent{}, err)
		return
	}
	lookup := makeRenderLookup(cfg, stats, tree)

	if cfg.DiffContent != "" {
		block := "## Git Unified Diff\n```diff\n" + cfg.DiffContent + "\n```\n\n"
		if !yield(fs.FileContent{Content: []byte(block)}, nil) {
			return
		}
	}

	if !cfg.NoHeader && !cfg.NoPrompt && cfg.Prompt != "" {
		block := "> " + strings.ReplaceAll(strings.TrimSpace(cfg.Prompt), "\n", "\n> ") + "\n\n"
		if !yield(fs.FileContent{Content: []byte(block)}, nil) {
			return
		}
	}

	if tree != "" && !strings.Contains(cfg.Start, "${TREE}") {
		if !yield(fs.FileContent{Content: []byte(tree)}, nil) {
			return
		}
	}

	if !cfg.NoHeader && !cfg.NoStart {
		start := cfg.Start
		if start == "" {
			start = defaultDocStart(editor)
		}
		if start != "" {
			expanded := expandPlaceholders(start, lookup)
			if !yield(fs.FileContent{Content: []byte(expanded)}, nil) {
				return
			}
		}
	}

	fileStart := cfg.FileStart
	if fileStart == "" {
		fileStart = defaultFileStart(editor)
	}
	fileEnd := cfg.FileEnd
	if fileEnd == "" {
		fileEnd = defaultFileEnd(editor)
	}

	for i := range files {
		file := &files[i]
		if file.Omitted {
			continue
		}
		if len(file.Content) == 0 && len(file.Snippets) == 0 {
			continue
		}
		block := renderFileBlock(file, fileStart, fileEnd)
		if !yield(fs.FileContent{FileMeta: file.FileMeta, Content: block}, nil) {
			return
		}
	}

	if !cfg.NoHeader && !cfg.NoEnd {
		end := cfg.End
		if end == "" {
			end = defaultDocEnd(editor)
		}
		if end != "" {
			expanded := expandPlaceholders(end, lookup)
			if !yield(fs.FileContent{Content: []byte(expanded)}, nil) {
				return
			}
		}
	}
}

func RenderMarkdownOperator() action.NamedOperator {
	return action.NewOperator("srcpack.render", RenderMarkdown)
}

// ── editor presets ───────────────────────────────────────────────────

func canonicalEditor(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "md", editorMarkdown:
		return editorMarkdown
	case editorZed:
		return editorZed
	case editorClaude:
		return editorClaude
	case editorFold, "details":
		return editorFold
	case editorRaw, "none", "text":
		return editorRaw
	default:
		return editorMarkdown
	}
}

func defaultDocStart(editor string) string {
	if editor == "fold" {
		return "<details>\n<summary>📦 Packed Context</summary>\n"
	}
	return ""
}

func defaultDocEnd(editor string) string {
	if editor == "fold" {
		return "</details>\n"
	}
	return ""
}

func defaultFileStart(editor string) string {
	switch editor {
	case "zed":
		return "```${LANG} ${REL_PATH}\n"
	case "claude":
		return "<file path=\"${REL_PATH}\" language=\"${LANG}\">\n```${LANG}\n"
	case "fold":
		return "<details>\n<summary>${REL_PATH}</summary>\n\n```${LANG}\n"
	case "raw":
		return ""
	default:
		return "### ${REL_PATH} (${LANG})\n```${LANG}\n"
	}
}

func defaultFileEnd(editor string) string {
	switch editor {
	case "claude":
		return "```\n</file>\n\n"
	case "fold":
		return "```\n</details>\n\n"
	case "raw":
		return ""
	default:
		return "```\n\n"
	}
}

// ── file block rendering ─────────────────────────────────────────────

func renderFileBlock(file *ParsedFile, fileStart, fileEnd string) []byte {
	var buf bytes.Buffer
	if metadata := sourceLocationComment(file); len(metadata) > 0 {
		buf.Write(metadata)
	}

	header := expandPlaceholders(fileStart, fileLookup(file))
	buf.WriteString(header)

	// The header may end without a newline (e.g. the default markdown
	// file_start ends with "```${LANG}"). Force one so the first line
	// of content does not merge into the opening fence.
	if buf.Len() > 0 && buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}

	if len(file.Content) > 0 {
		buf.Write(file.Content)
		if buf.Len() > 0 && buf.Bytes()[buf.Len()-1] != '\n' {
			buf.WriteByte('\n')
		}
	} else if len(file.Snippets) > 0 {
		writeSnippets(&buf, file.Snippets)
	}

	footer := expandPlaceholders(fileEnd, fileLookup(file))
	buf.WriteString(footer)

	return buf.Bytes()
}

func sourceLocationComment(file *ParsedFile) []byte {
	if file.SourceStartLine < 1 || file.SourceEndLine < file.SourceStartLine {
		return nil
	}
	path := file.RelPath
	if path == "" {
		path = file.Path
	}
	if path == "" {
		return nil
	}
	metadata, err := json.Marshal(struct {
		Path      string           `json:"path"`
		StartLine int              `json:"start_line"`
		EndLine   int              `json:"end_line"`
		Symbols   []SymbolSpan     `json:"symbols,omitempty"`
		LineSpans []SourceLineSpan `json:"line_spans,omitempty"`
	}{
		Path: path, StartLine: file.SourceStartLine, EndLine: file.SourceEndLine,
		Symbols: file.SymbolSpans, LineSpans: file.LineSpans,
	})
	if err != nil {
		return nil
	}
	result := append([]byte("<!-- srcpack-source: "), metadata...)
	return append(result, []byte(" -->\n")...)
}

// writeSnippets packs consecutive single-line declarations tightly and
// separates multi-line blocks with a blank line. Mirrors legacy
// FormatSnippets.
func writeSnippets(buf *bytes.Buffer, snippets []string) {
	written := false
	var previous string
	for _, raw := range snippets {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if written {
			if !strings.Contains(previous, "\n") && !strings.Contains(s, "\n") {
				buf.WriteByte('\n')
			} else {
				buf.WriteString("\n\n")
			}
		}
		buf.WriteString(s)
		previous = s
		written = true
	}
	if written {
		buf.WriteByte('\n')
	}
}

// ── placeholder lookup ───────────────────────────────────────────────

//nolint:gocyclo // one case per template placeholder; a table would be larger and less readable.
func makeRenderLookup(cfg RenderConfig, stats PackStats, tree string) func(string) (string, bool) {
	savedPct := stats.SavedPercent()
	outputFile := cfg.OutputFile
	if outputFile == "" {
		outputFile = "stdout"
	}
	targets := cfg.Targets
	if targets == "" {
		targets = "all"
	}

	return func(key string) (string, bool) {
		switch key {
		case "PROFILE_NAME", "PROFILE":
			return cfg.ProfileName, true
		case "TARGET", "TARGETS":
			return targets, true
		case "PROMPT_NAME":
			return cfg.PromptName, true
		case "EDITOR_NAME", "EDITOR":
			return cfg.Editor, true
		case "DIR", "DIRS":
			return cfg.Dirs, true
		case "OUTPUT_FILE":
			return outputFile, true
		case "CURRENT_DATE":
			return cfg.CurrentDate, true
		case "GIT_COMMIT":
			return cfg.GitCommit, true
		case "GIT_TAG":
			return cfg.GitTag, true
		case "GIT_BRANCH":
			return cfg.GitBranch, true
		case "TREE":
			return tree, true
		case "FILE_COUNT":
			return itoa(stats.FileCount), true
		case "FUNC_COUNT":
			return itoa(stats.FuncCount), true
		case "TOTAL_SIZE_BYTES":
			return i64toa(stats.PackedBytes), true
		case "TOTAL_SIZE_KBYTES":
			return ftoa(float64(stats.PackedBytes) / 1024), true
		case "RAW_SIZE_BYTES":
			return i64toa(stats.RawBytes), true
		case "RAW_SIZE_KBYTES":
			return ftoa(float64(stats.RawBytes) / 1024), true
		case "SAVED_PERCENT":
			return ftoa(savedPct), true
		case "TOTAL_SIZE_TOKENS":
			return i64toa(stats.EstimatedTokens), true
		default:
			return "", false
		}
	}
}

func fileLookup(file *ParsedFile) func(string) (string, bool) {
	return func(key string) (string, bool) {
		switch key {
		case "REL_PATH":
			return file.RelPath, true
		case "ABS_PATH":
			return file.Path, true
		case "LANG":
			return file.Lang, true
		case "LINES":
			return itoa(file.Lines), true
		case "SIZE_KB":
			return ftoa(float64(file.Size) / 1024), true
		default:
			return "", false
		}
	}
}
