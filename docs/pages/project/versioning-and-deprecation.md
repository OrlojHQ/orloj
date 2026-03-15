# Versioning and Deprecation

This policy defines stability levels and compatibility expectations for API, CRD, and extension surfaces.

## Stability Levels

- `experimental`: may change without long-term guarantees.
- `beta`: intended for broader use, but may still change with notice.
- `stable`: backward compatibility is expected within the same major version.

## Compatibility Rules

- No unversioned breaking changes on stable public surfaces.
- Additive changes are preferred over replacements.
- Breaking changes require explicit versioning and migration notes.

## Deprecation Windows

- Experimental: no guaranteed window.
- Beta: deprecation notice required before removal.
- Stable: deprecation notice and at least one release cycle of overlap before removal.

## Change Requirements

Any compatibility-impacting change must include:

- lifecycle label (`experimental|beta|stable`)
- migration instructions
- release note entry
- test updates for compatibility/conformance coverage

## CI Policy

Release pipelines must run API/schema compatibility checks and fail on unversioned breaking changes.
