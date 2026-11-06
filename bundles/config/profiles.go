package config

import "strings"

// ProfileMeta describes one shipped built-in profile. The Options
// field carries the profile's default overlay; a user override in
// nexss.srcpack.yml merges on top of it.
type ProfileMeta struct {
	Name        string
	Aliases     []string
	Description string
	PromptKey   string
	Options     ConfigOptions
}

// BuiltinProfiles is the ordered list of profiles every srcpack run
// starts from. User profiles with the same name override fields on
// top; user profiles with a new name are added alongside.
var BuiltinProfiles = []ProfileMeta{
	{
		Name:        "arch",
		Aliases:     []string{"no-comments", "nocomments", "skeleton", "api-map", "arch-clean"},
		Description: "Architecture map: signatures only (NO bodies, NO comments)",
		Options: ConfigOptions{
			ExportedOnly:   new(true),
			RemoveBodyFunc: new(true),
			StripComments:  new(true),
			NoTests:        new(true),
			NoGenerated:    new(true),
			NoDeps:         new(true),
		},
	},
	{
		Name:        "api",
		Aliases:     []string{"with-comments", "withcomments", "docs", "contracts", "arch-docs"},
		Description: "Public API declarations: signatures + doc comments (NO bodies, WITH comments)",
		Options: ConfigOptions{
			ExportedOnly:   new(true),
			RemoveBodyFunc: new(true),
			StripComments:  new(false),
			NoTests:        new(true),
			NoGenerated:    new(true),
			NoDeps:         new(true),
		},
	},
	{
		Name:        "impl",
		Aliases:     []string{"review", "logic"},
		Description: "Implementation logic (bodies preserved, comments stripped)",
		Options: ConfigOptions{
			ExportedOnly:   new(false),
			RemoveBodyFunc: new(false),
			StripComments:  new(true),
			NoTests:        new(true),
			NoGenerated:    new(true),
			NoDeps:         new(true),
		},
	},
	{
		Name:        "source",
		Aliases:     []string{"full", "src", "sources"},
		Description: "Full code context for AI (bodies & comments, no tests/deps)",
		Options: ConfigOptions{
			ExportedOnly:   new(false),
			RemoveBodyFunc: new(false),
			StripComments:  new(false),
			NoTests:        new(true),
			NoGenerated:    new(true),
			NoDeps:         new(true),
		},
	},
	{
		Name:        "with-tests",
		Aliases:     []string{"test", "tests"},
		Description: "Source code plus unit tests and test helpers",
		Options: ConfigOptions{
			ExportedOnly:   new(false),
			RemoveBodyFunc: new(false),
			StripComments:  new(false),
			NoTests:        new(false),
		},
	},
	{
		Name:        "compact",
		Aliases:     []string{"min", "tiny"},
		Description: "Aggressively reduced context for small token budgets",
		Options: ConfigOptions{
			ExportedOnly:   new(true),
			RemoveBodyFunc: new(true),
			StripComments:  new(true),
			TrimBlankLines: new(true),
			TrimIndent:     new(true),
			MaxLines:       new(100),
			NoTests:        new(true),
			NoGenerated:    new(true),
			NoDeps:         new(true),
		},
	},
	{
		Name:        "dense",
		Aliases:     []string{"ai-dense", "idl", "ultra", "signatures"},
		Description: "Ultra-compact IDL for AI reasoning (signatures only, max token savings)",
		Options: ConfigOptions{
			ExportedOnly:   new(true),
			RemoveBodyFunc: new(true),
			StripComments:  new(true),
			NoTests:        new(true),
			NoGenerated:    new(true),
			NoDeps:         new(true),
			TrimBlankLines: new(true),
			TrimIndent:     new(true),
			Dense:          new(true),
		},
	},
}

// CanonicalProfileName resolves a case-insensitive name or alias to a
// built-in profile's canonical name. Unknown names are returned trimmed
// but otherwise unchanged, so user-defined profile names pass through.
func CanonicalProfileName(name string) string {
	name = strings.TrimSpace(name)
	for i := range BuiltinProfiles {
		p := &BuiltinProfiles[i]
		if strings.EqualFold(p.Name, name) {
			return p.Name
		}
		for _, alias := range p.Aliases {
			if strings.EqualFold(alias, name) {
				return p.Name
			}
		}
	}
	return name
}

// FindBuiltinProfile returns the built-in profile matching name or one
// of its aliases.
func FindBuiltinProfile(name string) (ProfileMeta, bool) {
	canonical := CanonicalProfileName(name)
	for i := range BuiltinProfiles {
		if BuiltinProfiles[i].Name == canonical {
			return BuiltinProfiles[i], true
		}
	}
	return ProfileMeta{}, false
}
