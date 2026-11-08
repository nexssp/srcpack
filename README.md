# srcpack

<small>Nexss Flow 🌀 ready</small>

`srcpack` turns selected source code into one focused, AI-ready context. It can combine local folders, files, and public GitHub source in a single command, with a different detail profile for each selection. Use it to map a project, prepare a code review, or give an AI a clean slice of code without copying files by hand. It reads source; it does not rewrite it.

## Install and start using it

Install with Go 1.27.3 or newer:

```sh
go install github.com/nexssp/srcpack/cmd/srcpack@latest
```

Make sure `srcpack` is on your `PATH`, then run it from any project you want to inspect. No config file or `srcpack init` is needed:

```sh
cd /path/to/your/project
srcpack ".#arch"
srcpack cookbook
```

That creates a quick architecture map and copies it to the clipboard. Use `-o -` to print the result in your terminal or `-o FILE` to save it.
`srcpack cookbook` prints practical, copy-and-paste examples for selectors, line slices, Git changes, saved views, and configuration.

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

Use `--include-config` when a code map needs build, tooling, or CI context. It adds curated config and manifest files as raw text, with detected secret-like values masked; `.env` variants and common credential/private-key files remain excluded, even when explicitly selected. Masking is best-effort, not a guarantee that arbitrary secrets will be detected. Rendered file blocks also carry a machine-readable `srcpack-source` comment with original line ranges; Go signature profiles include symbol line spans there as well.

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
| 18 | `srcpack "bundles/pack#arch" -o -` | Print the result to stdout. |
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
