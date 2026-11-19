# Copyable document bundle

Copy this **entire directory**, including `document.nflow` and `interpolation.nflow`, into another Go repository. Change the consumer's Go import to its new local package path. The Go package only embeds and loads the NFlow sources; it imports no srcpack domain types, parser, workspace configuration or output effects. The workflow uses Flow 0.89.0 or a compatible release.

`document.nflow` owns Markdown/Zed/Claude/fold/raw presets and aliases, prompt/diff/tree ordering, document/file delimiters, snippet spacing, suppression flags, concatenation and emitted-chunk counts. `interpolation.nflow` owns one-pass `${KEY}` expansion and block concatenation in a typed-output NFlow pipeline. The Go package is limited to the embedded source loader and a no-action compatibility bundle.

## Mount and execute

The host imports `context`, Flow's `compiler`, `native`, `runner`, and the copied `document` package:

```go
cfg, err := runner.BuildConfig(native.Bundles())
if err != nil { return err }
ctx := compiler.WithSourceLoader(context.Background(), document.Loader())
input := map[string]any{
    "config": map[string]any{"editor": "zed"},
    "files": []any{map[string]any{
        "content": "package main\n",
        "snippets": []string{},
        "omitted": false,
        "values": map[string]string{"REL_PATH": "main.go", "LANG": "go"},
    }},
}
execution, err := runner.Execute(ctx, cfg,
    "@include \"document.nflow\"\npipeline.document_render",
    "consumer.nflow", input)
```

`execution.Output` is a map containing `content` (string) and `items` (int). `document.Bundle()` remains registered as a no-action compatibility bundle for existing Go hosts, but this workflow does not need `@require document` or a Go interpolation action. The bundle does not include NFlow source or install a source loader; `document.Loader()` resolves both embedded files. A larger host may compose this loader with its own namespaced source loader.

For composition inside another declared pipeline, use the public `@document_format()` macro after a **value boundary** (`collect` when upstream is a stream):

```nflow
@include "document.nflow"

@pipeline my_document
  { config: { editor: "raw" }, files: [], values: {}, tree: "" }
  -> @document_format()
@end

pipeline.my_document
```

## Input contract

Top-level fields are `config` (object, default `{}`), `values` (document placeholder string map, default `{}`), `tree` (precomputed text, default `""`), and `files` (array, default `[]`).

Formatting options in `config` are `editor`, `prompt`, `start`, `end`, `file_start`, `file_end`, `diff_content` (strings), plus `no_header`, `no_prompt`, `no_start`, `no_end` (booleans). Missing options default to empty strings/false. Metadata such as profile names, totals and Git labels is supplied through `values`, not interpreted as product-specific configuration by this bundle.

Each file supplies `content` (a Go string, including arbitrary bytes), `snippets` (string array, possibly nil), `values` (file placeholder string map, possibly nil), and `omitted` (bool). Content takes precedence over snippets. Omitted files and files with no content/snippets are skipped. Whitespace-only snippets can still represent an emitted empty file block, preserving legacy chunk counts.

The built-in file presets use `${SOURCE_RANGE}` in their path header; callers should provide it in each file's `values` map (empty for whole files, or `:N`/`:N-M` for a slice). Custom templates may also use `${SOURCE_URL}` for a source link. Unknown and unterminated placeholders remain literal. Substitution is single-pass: inserted values, source content, prompts, diffs and tree text are never parsed as NFlow. `items` counts document/file chunks, not bytes or interpolation segments; a nonempty template that expands to empty still counts as one chunk. This distinction lets callers avoid clearing a clipboard for a truly empty stream.

## Interpolation pipeline

The included `interpolation.pipeline.document_interpolate()` pipeline accepts the internal block/part shape `{blocks: [{parts: [{text, values?, expand?, newline?}]}]}` and returns a typed `{blocks: []string}` result. `expand: true` enables one-pass placeholder substitution; missing `expand` and `newline` flags default to false. Unknown, malformed and unterminated tokens are preserved. `newline: true` appends a newline only to a nonempty part not already ending in one, and parts within a block concatenate without separators.

The pipeline intentionally uses an output-only typed signature: Flow request-schema validation JSON-normalizes string input and would replace invalid UTF-8 in source content. Its response schema validates the output shape without replacing the returned bytes. The renderer keeps source text out of JSON coercion so embedded NUL and invalid UTF-8 remain intact.

The workflow has no filesystem discovery, state-cache writes, stdout or clipboard effects. Those remain the caller's responsibility. Run the bundled Go tests with `go test ./path/to/document`; they use native Flow plus the package's embedded source loader.
