# Orloj Agent Instructions

These instructions are for Codex and other OpenAI-style coding agents working in this repository. Also read [.agents/rules.md](.agents/rules.md) before making changes.

## Must-Follow Sync Rules

- If you change CRD-backed resource schemas in `crds/**/*.go`, `resources/resource_types.go`, `resources/agent.go`, or `resources/model_endpoint.go`, run `make generate-crds` and commit the resulting `config/crd/bases/` changes.
- If a CRD schema changes, check whether the embedded Helm CRD copy in `charts/orloj/templates/operator-crds.yaml` must be updated too.
- If an API-visible resource field, route, request body, or response body changes, update `openapi/` and run `npx --yes @redocly/cli@1.28.5 lint openapi/openapi.yaml`.
- If the change affects users or operators, add or update a bullet under `## [Unreleased]` in `CHANGELOG.md`.
- Run the narrowest useful tests while iterating, and run `go test ./... -count=1 -timeout 120s` before handing off broad code changes when feasible.

## Working Style

- Prefer existing package patterns and helpers over new abstractions.
- Keep generated artifacts committed with the source changes that caused them.
- Do not make unrelated refactors while fixing a focused issue.
- When reviewing contributor PRs, call out generated-file drift and release artifact drift explicitly.
