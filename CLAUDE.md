# Orloj Claude Instructions

These instructions are for Claude Code and Claude-based agents working in this repository. Also read [.agents/rules.md](.agents/rules.md) before making changes.

## Critical Repository Rules

- CRD-backed resource schema changes require `make generate-crds` and committed updates under `config/crd/bases/`.
- Helm embeds CRDs in `charts/orloj/templates/operator-crds.yaml`; keep that copy in sync when CRD fields change.
- API-visible resource, route, request, or response changes require OpenAPI updates under `openapi/` and `npx --yes @redocly/cli@1.28.5 lint openapi/openapi.yaml`.
- User/operator-visible changes require a `CHANGELOG.md` entry under `## [Unreleased]`, unless an existing entry already covers the change.
- Run relevant Go tests, and prefer `go test ./... -count=1 -timeout 120s` before handing off broad changes.

## Implementation Notes

- Follow existing Orloj package conventions.
- Keep changes scoped to the task.
- Commit generated outputs with their source changes.
- When unsure whether a schema or generated artifact is affected, inspect the diff after running the generator or linter instead of guessing.
