// Package config loads and merges nexss.srcpack.yml files. It owns
// every srcpack-specific type, default, and merge rule; file discovery
// and YAML decoding are delegated to github.com/nexssp/flow/config.
package config

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level structure of nexss.srcpack.yml. Profile is
// embedded so top-level keys such as `editor:` and `options:` bind
// directly to the default profile without an intermediate `profile:`
// wrapper.
type Config struct {
	Prompts    map[string]string       `yaml:"prompts,omitempty" json:"prompts,omitempty"`
	Editors    map[string]EditorFormat `yaml:"editors,omitempty" json:"editors,omitempty"`
	TargetsMap map[string]TargetGroup  `yaml:"targets,omitempty" json:"targets,omitempty"`
	Views      map[string][]string     `yaml:"views,omitempty"   json:"views,omitempty"`
	Profile    `yaml:",inline"`
	Profiles   map[string]Profile `yaml:"profiles,omitempty" json:"profiles,omitempty"`
}

// Profile defines transformation rules, prompt bindings, and file or
// directory scopes for a named content mode.
type Profile struct {
	Name             string        `yaml:"-"                          json:"name,omitempty"`
	BaseDir          string        `yaml:"-"                          json:"base_dir,omitempty"`
	Description      string        `yaml:"description,omitempty"      json:"description,omitempty"`
	Example          string        `yaml:"example,omitempty"          json:"example,omitempty"`
	Prompt           FlexibleText  `yaml:"prompt,omitempty"            json:"prompt,omitzero"`
	Start            FlexibleText  `yaml:"start,omitempty"             json:"start,omitzero"`
	End              FlexibleText  `yaml:"end,omitempty"               json:"end,omitzero"`
	FileStart        string        `yaml:"file_start,omitempty"       json:"file_start,omitempty"`
	FileEnd          string        `yaml:"file_end,omitempty"         json:"file_end,omitempty"`
	Editor           string        `yaml:"editor,omitempty"           json:"editor,omitempty"`
	Output           string        `yaml:"output,omitempty"           json:"output,omitempty"`
	OutputFile       string        `yaml:"output_file,omitempty"      json:"output_file,omitempty"`
	Options          ConfigOptions `yaml:"options,omitempty"           json:"options,omitzero"`
	Dir              []DirEntry    `yaml:"dir,omitempty"              json:"dir,omitempty"`
	Files            []string      `yaml:"files,omitempty"            json:"files,omitempty"`
	Includes         []string      `yaml:"includes,omitempty"         json:"includes,omitempty"`
	IncludeProfiles  []string      `yaml:"include_profiles,omitempty" json:"include_profiles,omitempty"`
	IncludeProfile   string        `yaml:"include_profile,omitempty"  json:"include_profile,omitempty"`
	IncludedProfiles []string      `yaml:"-"                          json:"included_profiles,omitempty"`
}

