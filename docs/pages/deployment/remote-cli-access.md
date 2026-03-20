# Remote CLI and API access

This guide is for **operators and users** who already have `orlojd` reachable on a network (self-hosted, VPS, Kubernetes, or internal URL) and need to call the API from **`orlojctl`**, scripts, or CI. It complements the [quickstart](../getting-started/quickstart.md), which focuses on a single-machine dev loop.

For deeper security context (generation, rotation, threat model), see [Control plane API tokens](../operations/security.md#control-plane-api-tokens).

## API tokens (shared secret)

Orloj **does not** issue API tokens from the web console. The **operator** generates a random string, configures **the same value** on the server and on every client that uses `Authorization: Bearer <token>`.

```bash
openssl rand -hex 32
```

Store the value in your secrets manager or deployment environment—**not** in git.

On the server, set **`orlojd --api-key=...`** or **`ORLOJ_API_TOKEN=...`** (or **`ORLOJ_API_TOKENS`** for multiple `token:role` pairs). See [Control plane API tokens](../operations/security.md#control-plane-api-tokens) for details.

## Server-side wiring

Where you set `ORLOJ_API_TOKEN` depends on how you run `orlojd`:

- **Docker Compose / systemd** — env var or secret in the service definition (e.g. [VPS deployment](./vps.md)).
- **Kubernetes / Helm** — `runtimeSecret` or equivalent env injection (see [Kubernetes deployment](./kubernetes.md)).

## Client-side: environment and flags

From any machine that should talk to the API:

| Mechanism | Purpose |
|-----------|---------|
| `ORLOJ_SERVER` | Default API base URL when `--server` is omitted |
| `ORLOJCTL_SERVER` | Same default; **takes precedence** over `ORLOJ_SERVER` |
| `ORLOJ_API_TOKEN` | Bearer token |
| `ORLOJCTL_API_TOKEN` | Same token; checked before `ORLOJ_API_TOKEN` by the CLI |
| `orlojctl --api-token <token>` | Overrides env for that process |
| `orlojctl --server <url>` | Overrides per-command default server |

## Precedence

**Token** (first match wins):

1. `orlojctl --api-token ...`
2. `ORLOJCTL_API_TOKEN`
3. `ORLOJ_API_TOKEN`
4. Active profile: `token` field, else value of the env var named by `token_env`

**Default `--server`** when the flag is omitted (first match wins):

1. `ORLOJCTL_SERVER`
2. `ORLOJ_SERVER`
3. Active profile `server`
4. `http://127.0.0.1:8080`

Explicit `--server` on a subcommand always overrides the default above.

## `orlojctl config` and `config.json`

Named **profiles** are stored as JSON:

- **Path:** `orlojctl config path` (typically `~/.config/orlojctl/config.json` on Unix).
- **Permissions:** file is written with mode `0600` when created or updated.

**The file does not exist until the first successful save** (for example `orlojctl config set-profile <name> ...`). Until then, only environment variables and flags apply—if you open the path early, an empty or missing file is normal.

Commands:

```bash
orlojctl config path
orlojctl config set-profile production --server https://orloj.example.com --token-env ORLOJ_PROD_TOKEN
orlojctl config use production
orlojctl config get
```

`set-profile` creates or updates a profile. The first profile you create also becomes **`current_profile`** if none was set. Prefer **`--token-env`** so the token is not stored in the JSON file.

### Example `config.json`

Shape matches the CLI (field names are JSON):

```json
{
  "current_profile": "production",
  "profiles": {
    "local": {
      "server": "http://127.0.0.1:8080"
    },
    "production": {
      "server": "https://orloj.example.com",
      "token_env": "ORLOJ_PROD_TOKEN"
    }
  }
}
```

You can hand-edit this file if you prefer; invalid JSON will cause `orlojctl` to error on load.

## Local UI auth vs API tokens

If you use **`--auth-mode=local`**, the web UI uses an **admin username/password** and **session cookies**. That is separate from API access: **`orlojctl` and automation should use the bearer token** configured with `ORLOJ_API_TOKEN` / `--api-key` on the server, not the UI password. See [Control plane API tokens](../operations/security.md#control-plane-api-tokens) and [CLI reference: orlojctl](../reference/cli.md#orlojctl).

## Related docs

- [CLI reference](../reference/cli.md) — full command list and flags
- [Configuration](../operations/configuration.md) — `orlojd` / `orlojworker` environment variables
- [VPS deployment](./vps.md) — single-node Compose + systemd
- [Kubernetes deployment](./kubernetes.md) — Helm and manifests
