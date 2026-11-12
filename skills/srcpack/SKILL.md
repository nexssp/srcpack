---
name: srcpack
description: Build focused source context with srcpack for repository orientation, symbol and text discovery, implementation investigation, change analysis, and code review. Use when an agent needs a selective view of local or public GitHub source instead of reading a whole repository.
---

# srcpack

Use the existing `srcpack` CLI to find and package only the source relevant to the task. Packing reads source; it does not edit it. One-off selectors need no `srcpack init` or config file. Do not install tools or create persistent configuration just to use this skill.

## Consult the built-in cookbook

On first use, run `srcpack cookbook` for the terminal-formatted guide. Use `srcpack cookbook --md` when raw Markdown is useful. Consult the current built-in examples before guessing about selectors, workflows, or config commands.

## Find, inspect, then package

1. Run from the intended repository and check `srcpack --help` if CLI syntax is uncertain. If `srcpack` is unavailable, say so and use focused repository-native tools; do not install it without authorization.
2. Find a declaration or matching source line before building a broad context. `--root` is the directory to search; `--kind text` searches matching lines, and `--json` emits machine-readable results.

   ```sh
   srcpack find "LoadConfig" --root ./internal --kind func
   srcpack find "access denied" --root ./internal --kind text --json
   ```

3. Open the reported paths and relevant tests to verify the actual behavior. Then package the smallest useful scope; line ranges are local-file, 1-based and inclusive.

   ```sh
   srcpack "internal/auth#arch" --stdout
   srcpack "internal/auth/service.go:40-125#source" "internal/auth#with-tests" --stdout
   srcpack "internal/auth#api" "internal/session#source" -o /tmp/auth-context.md
   ```

Use `--stdout` for the complete rendered package in the terminal, or `-o FILE` to save it. A normal run may show a shortened terminal preview; do not mistake that preview for the full package.

## Validate source changes

For code changes, run the focused Go linter `golangci-lint run --timeout=5m` and relevant tests when appropriate; do not run the full test suite unless asked. Compare diagnostics with a clean current/upstream baseline, and report pre-existing unrelated warnings instead of silently ignoring or fixing them outside scope.

## Choose a profile and selector

Profiles control detail. Use `arch` for a broad structural map, `api` for commented public interfaces, `impl` for focused implementation bodies, `source` for selected source with comments, and `with-tests` when behavior and tests matter. `source` omits tests; `with-tests` includes them. Check `srcpack list` and `srcpack info NAME` for the complete built-in set and current definitions.

A selector follows this form; quote it so the shell preserves `#`, `~`, and `@`:

```text
<path-or-github-source>[~exclude-patterns][#profile][@prompt]
```

Paths may be files, directories, comma-separated local paths, or local-file line ranges such as `file.go:20-60`. Directory selections recurse. Combine multiple selectors in one command and assign different profiles where useful. For public GitHub source, use `github:OWNER/REPO[//PATH][?ref=REF]`; private-repository credentials are not forwarded.

For change review, `--modified`, `--staged`, or `--since REF` narrows local selection; add `--diff-only` when the Git diff itself is needed. Use `srcpack pack SELECTOR --plan` to inspect a selection before scanning. Do not combine local-change selection flags with remote GitHub selectors.

## Keep configuration and output safe

Use one-off selectors by default; config is optional. To inspect existing setup, use `srcpack config path`, `srcpack config show`, `srcpack view list`, `srcpack target list`, and `srcpack prompt list`; use `srcpack config validate` or `srcpack doctor` when validation matters. `srcpack init`, `--save-view`, and view/target/prompt add or delete commands persist workspace config; create or change it only when requested or clearly useful for repeatable work. `--include-config` is opt-in. Secret masking is best-effort, so inspect generated context for credentials and sensitive content before sharing it outside the working context.

## Current references

When options or syntax have changed, trust the installed CLI's `srcpack --help` and cookbook, then verify against the current upstream docs:

- [README](https://github.com/nexssp/srcpack/blob/main/README.md)
- [Selector cheat sheet](https://github.com/nexssp/srcpack/blob/main/docs/cheatsheet.md)
- [Cookbook](https://github.com/nexssp/srcpack/blob/main/bundles/system/cookbook.md)
