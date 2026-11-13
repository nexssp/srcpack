# srcpack cookbook

Copy a command, replace `internal/auth` or the example file with paths from your project, and run it from the project root. The terminal view uses color when stdout is a terminal; use `--color always` or `--color never` to override it. Add `--md` to print this document as pure Markdown without terminal styling or banners. A normal pack prints the complete rendered result to stdout. Use `--copy` to copy the full result to the clipboard instead, and add `--preview` only when you want a short terminal preview of that copied output; previews are off by default. Use `-o FILE` to save the result. `--stdout` explicitly selects stdout and cannot be combined with `-o FILE`.

## Get oriented

```sh
# Quick architecture map of the current project (copy to clipboard)
srcpack ".#arch"

# Print the full rendered package to stdout
srcpack ".#arch" --stdout

# Save the selected source to a Markdown file
srcpack ".#source" -o project-context.md

# See the available profiles and a profile's description
srcpack list
srcpack info api

# See supported file-type aliases
srcpack types
```

## Pick files, folders, and detail levels

```sh
# One file or one folder
srcpack "internal/auth/service.go#source" --stdout
srcpack "internal/auth#impl" --stdout

# Several paths in one selector, or several selectors in one command
srcpack "cmd,internal#arch" --stdout
srcpack "internal/auth#api" "internal/store#with-tests" -o combined-context.md

# Keep repo roots visible, then expand only the selected kernel branch in the summary
srcpack "../flow#source" "../../nexss_ecosystem/kernel#arch" "../../nexss_ecosystem/transport#arch"
srcpack "../../nexss_ecosystem/kernel/...#arch" "../flow#source"

# Exclude matching files or folders (quote selectors so the shell keeps ~ and #)
srcpack "internal/auth~*_test.go#impl" -o auth.md
srcpack "internal~vendor,fixtures/**#source" -o source.md

# Mix fidelity: public API for one area, full source for another
srcpack "internal/auth#api" "internal/session#source" -o auth-review.md
```

Profiles: `arch` for a map of declarations, `api` for commented public interfaces, `impl` for implementation bodies, `source` for selected source plus comments, and `with-tests` to include tests. Use `srcpack list` for the complete built-in list.

## Slice a useful part of a file

Line numbers are 1-based and inclusive. These forms apply to local files:

```sh
# Lines 80 through 140
srcpack "internal/auth/service.go:80-140#source" --stdout

# From line 80 through the end of the file
srcpack "internal/auth/service.go:80-#source" --stdout

# From the start of the file through line 140
srcpack "internal/auth/service.go:-140#source" --stdout

# One line (same behavior as before)
srcpack "internal/auth/service.go:23#source" --stdout
```

## Include curated config and trace source lines

`--include-config` is opt-in. It adds eligible build and tooling config as raw text, masking detected secret-like values on a best-effort basis. `.env` variants and common credential/private-key files remain excluded, even when explicitly selected; review output before sharing.

```sh
# Include eligible config files alongside the architecture map
srcpack ".#arch" --include-config -o context.md

# Keep original source coordinates for a line slice
srcpack "internal/auth/service.go:80-140#source" --stdout
```

Formatted file blocks start with a machine-readable location comment; `raw`, `none`, and `text` modes remain unmodified. `start_line` and `end_line` are 1-based, inclusive original-file lines. Rendered Markdown omits line mappings derivable from the body's starting line. Otherwise, `line_ranges` compactly maps rendered-body lines to original lines: `1-3:52-54,5:60-62` means body lines 1–3 map to source lines 52–54, and body line 5 maps to source lines 60–62. Structured source records retain complete `line_spans` provenance. Coordinates are per-file, not positions in the concatenated pack. Blank, synthetic, ambiguous, and otherwise unmapped lines have no entry. GitHub URLs keep their required `#Lx-Ly` anchor syntax. Go signature profiles add `symbols` with declaration names, kinds, and original line spans:

```html
<!-- srcpack-source: {"path":"internal/auth/service.go","start_line":80,"end_line":140} -->
<!-- srcpack-source: {"path":"internal/auth/service.go","start_line":1,"end_line":120,"symbols":[{"name":"Lookup","kind":"func","start_line":12,"end_line":18}],"line_ranges":"1:12-18"} -->
```

