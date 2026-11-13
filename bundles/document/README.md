# Copyable document bundle

Copy this **entire directory**, including `document.nflow`, into another Go repository. Change the consumer's Go import to its new local package path. The implementation imports only Flow and kernel; it does not import srcpack, its CLI, parser, workspace configuration or output effects. Use Flow/kernel v0.35.0 or a compatible version.

`document.nflow` owns Markdown/Zed/Claude/fold/raw presets and aliases, prompt/diff/tree ordering, document/file delimiters, snippet spacing, suppression flags, concatenation and emitted-chunk counts. `library.go` supplies only a generic, byte-preserving `${KEY}` interpolation primitive and an embedded source loader.

## Mount and execute

The host imports `context`, Flow's `compiler`, `native`, `runner`, and the copied `document` package:

```go
cfg, err := runner.BuildConfig(append(native.Bundles(), document.Bundle(nil)))
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

`execution.Output` is a map containing `content` (string) and `items` (int). A registry factory is also registered as `document`; `@require document` identifies that bundle, but **does not include its NFlow source or install a source loader**. The example explicitly mounts the bundle and provides its loader. A larger host should compose loaders or materialize the included source beside its own files.

For composition inside another declared pipeline, use the public `@document_format()` macro after a **value boundary** (`collect` when upstream is a stream). This avoids dependency on the order in which Flow 0.35.0 materializes named pipelines:

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

The built-in file presets use `${SOURCE_RANGE}` in their path header; callers should provide it in each file's `values` map (empty for whole files, or `:N`/`:N-M` for a slice). Custom templates may also use `${SOURCE_URL}` for a source link. Unknown/malformed `${KEY}` tokens remain literal. Substitution is single-pass: inserted values, source content, prompts, diffs and tree text are never parsed as NFlow. `items` counts document/file chunks, not bytes or interpolation segments; a nonempty template that expands to empty still counts as one chunk. This distinction lets callers avoid clearing a clipboard for a truly empty stream.

## Generic primitive

`document.interpolate` accepts `{blocks: [{parts: [{text, values?, expand?, newline?}]}]}` and returns `{blocks: []string}`. `expand: true` enables one-pass placeholder substitution. `newline: true` appends a newline only to a nonempty segment not already ending in one. Each block's parts are concatenated without additional separators. Invalid types fail, and cancellation propagates.

The workflow has no filesystem discovery, state-cache writes, stdout or clipboard effects. Those remain the caller's responsibility. Run the bundled Go tests with `go test ./path/to/document`; they mount only native Flow and this bundle.
