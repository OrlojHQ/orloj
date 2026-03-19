# Release Process

This page defines the required steps for preparing and publishing Orloj releases.

## Versioning Scheme

- Orloj releases use semantic versioning with a `v` prefix: `vMAJOR.MINOR.PATCH`.
- MAJOR: breaking changes with migration notes.
- MINOR: backward-compatible features.
- PATCH: bug and security fixes.

## Artifact Destinations

- Container images: GitHub Container Registry (GHCR), using tags that match the git release tag.
  - `ghcr.io/orlojhq/orloj-orlojd:<version>`
  - `ghcr.io/orlojhq/orloj-orlojworker:<version>`
- Downloadable release artifacts (binaries, checksums, SBOM/provenance): GitHub Releases.
  - `https://github.com/OrlojHQ/orloj/releases`

## Release Inputs

- passing core test suite
- passing contract and runtime conformance suites
- passing documentation build and link checks
- passing compatibility smoke checks against pinned consumer references

## Packaging Requirements

- reproducible build inputs
- generated SBOM per release artifact
- signed binaries/images
- provenance/attestation metadata

## Release Checklist

1. Freeze release scope and changelog entries.
2. Run reliability validation (`orloj-loadtest`, `orloj-alertcheck`).
3. Run upgrade/canary verification in staging.
4. Complete security review and disclosure readiness checks.
5. Publish release notes and migration notes.
6. Tag and publish signed artifacts.

## Post-Release

- monitor early adoption signals and critical error reports
- publish hotfix plan for regressions when required
- track deferred work in [Roadmap](../phases/roadmap.md)
