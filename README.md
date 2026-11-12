# srcpack

<small>Nexss Flow 🌀 ready</small>

`srcpack` turns selected source code into one focused, AI-ready context. It can combine local folders, files, and public GitHub source in a single command, with a different detail profile for each selection. Use it to map a project, prepare a code review, or give an AI a clean slice of code without copying files by hand. It reads source; it does not rewrite it.

![Summary of the Nexss Srcpack nexssp/srcpack](https://github.com/user-attachments/assets/b04cefb3-d61d-4e19-9a50-cb88ad028516)

## Install and start using it

### Install a prebuilt release (recommended)

Tagged releases include prebuilt archives and installers. If a tag does not have release assets yet, use the Go install fallback below.

Linux or macOS:

```sh
curl -fsSL https://github.com/nexssp/srcpack/releases/latest/download/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://github.com/nexssp/srcpack/releases/latest/download/install.ps1 | iex
```

The installers verify the downloaded archive against the release's SHA-256 checksum file, install without administrator privileges, and support a pinned version and custom install directory. The checksum detects download corruption; it is not a publisher signature.

### Optional skill for AI agents

The binary installers install the `srcpack` CLI, not agent-specific instructions. If your AI agent supports Agent Skills, use either option:

- Run `srcpack skill` (if supported by your installed CLI) to get links matching that CLI version.
- [View the current skill on GitHub](https://github.com/nexssp/srcpack/blob/main/skills/srcpack/SKILL.md) or [download the raw `SKILL.md`](https://raw.githubusercontent.com/nexssp/srcpack/main/skills/srcpack/SKILL.md) directly. These URLs follow `main`; use the command when you need a version-matched skill and your CLI supports it.

Add the downloaded `SKILL.md` according to your AI agent's instructions. The skill is optional and separate from the CLI.

### Install with Go (fallback)

If you prefer to install from source, or a release does not offer a prebuilt installer for your platform, use Go 1.27.4 or newer, as required by this release's `go.mod`:

```sh
go install github.com/nexssp/srcpack/cmd/srcpack@latest
```

Make sure `srcpack` is on your `PATH`, then run it from any project you want to inspect. No config file or `srcpack init` is needed:

```sh
cd /path/to/your/project
srcpack ".#arch"
srcpack cookbook
```

`srcpack ".#arch"` creates a quick architecture map. With no destination, it copies the full package to the clipboard and shows a compact summary with a bounded preview; only the terminal preview may be shortened. Use `--stdout` to print the full rendered package, or `-o FILE` to save it. Do not combine `--stdout` with `-o FILE`; with no destination, `--no-copy` prints the full package to stdout.

`srcpack cookbook` prints practical, copy-and-paste examples for selectors, line slices, Git changes, saved views, and configuration in a colorized terminal layout. Use `srcpack cookbook --md` to print the cookbook as pure Markdown without terminal styling or banners.

## Combine local and remote code in one command

Each quoted selector contributes to the same context package. Local paths are relative to your current working directory: `./bundles/pack` is inside it, while `../flow/compiler` is in its parent directory. In this example, run from a local srcpack checkout with a Flow checkout alongside it; replace either local path with folders from your own workspace.

```sh
srcpack \
  "./bundles/pack#impl" \
  "../flow/compiler#api" \
  "github:nexssp/flow//compiler?ref=v0.70.0#arch" \
  "github:nexssp/kernel//action?ref=v0.37.0#api" \
  -o combined-context.md
```

That single command combines two local folders and two selected folders from public GitHub repositories. Each selector gets its own profile, so you can keep the context broad for one part and detailed for another. `srcpack` works with any public GitHub repository; the examples here use Nexss repos because they’re easy to try together.

## Select only the detail you need

A selector has this form:

```text
<local-path-or-github-source>[~exclude-patterns][#profile][@prompt]
```

Quote each selector in your shell. Paths can be directories, files, comma-separated local paths, or supported file line ranges. Remote selectors use `github:OWNER/REPO`, optionally followed by a selected path and a ref:

```text
github:OWNER/REPO[//PATH][?ref=REF]
```

For a branch, tag, or commit, add `?ref=`. If you omit it, srcpack uses the repository’s current default revision. Remote source is fetched temporarily for the command; no separate clone is needed. Git must be installed, and remote repositories must be public. Private-repository credentials are not forwarded. Run `srcpack cookbook` for working remote examples and limitations.

Directory selectors include files from that folder recursively, while the summary tree shows the selected folder as one top-level entry at the default depth. Increase `--tree-depth` to show additional levels under every selected folder, or append `/...` to expand just one folder recursively in the summary. This changes only the tree display, not which files are packed; Windows paths can use `\...` as the suffix.

```sh
srcpack "../flow#source" "../../nexss_ecosystem/kernel#arch" "../../nexss_ecosystem/transport#arch"
srcpack "../../nexss_ecosystem/kernel/...#arch" "../flow#source"
```

### Profiles

Profiles control how much source goes into the package. These descriptions match the built-in definitions:

| Profile | What it includes | Good for |
|---|---|---|
| `arch` | Exported declarations and signatures; no bodies or comments. | Mapping a project’s public structure. |
| `api` | Exported declarations and signatures with comments; no bodies. | Reviewing public interfaces and API docs. |
| `impl` | Implementation bodies, with comments stripped by default. | Inspecting a focused implementation. |
| `source` | Selected source with bodies and comments; tests are omitted. | Reading implementation details. |
| `with-tests` | Source and tests, including comments. | Understanding behavior alongside implementation. |
| `compact` | Trimmed exported signatures, without bodies or comments. | Keeping structural context small. |
| `dense` | Exported signatures in compressed, signature-oriented output; no bodies or comments. | Very compact structural context. |

These profiles skip generated and dependency files, along with example directories, by default. `source` means full selected non-test source, not every file in a repository. Use `srcpack list` to see built-in profiles and `srcpack info NAME` to inspect one. A selector’s `#profile` can be mixed with other profiles in the same command; `-p/--profile` supplies a default for the whole run.

Use `--include-config` when a code map needs build, tooling, or CI context. It adds curated config and manifest files as raw text, with detected secret-like values masked; `.env` variants and common credential/private-key files remain excluded, even when explicitly selected. Secret masking is enabled by default; `--mask`/`--mask-secrets` keep it on, while `--no-mask` disables it. Masking is best-effort, not a guarantee that arbitrary secrets will be detected. Formatted file blocks carry a machine-readable `srcpack-source` comment before each file header; raw, none, and text modes remain unchanged. The comment includes an optional commit-pinned GitHub `url` with `#Lx-Ly` anchors for remote sources, so citations remain clickable and stable even when the default branch moves. Its optional `line_spans` maps 1-based lines in that file's rendered body to original-file `source_start_line`/`source_end_line` ranges; these are per-file body coordinates, never positions in the concatenated pack. Blank, synthetic, ambiguous, or otherwise unmapped lines have no entry. Go signature profiles also include original declaration spans in `symbols`.

## 20 useful commands before you configure anything

Run these from a local srcpack checkout. Replace local folders with the ones in your workspace. Commands 1–20 work without `nexss.srcpack.yml`.

| # | Command | Use it to… |
|---:|---|---|
| 1 | `srcpack ".#arch"` | Map the current project. |
| 2 | `srcpack ".#api"` | See public signatures with comments. |
| 3 | `srcpack "bundles/pack#impl"` | Inspect srcpack’s pack implementation. |
| 4 | `srcpack "nflows#source"` | Read the NFlow pipelines in full. |
| 5 | `srcpack "bundles/pack#with-tests"` | Include pack tests with its source. |
| 6 | `srcpack "README.md#source" "go.mod#source"` | Package specific files together. |
| 7 | `srcpack "bundles/pack,nflows#arch"` | Map multiple directories at once. |
| 8 | `srcpack "bundles/pack#arch" "nflows#source"` | Mix detail levels in one result. |
| 9 | `srcpack "bundles/pack~*_test.go#impl"` | Exclude matching test files. |
| 10 | `srcpack "bundles/pack#impl" --prompt "Review input validation and errors."` | Add a one-off prompt to the rendered context. |
| 11 | `srcpack tree "bundles/pack"` | Inspect the selected directory tree. |
| 12 | `srcpack tree "bundles/pack" --tree-files` | Show file names in the tree. |
| 13 | `srcpack ".#arch" --modified` | Include changed and untracked local files. |
| 14 | `srcpack ".#arch" --staged` | Include staged local changes. |
| 15 | `srcpack ".#arch" --since HEAD~3` | Select local changes since a Git ref. |
| 16 | `srcpack ".#impl" --modified --diff-only` | Add the local diff to changed source. |
| 17 | `srcpack "nflows#source" -o nflows-context.md` | Save the result to a file. |
| 18 | `srcpack "bundles/pack#arch" --stdout` | Print the full rendered result to stdout. |
| 19 | `srcpack pack "bundles/pack#arch" --plan` | Preview selectors and destination before scanning. |
| 20 | `srcpack "./bundles/pack#impl" "../flow/compiler#api" "github:nexssp/flow//compiler?ref=v0.70.0#arch" "github:nexssp/kernel//action?ref=v0.37.0#api" -o combined-context.md` | Combine current, parent, and remote project code. |

These cover common one-off tasks: understand a project, inspect an API, focus on one implementation, include tests, filter changed files, compare local and remote code, and choose where the context goes.

## Save selections you repeat

Once a one-off selector is useful, save it as a named view. This writes config so you can reuse the selection later; it’s optional.

```sh
srcpack "./bundles/pack#impl" "../flow/compiler#api" --save-view flow-review
srcpack flow-review
srcpack view list
srcpack view show flow-review
```

Run `srcpack init` when you want reusable workspace configuration such as named targets, prompts, editors, or profiles. `srcpack doctor` checks that configuration. Run `srcpack --help` for the command reference, `srcpack cookbook` for practical workflows, or read the [selector cheat sheet](docs/cheatsheet.md) and [cookbook source](bundles/system/cookbook.md).
