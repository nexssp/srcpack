# Contributing to Nexss

This repository is part of the **Nexss Ecosystem**.

We keep packages small, focused, and composable. Contributions should improve correctness, clarity, portability, or measured performance without expanding the public API unnecessarily.

## Before opening a pull request

Run:

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go test ./... -run '^$' -bench=. -benchmem -count=1
```

Install Task and the upstream Flow CLI used by the workflow checks:

```bash
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/nexssp/flow/cmd/nflow@main
```

For focused checks, use Task's package arguments after `--`:

```bash
task lint -- ./bundles/document ./cmd/srcpack
task lint:nflow
task nflow:fmt-check
```

`task lint` runs Go lint only, like the other Nexss repositories; the NFlow
checks are separate because they validate registered workflow sources rather
than Go packages. NFlow checks use the upstream `nflow` executable, not a
srcpack-specific linter.

## Requirements

- A change to a **public interface** requires a compatibility explanation and tests.
- A performance claim requires a benchmark on a documented Go version and hardware.
- New dependencies require a clear reason, license review, and evidence that the dependency is not better placed in an adapter module.
- Keep business rules, transports, hosted services, and provider-specific behavior outside this repository.

## Local pre-commit (optional)

Install pre-commit:

```bash
pre-commit install
# or
prek install
```

Hooks run before commit and push.

Thank you for contributing to Nexss.
