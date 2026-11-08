# srcpack cheat sheet

## Selector grammar

```
path[,path...][~exclude[,exclude...]][#profile][@prompt]
```

| Slot | Example |
|---|---|
| Path | `src`, `main.go`, `backend` |
| Exclude | `~tests,vendor` |
| Profile | `#arch`, `#impl`, `#source`, `#dense` |
| Prompt | `@find auth bugs`, `@prompt.md` |
| Line range | `file.go:10-40`, `file.go:10-`, `file.go:-40`, `file.go:10` |

Compose:
```sh
srcpack "src#arch" "internal/auth#source@review" "cmd~tests#impl"
srcpack cookbook
```

## Profiles

| Profile | Bodies | Comments | Tests |
|---|:---:|:---:|:---:|
| `arch` | — | — | — |
| `api` | — | ✓ | — |
| `impl` | ✓ | — | — |
| `source` | ✓ | ✓ | — |
| `with-tests` | ✓ | ✓ | ✓ |
| `compact` | ✓ | — | — |
| `dense` | — | — | — |

## Everyday commands

```sh
# First look
srcpack arch

# One subsystem, full source
srcpack impl -t auth

# Mixed fidelity
srcpack "framework#dense" "app#source"

# Opt-in config context (secrets masked by default)
srcpack arch --include-config

# Prompt attached
srcpack "auth~tests#impl@find auth bugs"

# Only changed files
srcpack arch --modified
srcpack arch --staged
srcpack arch --since HEAD~3

# Full diff prepended
srcpack impl --modified --diff-only

# Tests matching local style
srcpack with-tests -t payments

# Tree only, no content
srcpack tree --tree-only

# Regenerate every target
srcpack update
srcpack update arch
srcpack update --check
```

## Views

```sh
srcpack "src#arch" "auth#source" --save-view auth-review
srcpack auth-review
srcpack view list
srcpack view show auth-review
srcpack view delete auth-review
```

## Setup

```sh
srcpack init                    # starter config + ignore file
srcpack init targets cmd internal pkg # starter config with named targets
srcpack init --force            # overwrite existing config
srcpack init --reset            # also clean _srcpack/
srcpack doctor                  # validate
```

## Config CRUD

```sh
srcpack target list
srcpack target show backend
srcpack target add auth internal/auth main.go
srcpack target delete auth

srcpack prompt list
srcpack prompt add security "Audit for auth bypasses."
srcpack prompt show security
srcpack prompt delete security

srcpack profile list
srcpack profile show arch

srcpack editor list
srcpack editor show zed
```

## Inspection

```sh
srcpack                          # dashboard
srcpack list                     # built-in profiles
srcpack info arch                # profile details
srcpack config path              # config file path
srcpack config show              # effective merged config
srcpack types                    # file-type aliases
srcpack . --ext json,yaml        # pass extension suffixes, not type labels
srcpack health                   # service status
srcpack demo                     # small AST demo
srcpack clean --dry-run          # preview cleanup
```

## All flags

```sh
# Selection
-t, --target PATH
-f, --file PATH
-p, --profile NAME
    --ext LIST
    --exclude LIST

# Transformation
    --dense
    --only-sig
    --comments / --no-comments
    --exported-only
    --mask / --mask-secrets / --no-mask
    --include-config
    --max-lines N
    --max-lines-head N
    --max-lines-tail N
    --trim-blank-lines
    --trim-indent

# Filters
    --no-tests / --tests
    --no-generated
    --no-deps
    --no-examples / --examples

# Prompt
    --prompt TEXT

# Views
    --save-view NAME
    --as NAME             (alias for --save-view)

# Output
-e, --editor NAME     markdown|zed|claude|fold|raw
-o, --output PATH     - for stdout
    --dir-prefix
    --color auto|always|never
-q, --quiet

# Tree
    --tree
    --tree-only
    --tree-files
    --full-tree
    --tree-depth N
-L, --depth N

# Git
    --modified, -m
    --staged
    --since REF
    --changed, --incremental
    --diff-only

# Control
    --check
    --cost, --roi
    --no-header
    --no-prompt
    --no-start
    --no-end
    --config PATH
-C, --cwd DIR
-c, --copy
    --save-view NAME
```

Rendered file blocks include a machine-readable `srcpack-source` comment with the original line range. Go signature profiles also include per-symbol line spans; symbol selection is not yet supported.

## Config file shape

```yaml
# nexss.srcpack.yml

prompts:
  security: "Audit for auth bypasses."
  review:   "Find logic errors and cite file paths."

options:
  strip_comments: true
  remove_body_func: true
  no_tests: true
  no_generated: true
  no_deps: true

targets:
  backend:
    dir:  [cmd, internal]
    files: [main.go]
  auth:
    dir: [internal/auth]
    targets: [backend]

profiles:
  arch:
    options: { exported_only: true }

views:
  auth-review:
    - "src#arch"
    - "internal/auth#source@review"
```

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Runtime error (pack failed, config invalid) |
| 2 | Usage error (bad flag, missing argument) |
