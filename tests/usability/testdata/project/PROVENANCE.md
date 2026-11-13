# Usability corpus provenance

`go/output_actions.go` and `go/output_actions_test.go` are byte-for-byte snapshots from `github.com/nexssp/srcpack` commit `f8cd790f041fba8ccf2e1bfe3a4ab1164c65d47d` (`fix: address srcpack review findings`). The exact commit is recorded here so the regression inputs do not move with upstream `main`.

The TypeScript client, its test, `experimental.ts`, and `other/unused.go` are small local fixtures authored for this suite. They exercise a second language, test-file filtering, path-pattern exclusion, and selection of a small relevant slice from a larger source set. They are not claimed to be upstream snapshots.
