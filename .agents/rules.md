# Orloj Agent Rules

This file is shared guidance for AI coding agents. Tool-specific entrypoints such as `AGENTS.md`, `CLAUDE.md`, and `.cursor/rules/*.mdc` should stay consistent with these rules.

## CRDs

Run `make generate-crds` when modifying Go types that feed Kubernetes CRD schemas:

- `crds/**/*_types.go`
- `crds/groupversion_info.go`
- `resources/resource_types.go`
- `resources/agent.go`
- `resources/model_endpoint.go`

Commit the generated YAML under `config/crd/bases/` with the source change. CI verifies this by rerunning the generator and checking `git diff --exit-code config/crd/bases/`.

The Helm chart also embeds CRDs in `charts/orloj/templates/operator-crds.yaml`. When CRD fields change, keep the Helm copy in sync as part of the same PR.

Runtime-only files such as `crds/reconciler.go`, `crds/convert.go`, and `crds/status_writer.go` usually do not require CRD regeneration unless they also change schema types or tags.

## OpenAPI

Update `openapi/` when the API-visible wire shape changes:

- HTTP routes, methods, query params, status codes, request bodies, or response bodies under `api/`.
- Serialized resource fields in `resources/` that users submit or receive through the API or `orlojctl apply`.
- CLI changes that introduce or alter HTTP API contracts.

Run:

```bash
npx --yes @redocly/cli@1.28.5 lint openapi/openapi.yaml
```

OpenAPI lint validates the spec structure. It may not catch missing fields after a Go resource schema change, so compare the relevant files under `openapi/schemas/` manually.

## Changelog

For user- or operator-visible changes, add a bullet under `## [Unreleased]` in `CHANGELOG.md`.

Use the existing Keep a Changelog sections: `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, and `Security`. Do not duplicate an existing entry; extend it when appropriate.

Skip changelog entries for internal refactors, test-only edits, and CI/tooling-only changes that do not affect users.

## Tests

Use targeted tests while iterating. Before handing off broad code changes, run:

```bash
go test ./... -count=1 -timeout 120s
```

For CRD work, also run or account for:

```bash
make generate-crds
go test ./crds/... -count=1 -timeout 120s
```

For OpenAPI work, run the Redocly lint command above.

## General Editing

- Follow existing package and naming patterns.
- Prefer small, focused changes over opportunistic refactors.
- Keep generated artifacts in the same commit as the source changes.
- Do not remove or rewrite contributor work unless the task requires it.
- If a change touches multiple release surfaces, mention all of them in the handoff.
