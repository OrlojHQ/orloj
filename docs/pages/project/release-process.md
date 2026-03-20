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
- Each image is also tagged `latest` on every `v*` release (prefer an explicit version tag in production).
- Downloadable release artifacts (archives per OS/arch, `checksums.txt`, SBOM): GitHub Releases.
  - `https://github.com/OrlojHQ/orloj/releases`

## Automated release (maintainers)

Pushing an annotated or lightweight tag matching `v*` on the default branch triggers [`.github/workflows/release.yml`](../../../.github/workflows/release.yml).

1. Ensure `main` (or the release branch) is green on CI and release criteria below are satisfied.
2. Create the git tag: `git tag vX.Y.Z` and `git push origin vX.Y.Z`.

The workflow then:

1. **verify** — same checks as CI: Bun frontend build, `go build ./...`, `go test`, `go vet`.
2. **goreleaser** — builds `orlojd`, `orlojworker`, and `orlojctl` for Linux, macOS, and Windows (`amd64` / `arm64` where applicable); uploads archives and `checksums.txt` to the GitHub Release; generates the changelog from git commits (see [.goreleaser.yaml](../../../.goreleaser.yaml)).
3. **SBOM** — Syft SPDX JSON for the repository tree is attached to the release as `orloj-<tag>-sbom.spdx.json` (via `anchore/sbom-action`).
4. **images** — multi-arch (`linux/amd64`, `linux/arm64`) images for `orlojd` and `orlojworker` are pushed to GHCR with the tag and `latest`; Docker BuildKit **provenance** and **SBOM** attestations are generated; images are **keyless-signed with Cosign** (Sigstore/OIDC).

### Repository settings

- **GHCR**: New packages may start private; set each package visibility to **public** for OSS pulls without auth, or document login for private registries.
- **Actions**: The workflow needs permission to create releases, upload assets, push packages, and request an OIDC token for Cosign (`contents: write`, `packages: write`, `id-token: write`, `attestations: write`).

### Verifying container signatures

After install [Cosign](https://docs.sigstore.dev/cosign/overview/), verify an image by digest or tag (example for `v0.1.0`):

```bash
cosign verify "ghcr.io/orlojhq/orloj-orlojd:v0.1.0" \
  --certificate-identity-regexp 'https://github.com/OrlojHQ/orloj/' \
  --certificate-oidc-issuer-regexp 'https://token.actions.githubusercontent.com'
```

Adjust `OrlojHQ/orloj` if you fork the repository (GHCR image names use a lowercased GitHub owner).

### Release archive names

GoReleaser publishes one archive per binary and platform, for example:

- `orlojd_v0.1.0_linux_amd64.tar.gz`
- `orlojctl_v0.1.0_darwin_arm64.tar.gz`
- `orlojworker_v0.1.0_windows_amd64.zip`

Verify downloads against `checksums.txt` on the same release.

### Version metadata in binaries

`orlojd`, `orlojworker`, and `orlojctl` support `--version` / `orlojctl version` and embed **version**, **commit**, and **build date** via `-ldflags` at link time (release builds and Docker images).

## Release Inputs

- passing core test suite
- passing contract and runtime conformance suites
- passing documentation build and link checks
- passing compatibility smoke checks against pinned consumer references

## Packaging Requirements

- reproducible build inputs
- generated SBOM per release (repository SPDX on the GitHub Release; image SBOM/provenance from BuildKit)
- signed container images (Cosign keyless)
- provenance/attestation metadata (BuildKit `provenance: mode=max`)

## Release Checklist

1. Freeze release scope and changelog entries.
2. Run reliability validation (`orloj-loadtest`, `orloj-alertcheck`).
3. Run upgrade/canary verification in staging.
4. Complete security review and disclosure readiness checks.
5. Publish release notes and migration notes.
6. Tag `vX.Y.Z` and push; confirm the **release** workflow completes and artifacts appear on the GitHub Release and GHCR.

## Post-Release

- monitor early adoption signals and critical error reports
- publish hotfix plan for regressions when required
- track deferred work in [Roadmap](../phases/roadmap.md)
