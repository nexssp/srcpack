# srcpack usability regression tests

Run the end-to-end tests with:

```sh
GOWORK=off go test -v ./tests/usability
```

The suite builds the CLI from the current checkout, selects committed fixture files using real selectors, saves output to temporary files, and compares the complete rendered contexts with the goldens in `testdata/golden/`. It checks Go implementation selection, the `with-tests` profile, a TypeScript source selection, and the selector exclusion form `path~pattern#profile`. It also rejects adjacent file blocks whose closing code fence is joined to the next heading.

The Go source and test file are snapshots from srcpack commit `f8cd790f041fba8ccf2e1bfe3a4ab1164c65d47d`; `testdata/project/PROVENANCE.md` records that source. The TypeScript and unrelated-source files are local deterministic fixtures. Tests need no remote repository or clipboard access.

## Size check

On this fixture, the focused mixed-language output is 7,422 bytes. The complete Go/TypeScript source corpus is 13,776 bytes, so the rendered context is about 53.9% of that baseline. The two selected raw files total 7,353 bytes; rendered framing adds 69 bytes (about 0.9%). Tests enforce both that the complete context stays below 80% of the fixture source corpus and that framing adds no more than 2% over the selected raw source files.

The approximate token figures divide bytes by 3.5 to match srcpack's existing size heuristic. This is a useful, consistent size proxy—not a guarantee for any particular model tokenizer. The test counts the complete saved context, including file labels and Markdown fences; it therefore makes framing overhead visible instead of treating source-body bytes as the entire prompt cost.

To refresh golden files after an intentional renderer change, first review the changed output, then run:

```sh
GOWORK=off UPDATE_GOLDEN=1 go test ./tests/usability
```
