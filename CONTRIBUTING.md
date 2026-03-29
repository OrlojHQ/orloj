# Contributing

Thanks for contributing to Orloj.

Please note that this project is governed by a
[Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you agree to uphold it.

## Quick Links

- [Good first issue](https://github.com/OrlojHQ/orloj/issues?q=is%3Aissue%20is%3Aopen%20label%3A%22good%20first%20issue%22)
- [Help wanted](https://github.com/OrlojHQ/orloj/issues?q=is%3Aissue%20is%3Aopen%20label%3A%22help%20wanted%22)
- [Use-case templates contribution guide](./examples/use-cases/CONTRIBUTING.md)
- [Project roadmap](./ROADMAP.md)

## Before You Start

- Open an issue first for substantial changes.
- Keep pull requests focused and small when possible.
- Add tests for behavior changes and bug fixes.
- Keep docs and examples aligned with code changes.

Use GitHub issue forms for new work:

- `Bug report` for defects (includes reproduction fields).
- `Feature request` for enhancements.
- `Good first task` for scoped onboarding tickets.

## Local Development Setup

Prerequisites:

- Go `1.25+`
- Bun `1.3+` (for frontend/docs)

From repository root:

```bash
make ui-install
make ui-build
go build ./...
```

Start a local server with embedded worker:

```bash
go run ./cmd/orlojd \
  --storage-backend=memory \
  --task-execution-mode=sequential \
  --embedded-worker
```

In another terminal, run quick validation commands while developing:

```bash
go run ./cmd/orlojctl apply -f examples/blueprints/pipeline/ --run
go run ./cmd/orlojctl get task bp-pipeline-task
```

## Fast Test Matrix

Run the smallest relevant checks first, then run broader checks before review:

| Change type | Recommended command |
| --- | --- |
| Single package change | `go test ./path/to/package -count=1` |
| API/runtime touched | `go test ./api ./controllers ./store -count=1` |
| Frontend touched | `cd frontend && bun run build` |
| Docs touched | `cd docs && bun run build` |
| Examples/manifests touched | `go run ./cmd/orlojctl validate -f examples/` |
| Pre-PR full pass | `go test ./... -count=1 -timeout 120s` |

## Pull Request Expectations

- Use the PR template and complete every checklist item.
- Explain user-visible behavior changes and risk areas in the PR description.
- Add or update tests for behavior changes.
- Update docs/examples/changelog when applicable.
- Avoid unrelated refactors in feature or fix PRs.

## Review and Response SLA

We target a maintainer first response within **72 hours** for new PRs and issues.

If you do not receive a response in that window, post a short bump comment on the same thread.

## Changelog

Every PR that changes **user-visible** behavior should include a bullet under
`[Unreleased]` in [CHANGELOG.md](./CHANGELOG.md). Use the appropriate section:
**Added**, **Changed**, **Deprecated**, **Removed**, **Fixed**, or **Security**
([Keep a Changelog](https://keepachangelog.com/) format).

You do **not** need a changelog entry for internal refactors, test-only changes,
or CI and tooling updates that do not affect users or operators.

## Commit Sign-off (DCO)

By contributing, you certify the Developer Certificate of Origin (DCO).

Each commit must include a sign-off line:

```text
Signed-off-by: Your Name <you@example.com>
```

You can add this automatically with:

```bash
git commit -s
```

## License

By submitting a contribution, you agree that your contribution is licensed
under the Apache License 2.0 in this repository.
