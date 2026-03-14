# Documentation Workspace

This directory is the canonical source for future documentation site content.

## Structure

- `index.md`: docs landing page.
- `architecture/`: system architecture and execution model docs.
- `reference/`: CRD and API reference docs.
- `operations/`: deployment, runtime, and production operations docs.
- `phases/`: phase-by-phase implementation history.

## Authoring Guidelines

- Keep pages in Markdown (`.md`) with stable headings.
- Prefer linking to source files and API paths directly.
- Put new feature docs in both:
  - a focused page in `architecture/`, `reference/`, or `operations/`
  - an entry in `phases/phase-log.md`
- Keep examples runnable from repository root.
- Versioning convention: update `v1` docs/contracts in place unless a new major is explicitly approved.

## Planned Site Targets

This layout is designed to map cleanly to MkDocs, Docusaurus, or mdBook later.
