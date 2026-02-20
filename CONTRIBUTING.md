# Contributing

Thanks for contributing to `ew`.

## Development setup

Requirements:
- Go `1.25` or `1.26`
- `make`

Build:

```bash
make build
```

Run checks:

```bash
make fmt
make test
make vet
go test -race ./...
go test -race ./cmd/_ew
```

## Pull request rules

1. Keep changes focused and atomic.
2. Add or update tests for behavior changes.
3. Keep docs in sync (`README.md`, `docs/*`).
4. Do not add AI signatures/co-authored tags to commit messages.
5. Keep user-facing UX intuitive (natural language first for `ew`).

## Release docs

- Release runbook: `docs/RELEASING.md`
- Operational checklist: `docs/RELEASE-CHECKLIST.md`
- Homebrew policy and commands: `docs/HOMEBREW.md`

## Architecture notes

- Public interface: `ew`
- Internal deterministic helpers: `_ew`
- Provider integrations: `internal/provider`
- OS paths/config: `internal/appdirs`, `internal/config`

Add a new provider by:
1. Adding config under `providers.<name>`.
2. Reusing `type = "command"` where possible.
3. Implementing and registering a new adapter only when required.

## Reporting bugs

Use GitHub issues for normal bugs/features.
For security issues, follow `SECURITY.md`.
