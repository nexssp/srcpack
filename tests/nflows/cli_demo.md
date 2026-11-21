# Srcpack Flow CLI demo

`cli_demo.nflow` shows the focused Flow path: a request schema defines the command surface, `tcli.listen` dispatches it, and the existing srcpack operators do the domain work.

## What it demonstrates

- `PackDemoInput` describes only the options this small demo actually executes.
- `@pipeline pack:cli=...:request-schema=PackDemoInput` declares the command, help text, and request schema.
- `srcpack.source -> ast.parse -> stats.collect -> srcpack.render -> stream.map(item.Content) -> collect -> bytes.concat` keeps rendered content as `[]byte` through collection and makes the join explicit in Flow.
- `tcli.listen` exposes the pipeline as a `pack` command.
- `bytes.concat` preserves chunk order and the exact byte sequence, returning a Go `string` that the CLI adapter writes as Markdown. Do not JSON-encode arbitrary binary output: invalid UTF-8 may be replaced with U+FFFD.

This is deliberately **not** a replacement for the full `srcpack` CLI: output-file policy, clipboard behavior, profile and selector resolution, workspace configuration, and CLI exit-code parity are outside this minimal example. See [`../examples/nflow-showcase/README.md`](../examples/nflow-showcase/README.md) for the runnable contributor example and its scope.

## Running it

The Flow source is embedded in `nflows.FS`. A host loads the srcpack operator bundle and the `transport/tcli/nexssflow` bundle, then runs the file from the workspace to package. The schema-request bridge is included in the published `github.com/nexssp/transport` `v0.19.1` dependency already pinned by this module.

Example arguments:

```text
pack --target ./cmd --target ./operators --only-sig
pack -t ./cmd -t ./operators --ext go
pack --file go.mod --file README.md --no-tests
```
