# Documentation Workspace

This directory is the canonical source for the Vocs documentation site.
It is fully self-contained: `package.json`, `vocs.config.ts`, and `vercel.json` all live here.

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
- Config: `vocs.config.ts` in this directory, with `rootDir: '.'`.

## Local Preview

From the `docs/` directory:

```bash
bun install
bun run dev
```

Build static docs:

```bash
bun run build
```

## Vercel

Vercel Root Directory is set to `docs/` so the repo's top-level Go `api/` package is not treated as Vercel serverless routes. All install, build, and output settings are in `vercel.json`.

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
