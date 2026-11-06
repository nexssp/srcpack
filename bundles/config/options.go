package config

import (
	"os"
	"time"
)

// DefaultTreeDepth is the single source of truth for omitted tree depth.
const DefaultTreeDepth = 1

// ResolveTreeDepth preserves positive overrides and defaults unset/invalid values.
func ResolveTreeDepth(depth int) int {
	if depth <= 0 {
		return DefaultTreeDepth
	}
	return depth
}

// ChangeDetector reports whether a file should be included under an
// incremental or git-filtered run. Implementations live in the
// operators package; the config layer only needs the contract.
type ChangeDetector interface {
	IsActive() bool
	IsFileChanged(absPath, relPath string, modTime time.Time) (bool, error)
}

// Guardrails enforces per-scan safety limits.
type Guardrails interface {
	CheckFile(path string, info os.FileInfo) error
}

// DefaultExampleFolders is the built-in set of directory names treated
// as "examples" and excluded when NoExamples is on.
var DefaultExampleFolders = []string{
	"examples", "example",
	"samples", "sample",
	"demo", "demos",
}

// Options is the fully resolved configuration for one pipeline run.
// Every field is concrete; overlay of ConfigOptions on top of it
// happens once, before scanning starts.
type Options struct {
	StripComments    bool
	RemoveBodyFunc   bool
	MaxLines         int
	MaxLinesHead     int
	MaxLinesTail     int
	NoTests          bool
	NoGenerated      bool
	TrimBlankLines   bool
	TrimIndent       bool
	ExtFilter        string
	NoDeps           bool
	OutputFile       string
	ExportedOnly     bool
	NoHeader         bool
	NoPrompt         bool
	NoStart          bool
	NoEnd            bool
	Since            string
	Modified         bool
	Staged           bool
	Changed          bool
	MaskSecrets      bool
	DiffOnly         bool
	Tree             bool
	TreeOnly         bool
	TreeFiles        bool
	TreeDepth        int
	FullTree         bool
	CostROI          bool
	IncludeDirPrefix bool
	Dense            bool
	Examples         []string
	NoExamples       bool
	Detector         ChangeDetector
	Guardrails       Guardrails
}

// ConfigOptions is the sparse, YAML-facing form of Options. A nil
// pointer means "not specified"; overlay skips those fields.
type ConfigOptions struct {
	StripComments    *bool    `yaml:"strip_comments,omitempty"`
	RemoveBodyFunc   *bool    `yaml:"remove_body_func,omitempty"`
	MaxLines         *int     `yaml:"max_lines,omitempty"`
	MaxLinesHead     *int     `yaml:"max_lines_head,omitempty"`
	MaxLinesTail     *int     `yaml:"max_lines_tail,omitempty"`
	NoTests          *bool    `yaml:"no_tests,omitempty"`
	NoGenerated      *bool    `yaml:"no_generated,omitempty"`
	TrimBlankLines   *bool    `yaml:"trim_blank_lines,omitempty"`
	TrimIndent       *bool    `yaml:"trim_indent,omitempty"`
	NoDeps           *bool    `yaml:"no_deps,omitempty"`
	ExtFilter        *string  `yaml:"ext_filter,omitempty"`
	ExportedOnly     *bool    `yaml:"exported_only,omitempty"`
	NoHeader         *bool    `yaml:"no_header,omitempty"`
	NoPrompt         *bool    `yaml:"no_prompt,omitempty"`
	NoStart          *bool    `yaml:"no_start,omitempty"`
	NoEnd            *bool    `yaml:"no_end,omitempty"`
	Since            *string  `yaml:"since,omitempty"`
	Modified         *bool    `yaml:"modified,omitempty"`
	Staged           *bool    `yaml:"staged,omitempty"`
	Changed          *bool    `yaml:"changed,omitempty"`
	MaskSecrets      *bool    `yaml:"mask_secrets,omitempty"`
	DiffOnly         *bool    `yaml:"diff_only,omitempty"`
	Tree             *bool    `yaml:"tree,omitempty"`
	TreeOnly         *bool    `yaml:"tree_only,omitempty"`
	TreeFiles        *bool    `yaml:"tree_files,omitempty"`
	TreeDepth        *int     `yaml:"tree_depth,omitempty"`
	FullTree         *bool    `yaml:"full_tree,omitempty"`
	CostROI          *bool    `yaml:"cost,omitempty"`
	IncludeDirPrefix *bool    `yaml:"include_dir_prefix,omitempty"`
	Dense            *bool    `yaml:"dense,omitempty"`
	Examples         []string `yaml:"examples,omitempty"`
	NoExamples       *bool    `yaml:"no_examples,omitempty"`
}

// OptionMutator is one step in a chain of option overlays.
type OptionMutator func(*Options)

// Apply runs every non-nil mutator in order and returns the receiver
// for chaining.
func (o *Options) Apply(mutators ...OptionMutator) *Options {
	for _, m := range mutators {
		if m != nil {
			m(o)
		}
	}
	return o
}

func setIf[T any](dst, src *T) {
	if src != nil {
		*dst = *src
	}
}

func setIfPtr[T any](dst **T, src *T) {
	if src != nil {
		*dst = src
	}
}

