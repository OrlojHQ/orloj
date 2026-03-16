# Security Policy

This policy defines how vulnerabilities are reported, triaged, fixed, and disclosed for Orloj.

## Reporting a Vulnerability

Do not open public issues for suspected vulnerabilities.

Use private reporting through repository security advisories or maintainer-designated security contact channels.

## Disclosure Process

1. Receive and acknowledge report.
2. Validate severity and impact.
3. Prepare a fix and regression tests.
4. Publish patched release and advisory.

## Response Targets

- Initial acknowledgement: within 3 business days.
- Severity triage: within 5 business days.
- Critical fix target: as soon as safely possible, with expedited patch release.

## Supported Fix Policy

- Security fixes are released as patch versions when possible.
- Release notes must include mitigation and upgrade instructions.
- CVE assignment is used when appropriate.

## Security Scope

This policy covers:

- server and worker binaries
- API and resource surfaces
- runtime isolation boundaries
- release artifacts and provenance

## Exclusions

General usage questions and non-security bugs should use standard support channels from [Support](./support.md).
