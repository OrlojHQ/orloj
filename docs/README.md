# Orloj Docs

This directory is the canonical source for the Vocs documentation site.

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

## Authoring Guidelines

- Keep pages in Markdown (`.md`) with stable headings.
- Prefer linking to source files and API paths directly.
- Put new feature docs in both:
  - a focused page in `pages/architecture/`, `pages/reference/`, or `pages/operations/`
  - an entry in `pages/phases/phase-log.md`
- Keep examples runnable from repository root.
- Versioning convention: update `v1` docs/contracts in place unless a new major is explicitly approved.
- Treat `docs/pages/*.md` as the only docs source-of-truth.
