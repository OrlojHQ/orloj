# Governance

This page defines decision-making and maintainer responsibilities for Orloj.

## Roles

- Maintainers: final merge and release authority.
- Reviewers: technical review ownership for code and docs.
- Contributors: proposed changes through pull requests and issues.

## Decision Model

- Additive changes: approved through standard review.
- Breaking changes: require explicit maintainer approval and a documented migration plan.
- Security-sensitive changes: require security review before merge.

## Release Cadence Policy

- Minor releases deliver backward-compatible features.
- Patch releases deliver bug and security fixes.
- Breaking changes are versioned and announced with migration guidance.

## Breaking-Change Escalation

A breaking change proposal must include:

- compatibility impact summary
- migration path and timeline
- rollback strategy
- conformance/test updates

## Contribution Standards

- New behavior requires tests and documentation updates.
- Contract/API changes require compatibility notes.
- Operator-facing changes require runbook and troubleshooting coverage.