// GetIncludeProfiles returns IncludeProfile followed by IncludeProfiles,
// with empty entries dropped.
func (p Profile) GetIncludeProfiles() []string {
	var out []string
	if v := strings.TrimSpace(p.IncludeProfile); v != "" {
		out = append(out, v)
	}
	for _, inc := range p.IncludeProfiles {
		if v := strings.TrimSpace(inc); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// GetOutputFile returns OutputFile, falling back to the legacy Output key.
func (p Profile) GetOutputFile() string {
	if p.OutputFile != "" {
		return p.OutputFile
	}
	return p.Output
}

// TargetGroup specifies a named subsystem of directories and explicit files.
type TargetGroup struct {
	Name        string        `yaml:"-"                    json:"name,omitempty"`
	Description string        `yaml:"description,omitempty" json:"description,omitempty"`
	Paths       []string      `yaml:"paths,omitempty"       json:"paths,omitempty"`
	Dir         []DirEntry    `yaml:"dir,omitempty"         json:"dir,omitempty"`
	Files       []string      `yaml:"files,omitempty"       json:"files,omitempty"`
	Targets     []string      `yaml:"targets,omitempty"     json:"targets,omitempty"`
	Prompt      FlexibleText  `yaml:"prompt,omitempty"       json:"prompt,omitzero"`
	Header      FlexibleText  `yaml:"header,omitempty"       json:"header,omitzero"`
	Options     ConfigOptions `yaml:"options,omitempty"      json:"options,omitzero"`
}

// EditorFormat defines layout rules for one output format.
type EditorFormat struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Start       string `yaml:"start,omitempty"       json:"start,omitempty"`
	End         string `yaml:"end,omitempty"         json:"end,omitempty"`
	FileStart   string `yaml:"file_start,omitempty"  json:"file_start,omitempty"`
	FileEnd     string `yaml:"file_end,omitempty"    json:"file_end,omitempty"`
}

// DirEntry is one entry in a target's `dir:` list. When no per-entry
// rule is present it may be written as a bare string; otherwise it is
// an object with optional full/omit/signatures sub-lists.
type DirEntry struct {
	Path       string   `yaml:"path"                 json:"path"`
	Full       []string `yaml:"full,omitempty"       json:"full,omitempty"`
	Omit       []string `yaml:"omit,omitempty"       json:"omit,omitempty"`
	Signatures []string `yaml:"signatures,omitempty" json:"signatures,omitempty"`
}

// FlexibleText accepts a string, a YAML block scalar, or the boolean
// false (meaning "explicitly disabled"). Set records whether the field
// was present at all, so overlay merge can distinguish "not specified"
// from "specified as disabled".
type FlexibleText struct {
	Value    string
	Disabled bool
	Set      bool
}

func (f FlexibleText) MarshalYAML() (any, error) {
	if f.Disabled {
		return false, nil
	}
	if !f.Set && f.Value == "" {
		return nil, nil
	}
	return f.Value, nil
}

func (f *FlexibleText) UnmarshalYAML(value *yaml.Node) error {
	f.Set = true
	var b bool
	if err := value.Decode(&b); err == nil {
		if !b {
			f.Disabled = true
		}
		return nil
	}
	var s string
	if err := value.Decode(&s); err == nil {
		s = strings.TrimSpace(s)
		if s == "false" {
			f.Disabled = true
		} else {
			f.Value = s
		}
	}
	return nil
}

func (d DirEntry) MarshalYAML() (any, error) {
	if len(d.Full) == 0 && len(d.Omit) == 0 && len(d.Signatures) == 0 {
		return d.Path, nil
	}
	type alias DirEntry
	return alias(d), nil
}

func (d *DirEntry) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		d.Path = s
		return nil
	}
	type alias DirEntry
	return value.Decode((*alias)(d))
}

// FormatSnippet renders a short preview of multi-line text: the first
// few words, then an ellipsis, then the last few words.
func FormatSnippet(s string, maxLen int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if len(clean) <= maxLen {
		return clean
	}
	headLen := (maxLen - 9) * 3 / 5
	tailLen := (maxLen - 9) - headLen
	if headLen < 15 {
		headLen = 15
	}
	if tailLen < 10 {
		tailLen = 10
	}
	if headLen+tailLen >= len(clean) {
		return clean
	}
	return clean[:headLen] + "... [...] ..." + clean[len(clean)-tailLen:]
}