// Mutator returns an OptionMutator that overlays every non-nil field
// of c onto the target Options.
//
//nolint:dupl // Mirrors MergeConfigOptions field-for-field by design; both must stay in sync with the ConfigOptions schema.
func (c ConfigOptions) Mutator() OptionMutator {
	return func(o *Options) {
		setIf(&o.StripComments, c.StripComments)
		setIf(&o.RemoveBodyFunc, c.RemoveBodyFunc)
		setIf(&o.MaxLines, c.MaxLines)
		setIf(&o.MaxLinesHead, c.MaxLinesHead)
		setIf(&o.MaxLinesTail, c.MaxLinesTail)
		setIf(&o.NoTests, c.NoTests)
		setIf(&o.NoGenerated, c.NoGenerated)
		setIf(&o.TrimBlankLines, c.TrimBlankLines)
		setIf(&o.TrimIndent, c.TrimIndent)
		setIf(&o.NoDeps, c.NoDeps)
		setIf(&o.ExtFilter, c.ExtFilter)
		setIf(&o.ExportedOnly, c.ExportedOnly)
		setIf(&o.NoHeader, c.NoHeader)
		setIf(&o.NoPrompt, c.NoPrompt)
		setIf(&o.NoStart, c.NoStart)
		setIf(&o.NoEnd, c.NoEnd)
		setIf(&o.Since, c.Since)
		setIf(&o.Modified, c.Modified)
		setIf(&o.Staged, c.Staged)
		setIf(&o.Changed, c.Changed)
		setIf(&o.MaskSecrets, c.MaskSecrets)
		setIf(&o.DiffOnly, c.DiffOnly)
		setIf(&o.Tree, c.Tree)
		setIf(&o.TreeOnly, c.TreeOnly)
		setIf(&o.TreeFiles, c.TreeFiles)
		setIf(&o.TreeDepth, c.TreeDepth)
		setIf(&o.FullTree, c.FullTree)
		setIf(&o.CostROI, c.CostROI)
		setIf(&o.IncludeDirPrefix, c.IncludeDirPrefix)
		setIf(&o.Dense, c.Dense)
		if len(c.Examples) > 0 {
			o.Examples = append([]string(nil), c.Examples...)
		}
		setIf(&o.NoExamples, c.NoExamples)
	}
}

// MergeConfigOptions overlays every non-nil field of src onto dst.
//
//nolint:dupl // Mirrors ConfigOptions.Mutator field-for-field by design; both must stay in sync with the ConfigOptions schema.
func MergeConfigOptions(dst *ConfigOptions, src ConfigOptions) {
	setIfPtr(&dst.StripComments, src.StripComments)
	setIfPtr(&dst.RemoveBodyFunc, src.RemoveBodyFunc)
	setIfPtr(&dst.MaxLines, src.MaxLines)
	setIfPtr(&dst.MaxLinesHead, src.MaxLinesHead)
	setIfPtr(&dst.MaxLinesTail, src.MaxLinesTail)
	setIfPtr(&dst.NoTests, src.NoTests)
	setIfPtr(&dst.NoGenerated, src.NoGenerated)
	setIfPtr(&dst.TrimBlankLines, src.TrimBlankLines)
	setIfPtr(&dst.TrimIndent, src.TrimIndent)
	setIfPtr(&dst.NoDeps, src.NoDeps)
	setIfPtr(&dst.ExtFilter, src.ExtFilter)
	setIfPtr(&dst.ExportedOnly, src.ExportedOnly)
	setIfPtr(&dst.NoHeader, src.NoHeader)
	setIfPtr(&dst.NoPrompt, src.NoPrompt)
	setIfPtr(&dst.NoStart, src.NoStart)
	setIfPtr(&dst.NoEnd, src.NoEnd)
	setIfPtr(&dst.Since, src.Since)
	setIfPtr(&dst.Modified, src.Modified)
	setIfPtr(&dst.Staged, src.Staged)
	setIfPtr(&dst.Changed, src.Changed)
	setIfPtr(&dst.MaskSecrets, src.MaskSecrets)
	setIfPtr(&dst.DiffOnly, src.DiffOnly)
	setIfPtr(&dst.Tree, src.Tree)
	setIfPtr(&dst.TreeOnly, src.TreeOnly)
	setIfPtr(&dst.TreeFiles, src.TreeFiles)
	setIfPtr(&dst.TreeDepth, src.TreeDepth)
	setIfPtr(&dst.FullTree, src.FullTree)
	setIfPtr(&dst.CostROI, src.CostROI)
	setIfPtr(&dst.IncludeDirPrefix, src.IncludeDirPrefix)
	setIfPtr(&dst.Dense, src.Dense)
	if len(src.Examples) > 0 {
		dst.Examples = append([]string(nil), src.Examples...)
	}
	setIfPtr(&dst.NoExamples, src.NoExamples)
}

// ToOptions materializes a fresh Options with the srcpack defaults and
// then overlays c on top.
func (c ConfigOptions) ToOptions() Options {
	o := Options{
		MaskSecrets: true,
		Examples:    append([]string(nil), DefaultExampleFolders...),
		NoExamples:  true,
	}
	o.Apply(c.Mutator())
	o.TreeDepth = ResolveTreeDepth(o.TreeDepth)
	return o
}
