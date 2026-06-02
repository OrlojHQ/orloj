# Define a CLI Tool

This guide shows how to define a CLI tool that invokes a local binary (e.g., `kubectl`, `gh`, `aws`) under Orloj's governance and isolation model.

## Quick start

```yaml
apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: kubectl-get-pods
spec:
  type: cli
  description: "List Kubernetes pods in a namespace"
  input_schema:
    type: object
    properties:
      namespace:
        type: string
      format:
        type: string
        enum: [json, yaml, wide]
  cli:
    command: kubectl
    args:
      - get
      - pods
      - -n
      - "{{ .namespace }}"
      - -o
      - "{{ .format }}"
    image: bitnami/kubectl:1.30
    env_from:
      - name: KUBECONFIG
        secretRef: k8s-kubeconfig
  risk_level: medium
  operation_classes: [read]
  runtime:
    timeout: 15s
```

Apply:

```bash
orlojctl apply -f kubectl-get-pods.yaml
```

## How it works

1. The agent selects the tool and provides JSON input matching `input_schema`.
2. Orloj evaluates `cli.args` templates against the parsed JSON input to build an argv array.
3. Secrets referenced in `cli.env_from` are resolved from the secret store.
4. The command runs inside a container (the default) or directly on the worker host (`isolation_mode: none`).
5. Stdout is returned to the agent as the tool result.

## Argument templates

Each entry in `cli.args` is evaluated as a Go `text/template` with the model's JSON input as the data context. Static entries (without `{{ }}`) pass through unchanged.

```yaml
args:
  - get
  - pods
  - -n
  - "{{ .namespace }}"
```

If the model sends `{"namespace": "production", "format": "json"}`, the resulting argv is `["get", "pods", "-n", "production"]`. Each template produces exactly one argv entry -- there is no shell splitting.

Templates that evaluate to an empty string are dropped from the final argv.

## Passing input via stdin

For tools that read structured input from stdin (e.g., `jq`, custom CLIs), set `stdin_from_input: true`:

```yaml
cli:
  command: jq
  args: [".items[].metadata.name"]
  image: ghcr.io/jqlang/jq:1.7
  network: none
  stdin_from_input: true
```

Both templated args and stdin can be combined.

## Credentials

CLI tools do not use `spec.auth` (it is rejected at validation time). Instead, map Orloj secrets to the environment variables your binary expects using `env_from`:

```yaml
cli:
  command: gh
  args: ["pr", "list", "--repo", "{{ .repo }}"]
  image: ghcr.io/cli/cli:2.50
  env_from:
    - name: GITHUB_TOKEN
      secretRef: gh-api-token
```

For multiple credentials (e.g., AWS):

```yaml
env_from:
  - name: AWS_ACCESS_KEY_ID
    secretRef: aws-creds
    key: access_key
  - name: AWS_SECRET_ACCESS_KEY
    secretRef: aws-creds
    key: secret_key
```

Use `env` for non-secret literals:

```yaml
env:
  AWS_DEFAULT_REGION: us-east-1
```

## Container isolation (default)

CLI tools default to `container` isolation. The operator provides a container image containing the binary via `cli.image`. The container runs with:

- `--read-only` filesystem
- `--cap-drop=ALL`
- `--security-opt no-new-privileges`
- `--network none` by default (inherits `--tool-container-network`; configurable per tool via `cli.network`)
- Resource limits from `cli.resources` (per-tool) or the global worker config (`--tool-container-memory`, `--tool-container-cpus`, `--tool-container-pids-limit`)

Set `cli.network: bridge` when the binary needs outbound network access (e.g., `kubectl`, `gh`, `curl`):

```yaml
cli:
  command: gh
  image: ghcr.io/cli/cli:2.50
  network: bridge
  env_from:
    - name: GITHUB_TOKEN
      secretRef: gh-api-token
```

Tools that do not need network access (e.g., `jq`, `yq`) can leave `cli.network` unset or set it explicitly to `none`:

```yaml
cli:
  command: jq
  image: ghcr.io/jqlang/jq:1.7
  network: none
```

### Per-tool container resources

Tools that need more resources than the global defaults (e.g. Chromium-based tools) can declare per-tool overrides via `cli.resources`. When set, these take precedence over the global `--tool-container-*` flags:

```yaml
cli:
  command: screenshot
  image: my-chromium:latest
  network: bridge
  resources:
    memory: 1g
    cpus: "1.0"
    pids_limit: 256
```

| Field | Format | Description |
|-------|--------|-------------|
| `memory` | Docker memory string (`128m`, `1g`) | Container memory limit. |
| `cpus` | Decimal string (`0.50`, `1.0`) | Container CPU limit. |
| `pids_limit` | Integer | Container PID limit. |

Operators can set a ceiling with `--tool-container-max-memory`, `--tool-container-max-cpus`, and `--tool-container-max-pids-limit` on `orlojd`. Manifests exceeding the ceiling are rejected at apply time.

## Kubernetes isolation

When `isolation_mode: kubernetes`, CLI tools run as ephemeral Kubernetes Jobs instead of local Docker containers. The same `cli.image`, `cli.command`, `cli.args`, and `cli.resources` fields are used, but execution happens in the cluster via the Kubernetes API rather than via `docker run`.

```yaml
spec:
  type: cli
  cli:
    command: kubectl
    args: ["get", "pods", "-n", "{{ .namespace }}"]
    image: bitnami/kubectl:1.30
    resources:
      memory: 256m
      cpus: "0.50"
  runtime:
    isolation_mode: kubernetes
    timeout: 30s
```

This mode requires `--tool-k8s-enabled=true` on the server and worker. The runtime creates a Job in the configured namespace, waits for completion (bounded by `runtime.timeout`), captures stdout/stderr from the Pod logs, and cleans up via `ttlSecondsAfterFinished`.

Key differences from `container` isolation:

- Execution happens in-cluster; no Docker socket is required on the worker host.
- Resource limits from `cli.resources` map to Kubernetes resource requests/limits on the Pod spec.
- `runtime.timeout` sets `activeDeadlineSeconds` on the Job.
- Network isolation is managed via Kubernetes NetworkPolicies rather than Docker network modes (`cli.network` is ignored).
- Credentials from `cli.env_from` are injected as environment variables on the Job's container spec.

See [Kubernetes Deployment](../../deploy/kubernetes.md) for RBAC and Helm configuration.

## Direct execution (no container)

For trusted tools on the worker host, set `isolation_mode: none`. The binary must exist on the worker's filesystem. `cli.image` is not required in this mode.

```yaml
spec:
  type: cli
  cli:
    command: /usr/local/bin/my-tool
    args: ["--flag", "{{ .value }}"]
  runtime:
    isolation_mode: none
```

## Output capture

`cli.output` controls what is returned to the agent:

- `stdout` (default) -- return stdout only
- `stderr` -- return stderr only
- `both` -- return `{"stdout": "...", "stderr": "..."}` as JSON

Non-zero exit codes produce a tool error with the exit code and stderr tail in the error details.

## Worker flags

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--cli-tool-allowed-commands` | `ORLOJ_CLI_TOOL_ALLOWED_COMMANDS` | (empty) | Comma-separated command allowlist. Empty allows all. |
| `--cli-tool-max-argv-length` | `ORLOJ_CLI_TOOL_MAX_ARGV_LENGTH` | `4096` | Max total argv byte length. |

## See also

- [Tool reference](../../reference/resources/tool.md)
- [Security and Isolation](../../operations/security.md)