// MergeProfile overlays src on dst. Fields that src leaves unset are
// preserved. Includes are unioned; Options merge recursively.
func MergeProfile(dst *Profile, src Profile) {
	if src.Description != "" {
		dst.Description = src.Description
	}
	if src.Example != "" {
		dst.Example = src.Example
	}
	if src.Prompt.Set {
		dst.Prompt = src.Prompt
	}
	if src.Start.Set {
		dst.Start = src.Start
	}
	if src.End.Set {
		dst.End = src.End
	}
	if src.FileStart != "" {
		dst.FileStart = src.FileStart
	}
	if src.FileEnd != "" {
		dst.FileEnd = src.FileEnd
	}
	if src.Editor != "" {
		dst.Editor = src.Editor
	}
	if src.Output != "" {
		dst.Output = src.Output
	}
	if src.OutputFile != "" {
		dst.OutputFile = src.OutputFile
	}
	if len(src.Dir) > 0 {
		dst.Dir = src.Dir
	}
	if len(src.Files) > 0 {
		dst.Files = src.Files
	}
	if len(src.Includes) > 0 {
		dst.Includes = MergeStringSlices(dst.Includes, src.Includes)
	}
	if len(src.IncludeProfiles) > 0 {
		dst.IncludeProfiles = MergeStringSlices(dst.IncludeProfiles, src.IncludeProfiles)
	}
	if src.IncludeProfile != "" {
		dst.IncludeProfile = src.IncludeProfile
	}
	MergeConfigOptions(&dst.Options, src.Options)
}

// MergeStringSlices appends src items to dst, skipping duplicates.
func MergeStringSlices(dst, src []string) []string {
	seen := make(map[string]bool, len(dst)+len(src))
	for _, item := range dst {
		seen[item] = true
	}
	for _, item := range src {
		if !seen[item] {
			seen[item] = true
			dst = append(dst, item)
		}
	}
	return dst
}

// ConfigRes is the structured response for `srcpack list`.
type ConfigRes struct {
	Profiles []ProfileInfo     `json:"profiles"`
	Targets  []TargetInfo      `json:"targets"`
	Prompts  map[string]string `json:"prompts"`
	Editors  []EditorInfo      `json:"editors"`
}

type ProfileInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Includes    []string `json:"includes,omitempty"`
}

type TargetInfo struct {
	Name    string   `json:"name"`
	Paths   []string `json:"paths,omitempty"`
	Dir     []string `json:"dir,omitempty"`
	Files   []string `json:"files,omitempty"`
	Targets []string `json:"targets,omitempty"`
	Prompt  string   `json:"prompt,omitempty"`
}

type EditorInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (c ConfigRes) String() string {
	var b strings.Builder

	if len(c.Profiles) > 0 {
		b.WriteString("Profiles:\n")
		for _, p := range c.Profiles {
			desc := p.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(&b, "  • %-12s %s\n", p.Name, desc)
			if len(p.Includes) > 0 {
				fmt.Fprintf(&b, "                 (Aliases: %s)\n", strings.Join(p.Includes, ", "))
			}
		}
		b.WriteString("\n")
	}

	if len(c.Targets) > 0 {
		b.WriteString("Targets:\n")
		for i := range c.Targets {
			t := &c.Targets[i]
			parts := append([]string{}, t.Paths...)
			parts = append(parts, t.Dir...)
			parts = append(parts, t.Files...)
			parts = append(parts, t.Targets...)

			marker := ""
			if t.Prompt != "" {
				marker = " 💬 [Custom Prompt attached]"
			}
			fmt.Fprintf(&b, "  • %-12s [%s]%s\n", t.Name, strings.Join(parts, ", "), marker)
		}
		b.WriteString("\n")
	}

	if len(c.Editors) > 0 {
		b.WriteString("Editors:\n")
		for _, e := range c.Editors {
			fmt.Fprintf(&b, "  • %-12s %s\n", e.Name, e.Description)
		}
		b.WriteString("\n")
	}

	if len(c.Prompts) > 0 {
		names := make([]string, 0, len(c.Prompts))
		for name := range c.Prompts {
			names = append(names, name)
		}
		sort.Strings(names)

		b.WriteString("Prompts:\n")
		for _, name := range names {
			fmt.Fprintf(&b, "  • %-12s %s\n", name, FormatSnippet(c.Prompts[name], 70))
		}
	}

	return b.String()
}
