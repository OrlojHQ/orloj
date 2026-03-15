# Support

This page describes how to get help and how support expectations differ by channel.

## Channels

- GitHub Issues: bug reports, feature requests, and documentation defects.
- GitHub Discussions: usage questions, architecture guidance, and design feedback.
- Security Reports: use the private security reporting process in [Security Policy](./security-policy.md).

## What to Include in a Support Request

- Orloj version and deployment mode.
- Control-plane and worker startup flags.
- Relevant manifest snippets (`Agent`, `AgentSystem`, `Task`, `Tool`).
- Output of:
  - `go run ./cmd/orlojctl get workers`
  - `go run ./cmd/orlojctl get tasks`
  - `go run ./cmd/orlojctl trace task <task-name>`
- Logs from `orlojd` and `orlojworker` around the failure window.

## Response Expectations

- Best-effort community support for all OSS users.
- Issues with complete reproduction detail are prioritized.
- Security issues are handled through private disclosure, not public issues.

## Escalation

If an issue blocks production rollout, provide impact level, mitigation attempts, and rollback status in the issue body.
