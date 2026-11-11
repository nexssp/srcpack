package operators

import (
	"github.com/nexssp/flow/extensions/fs"
)

type StatEntry struct {
	RelPath   string
	Size      int64
	FuncCount int
}

type PackConfig struct {
	Targets        []string          `json:"targets" cli:"targets,target,t"`
	Files          []string          `json:"files" cli:"files,file,f"`
	TargetProfiles map[string]string `json:"target_profiles"`
	TargetExcludes map[string]string `json:"target_excludes"`
	TargetPrompts  map[string]string `json:"target_prompts"`
	// Remote materialization keeps filesystem roots separate from stable labels.
	TargetRoots    map[string]string `json:"target_roots,omitempty"`
	TargetLabels   map[string]string `json:"target_labels,omitempty"`
	Ext            string            `json:"ext" cli:"ext"`
	NeedFuncCount  bool              `json:"need_func_count"`
	Tests          bool              `json:"tests" cli:"tests"`
	NoTests        bool              `json:"no_tests" cli:"no-tests"`
	NoGenerated    bool              `json:"no_generated" cli:"no-generated"`
	NoDeps         bool              `json:"no_deps" cli:"no-deps"`
	NoExamples     bool              `json:"no_examples" cli:"no-examples"`
	Examples       []string          `json:"examples"`
	Comments       bool              `json:"comments" cli:"comments"`
	OnlySig        bool              `json:"only_sig" cli:"only-sig"`
	Dense          bool              `json:"dense" cli:"dense"`
	Mask           bool              `json:"mask" cli:"mask"`
	IncludeConfig  bool              `json:"include_config" cli:"include-config"`
	MaxLines       int               `json:"max_lines" cli:"max-lines"`
	MaxLinesHead   int               `json:"max_lines_head"`
	MaxLinesTail   int               `json:"max_lines_tail"`
	TrimBlankLines bool              `json:"trim_blank_lines" cli:"trim-blank-lines"`
	TrimIndent     bool              `json:"trim_indent" cli:"trim-indent"`
	ExportedOnly   bool              `json:"exported_only" cli:"exported-only"`
	Exclude        string            `json:"exclude" cli:"exclude,x"`
	Prompt         string            `json:"prompt" cli:"prompt"`
	Editor         string            `json:"editor" cli:"editor,e"`
	Output         string            `json:"output" cli:"o,output"`
	Copy           bool              `json:"copy" cli:"copy,c"`
	NoCopy         bool              `json:"no_copy" cli:"no-copy"`
	TreeOnly       bool              `json:"tree_only" cli:"tree-only"`
	FullTree       bool              `json:"full_tree" cli:"full-tree"`
	Depth          int               `json:"depth" cli:"depth,L"`
	Changed        bool              `json:"changed" cli:"changed,m"`
	Since          string            `json:"since" cli:"since"`
	Modified       bool              `json:"modified"`
	Staged         bool              `json:"staged"`
	DiffOnly       bool              `json:"diff_only"`
	DiffContent    string            `json:"diff_content"`
	Quiet          bool              `json:"quiet" cli:"quiet,q"`
	Color          string            `json:"color" cli:"color"`
	CostROI        bool              `json:"cost"`
	DirPrefix      bool              `json:"dir_prefix"`
	NoHeader       bool              `json:"no_header"`
	NoPrompt       bool              `json:"no_prompt"`
	NoStart        bool              `json:"no_start"`
	NoEnd          bool              `json:"no_end"`

	MaxFileSize  int64 `json:"max_file_size"`
	MaxFilesScan int64 `json:"max_files_scan"`

	DenseOverride    *bool `json:"dense_override"`
	OnlySigOverride  *bool `json:"only_sig_override"`
	CommentsOverride *bool `json:"comments_override"`
	NoTestsOverride  *bool `json:"no_tests_override"`
}

type ParsedFile struct {
	fs.FileMeta
	Content         []byte
	Snippets        []string
	FuncCount       int
	Lines           int
	SourceStartLine int          `json:"source_start_line,omitempty"`
	SourceEndLine   int          `json:"source_end_line,omitempty"`
	SymbolSpans     []SymbolSpan `json:"symbol_spans,omitempty"`
	IsRaw           bool
	IsFull          bool
	Omitted         bool
	OmitReason      string
	ExpandTree      bool
	Hash            string
	AbsPath         string
	Profile         string
	Prompt          string
	ParseOptions    ParseOptions
	ParseOptionsSet bool
}

type SymbolSpan struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Receiver  string `json:"receiver,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type ParseOptions struct {
	OnlySig        bool
	Dense          bool
	Comments       bool
	NeedFuncCount  bool
	Mask           bool
	IncludeConfig  bool
	MaxLines       int
	MaxLinesHead   int
	MaxLinesTail   int
	ExportedOnly   bool
	TrimIndent     bool
	TrimBlankLines bool
}
