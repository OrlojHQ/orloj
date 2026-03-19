# Documentation Workspace

This directory is the canonical source for the Vocs documentation site.

## Structure

- `pages/index.md`: docs landing page.
- `pages/getting-started/`: onboarding (install, quickstart).
- `pages/concepts/`: conceptual explanations and mental model.
- `pages/architecture/`: system architecture and execution model docs.
- `pages/reference/`: resource and API reference docs.
- `pages/operations/`: deployment, runtime, and production operations docs.
- `pages/phases/`: phase-by-phase implementation history.

## Site Framework

- Framework: [Vocs](https://vocs.dev/docs).
- Config: `vocs.config.ts` (repo root), with `rootDir: docs`.

## Local Preview

From repository root:

```bash
bun install
bun run docs:dev
```

Build static docs:

```bash
bun run docs:build
```

## Authoring Guidelines

- Keep pages in Markdown (`.md`) with stable headings.
- Prefer linking to source files and API paths directly.
- Put new feature docs in both:
  - a focused page in `pages/architecture/`, `pages/reference/`, or `pages/operations/`
  - an entry in `pages/phases/phase-log.md`
- Keep examples runnable from repository root.
- Versioning convention: update `v1` docs/contracts in place unless a new major is explicitly approved.
- Treat `docs/pages/*.md` as the only docs source-of-truth.

## Planned Site Targets

This layout now targets Vocs-first publishing for OSS launch hardening.