## Add prompt text

```sh
# Attach inline prompt text to a selector
srcpack 'internal/auth#source@Look for authorization gaps and cite relevant checks.' -o auth-review.md

# Attach prompt-file contents to a selector
srcpack "internal/auth#source@review-prompt.txt" -o auth-review.md

# Put an instruction in the rendered package for this run
srcpack "internal/auth#source" --prompt "Check authorization paths and cite relevant checks." -o auth-review.md
```

`@...` accepts inline text or reads a prompt file into per-file selector metadata. The current Markdown renderer does not print that per-file metadata. Use inline `--prompt` text when you want a shared prompt block in the rendered package; a path passed to `--prompt` is treated as literal text.

## Compare local changes

Run these inside a Git worktree:

```sh
# Modified and untracked local files
srcpack ".#impl" --modified -o modified-context.md

# Staged files only
srcpack ".#source" --staged -o staged-context.md

# Include the unified diff with the changed source
srcpack ".#impl" --modified --diff-only -o review.md

# Files changed since a base ref
srcpack ".#with-tests" --since origin/main -o branch-context.md

# Reuse srcpack's saved incremental state
srcpack ".#arch" --changed -o changed-since-last-run.md
```

`--modified`, `--staged`, `--since`, and `--changed` select local workspace changes; do not combine them with remote GitHub selectors. Add `--diff-only` when you also want the Git diff included.

## Use public GitHub source

```sh
# A subfolder at a tag, branch, or commit; repositories must be public
srcpack 'github:nexssp/flow//compiler?ref=v0.68.0#arch' -o flow-compiler.md

# Replace OWNER, REPO, PATH, and REF with the public repository and revision
srcpack 'github:OWNER/REPO//PATH?ref=REF#source' -o remote-context.md

# Combine local and remote code in one result
srcpack 'internal/client#impl' 'github:OWNER/REPO//PATH?ref=REF#api' -o comparison.md
```

Git must be installed. Percent-encode special characters in a remote path or ref when needed. Remote source cannot use the local-change flags above.

## Print, save, preview, and browse a tree

```sh
# Print the full result to the terminal, or write to a file
srcpack "internal/auth#api" --stdout
srcpack "internal/auth#api" -o auth-api.md

# Preview the resolved selection without scanning or writing output
srcpack pack "internal/auth#impl" --plan

# Preview whether a generated output would change
srcpack "internal/auth#impl" -o auth.md --check

# Inspect folder structure; include files and limit depth
srcpack tree "internal/auth" --tree-files --depth 4 -o auth-tree.md

# Show the whole local project tree, including excluded paths
srcpack tree --full-tree --tree-files -o project-tree.md
```

## Save and reuse a selector set

```sh
# Save a repeatable review selection (this updates workspace configuration)
srcpack "internal/auth#impl" "internal/session#with-tests" --save-view auth-review

# Re-run it, print it, inspect it, or remove it
srcpack auth-review -o auth-review.md
srcpack view list
srcpack view show auth-review
srcpack view delete auth-review
```

## Set up and inspect workspace configuration

```sh
# Create the starter nexss.srcpack.yml and ignore file
srcpack init

# Validate and inspect configuration
srcpack doctor
srcpack config path
srcpack config show
srcpack config validate

# Add a reusable named target, then use it like a path
srcpack target add auth internal/auth
srcpack target list
srcpack target show auth
srcpack "auth#impl" -o auth.md
srcpack target delete auth

# Manage reusable prompt snippets
srcpack prompt add review "Check error handling, edge cases, and security assumptions."
srcpack prompt list
srcpack prompt show review
srcpack prompt delete review

# Inspect built-in or configured profiles and output formats
srcpack profile list
srcpack profile show arch
srcpack editor list
srcpack editor show markdown
```

`target add`, `prompt add`, and `view delete` change `nexss.srcpack.yml`. To reuse a named prompt on a profile, set the profile's `prompt` to that snippet name in the config:

```yaml
prompts:
  review: "Check error handling, edge cases, and security assumptions."
profiles:
  auth-review:
    prompt: review
```

Profiles and editors can be inspected from the CLI; edit their configuration in that file.

## When a command is unfamiliar

```sh
srcpack --help
srcpack pack --help
srcpack cookbook
```
