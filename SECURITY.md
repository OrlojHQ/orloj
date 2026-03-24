# Security Policy

## Supported Versions

Security fixes are applied to the latest release. We do not backport fixes to older releases.

| Version  | Supported |
| -------- | --------- |
| Latest   | Yes       |
| < Latest | No        |

## Reporting a Vulnerability

If you discover a security vulnerability in Orloj, please report it responsibly. **Do not open a public GitHub issue.**

Email **info@orloj.dev** with:

- A description of the vulnerability
- Steps to reproduce
- Affected versions (if known)
- Any potential impact assessment

We will acknowledge receipt within 48 hours and aim to provide an initial assessment within 5 business days. Once a fix is available, we will coordinate disclosure with you before publishing.

## Scope

This policy covers the Orloj open-source repository (`github.com/OrlojHQ/orloj`), including:

- The server (`orlojd`)
- The worker (`orlojworker`)
- The CLI (`orlojctl`)
- The web console

## Security Best Practices

For operational security guidance (API tokens, secret management, tool isolation, network hardening), see the [Security and Isolation](docs/pages/operations/security.md) documentation.
