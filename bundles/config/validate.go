package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	remote "github.com/nexssp/srcpack/bundles/github_source"
)

// IssueSeverity classifies one validation finding.
type IssueSeverity string

const (
	SeverityOK      IssueSeverity = "OK"
	SeverityWarning IssueSeverity = "WARN"
	SeverityError   IssueSeverity = "ERROR"
)

// ValidationIssue is one finding from Validate.
type ValidationIssue struct {
	Severity IssueSeverity
	Category string
	Message  string
}

// ValidationReport is the structured result of a config audit.
type ValidationReport struct {
	ConfigFile string
	Issues     []ValidationIssue
	Passed     bool
}

func (r ValidationReport) String() string {
	var b strings.Builder
	b.WriteString("\n🩺 Srcpack doctor\n")
	b.WriteString(strings.Repeat("─", 45) + "\n")
	for _, issue := range r.Issues {
		icon := "✅"
		switch issue.Severity {
		case SeverityWarning:
			icon = "⚠️ "
		case SeverityError:
			icon = "❌"
		}
		fmt.Fprintf(&b, "%s [%-7s] %s\n", icon, issue.Category, issue.Message)
	}
	b.WriteString(strings.Repeat("─", 45) + "\n")
	if r.Passed {
		b.WriteString("All checks passed.\n\n")
	} else {
		b.WriteString("Errors found. Resolve issues above.\n\n")
	}
	return b.String()
}

// Validate audits a workspace configuration: missing target paths,
// cycles in target sub-targets, references into _srcpack, and profile
// prompts that name an undefined prompt. It never fails — problems are
// reported in the returned report with Passed=false.
func Validate(cwd, configPath string) ValidationReport {
	report := ValidationReport{ConfigFile: configPath, Passed: true}

	cfg, err := Load(cwd, configPath)
	if err != nil {
		report.Issues = append(report.Issues, ValidationIssue{
			Severity: SeverityError,
			Category: "config",
			Message:  fmt.Sprintf("failed to load: %v", err),
		})
		report.Passed = false
		return report
	}

	report.Issues = append(report.Issues, ValidationIssue{
		Severity: SeverityOK,
		Category: "config",
		Message:  "loaded " + filepath.Base(configPath),
	})

	configDir := filepath.Dir(configPath)
	auditTargets(&report, cfg, configDir)
	auditProfiles(&report, cfg)
	return report
}

func auditTargets(report *ValidationReport, cfg Config, configDir string) {
	names := make([]string, 0, len(cfg.TargetsMap))
	for name := range cfg.TargetsMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		tg := cfg.TargetsMap[name]
		targetOK := true

		paths := append([]string{}, tg.Paths...)
		for _, entry := range tg.Dir {
			paths = append(paths, entry.Path)
		}
		paths = append(paths, tg.Files...)

		for _, raw := range paths {
			if remote.IsRemote(raw) {
				if _, err := remote.Parse(raw); err != nil {
					report.Issues = append(report.Issues, ValidationIssue{Severity: SeverityError, Category: categoryTarget, Message: fmt.Sprintf("%s has invalid remote source %s: %v", name, raw, err)})
					report.Passed = false
					targetOK = false
				} else {
					report.Issues = append(report.Issues, ValidationIssue{Severity: SeverityOK, Category: categoryTarget, Message: fmt.Sprintf("%s: remote syntax valid (availability not checked): %s", name, raw)})
				}
				continue
			}
			clean := filepath.ToSlash(raw)
			if strings.Contains(clean, "_srcpack") {
				report.Issues = append(report.Issues, ValidationIssue{
					Severity: SeverityError,
					Category: categoryTarget,
					Message:  fmt.Sprintf("%s references output directory: %s", name, raw),
				})
				report.Passed = false
				targetOK = false
				continue
			}
			abs := raw
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(configDir, abs)
			}
			if _, statErr := os.Stat(abs); statErr != nil {
				report.Issues = append(report.Issues, ValidationIssue{
					Severity: SeverityError,
					Category: categoryTarget,
					Message:  fmt.Sprintf("%s references missing path: %s", name, raw),
				})
				report.Passed = false
				targetOK = false
			}
		}

		for _, sub := range tg.Targets {
			if _, ok := cfg.TargetsMap[sub]; !ok {
				report.Issues = append(report.Issues, ValidationIssue{
					Severity: SeverityError,
					Category: categoryTarget,
					Message:  fmt.Sprintf("%s references undefined sub-target: %s", name, sub),
				})
				report.Passed = false
				targetOK = false
			}
		}

		if hasCycle(name, cfg.TargetsMap) {
			report.Issues = append(report.Issues, ValidationIssue{
				Severity: SeverityError,
				Category: "target",
				Message:  name + " has a circular sub-target dependency",
			})
			report.Passed = false
			targetOK = false
		}

		if targetOK {
			report.Issues = append(report.Issues, ValidationIssue{
				Severity: SeverityOK,
				Category: "target",
				Message:  name + ": valid",
			})
		}
	}
}

// TargetHasCycle reports whether a target's reachable dependency graph cycles.
// It performs no filesystem access and leaves reporting policy to its caller.
func TargetHasCycle(start string, targets map[string]TargetGroup) bool {
	return hasCycle(start, targets)
}

func hasCycle(start string, targets map[string]TargetGroup) bool {
	visited := map[string]bool{}
	var walk func(name string) bool
	walk = func(name string) bool {
		if visited[name] {
			return true
		}
		visited[name] = true
		for _, sub := range targets[name].Targets {
			if _, ok := targets[sub]; !ok {
				continue
			}
			if walk(sub) {
				return true
			}
		}
		visited[name] = false
		return false
	}
	// Temporary visit marker for the root, cleared before returning.
	visited[start] = true
	for _, sub := range targets[start].Targets {
		if _, ok := targets[sub]; !ok {
			continue
		}
		if walk(sub) {
			return true
		}
	}
	return false
}

func auditProfiles(report *ValidationReport, cfg Config) {
	//nolint:gocritic // rangeValCopy: map values are not addressable; Profile is config-time only.
	for pName, p := range cfg.Profiles {
		if !p.Prompt.Set || p.Prompt.Disabled || p.Prompt.Value == "" {
			continue
		}
		if _, ok := cfg.Prompts[p.Prompt.Value]; !ok {
			report.Issues = append(report.Issues, ValidationIssue{
				Severity: SeverityWarning,
				Category: "profile",
				Message:  fmt.Sprintf("%s references undefined prompt %q", pName, p.Prompt.Value),
			})
		}
	}
}
