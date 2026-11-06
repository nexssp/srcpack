# Build once, extend in Flow

A small, runnable srcpack demo: turn source files into AI context, then add a report metric by editing NFlow rather than building another Go operator.

## Run it

From the repository root, with Go 1.27.3 or newer:

```sh
go run ./examples/flow-fold
```

The demo creates three tiny Go files in a temporary directory, excludes the test file, and extracts signatures from the other two. It cleans up on exit and never writes an output artifact, changes your working directory, or touches the clipboard.

## The story to show

**1. Build the useful system from small parts.** The demo reads the current embedded [`pack.nflow`](../../nflows/pack.nflow), not a copied fold expression:

```text
srcpack.source → ast.parse → pack.stats_input → pack.batch → stream.fold
```

srcpack owns workspace selection, parsing, and byte-oriented facts. Flow owns composition and the aggregation expression. The fold keeps the original file records alongside the totals, so they are still available to srcpack's renderer.

The fixture reports:

```json
{
  "fileCount": 2,
  "funcCount": 2,
  "packedBytes": 47,
  "rawBytes": 136
}
```

`packedBytes` measures extracted content/snippet bytes, not the final document's headings and fences. `Café` in a signature exercises UTF-8 byte counting.

**2. Extend the behavior without adding a Go operator.** Open [`report.nflow`](report.nflow). One `stream.map` after the fold preserves the file records and existing totals, then adds:

```text
retainedRecords:          2
nonContributingRecords:   0
packedBytesPerFile:      23.5
```

The per-file average is guarded against division by zero. Change the reporting expression and rerun `go run ./examples/flow-fold`; no Go changes or operator registration are needed. The `.nflow` fragment is embedded, so a previously built binary must be rebuilt to pick up edits.

**3. Keep the rest of the system working.** Both the original and extended envelopes go through `stats.collect → srcpack.render → collect → srcpack.join`. The runner checks that the rendered context is byte-for-byte identical. The console prints the two signatures, without test code or function bodies.

The point: reusable Go capabilities provide the building blocks; Flow makes their wiring and reporting policy visible and changeable in the DSL.

## What the harness changes

[`main.go`](main.go) keeps the production schema and aggregation stages verbatim, replacing the presentation tail in memory and omitting the workspace cache-saving stage. It runs the fold with and without the reporting extension and also renders both variants. It deliberately omits the production state-cache/file/clipboard side effects. This demo-only extraction is separate from the production CLI, which executes the unchanged root NFlow source with config bindings. A guarded replacement fails explicitly if the expected production pipeline seam changes.

This is a compositional demo of the current fold path, not a replacement CLI, a full end-to-end test of production output side effects, or a benchmark.

## Two important boundaries

`pack.batch` materializes the whole selected record stream before the fold. The fold's expression uses a linear reduction, but this path is **not constant-memory streaming**. Existing source guardrails remain responsible for scan limits.

`nonContributingRecords` means records retained in the fold envelope that did not contribute to its totals. It is **not the total number of files excluded during scanning**: upstream filters and the AST stage can discard records entirely. The excluded test fixture therefore does not increase this metric.

## Verify it

```sh
go test ./examples/flow-fold -v
```

Tests exercise the real Flow compiler and runner, unchanged production fold, original totals, Unicode byte lengths, DSL-only extension, identical rendering, empty workspaces, excluded-only workspaces, absence of output/clipboard stages, and unchanged process working directory.
