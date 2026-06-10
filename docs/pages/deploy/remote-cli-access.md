# Remote CLI and API access

This guide is for **operators and users** who already have `orlojd` reachable on a network (self-hosted, VPS, Kubernetes, or internal URL) and need to call the API from **`orlojctl`**, scripts, or CI. It complements the [quickstart](../getting-started/quickstart.md), which focuses on a single-machine dev loop.

For deeper security context (generation, rotation, threat model), see [Control plane API tokens](../operations/security.md#control-plane-api-tokens).

## Install `orlojctl` locally

You need the CLI on **your** machine (or in CI), not inside the server container. The easiest path is Homebrew:

```bash
brew tap OrlojHQ/orloj
brew install orlojctl
```

Alternatively, download the standalone binary from [GitHub Releases](https://github.com/OrlojHQ/orloj/releases) (`orlojctl_<tag>_<os>_<arch>`), verify with `checksums.txt`, extract, and add it to your `PATH`. Details and naming conventions are in [Install: CLI only for hosted deployments](../getting-started/install.md#cli-only-for-hosted-deployments). If you already cloned the repo with Go installed, `go run ./cmd/orlojctl` works the same way against a remote `--server`.

## API tokens (shared secret)

Orloj **does not** issue API tokens from the web console. The **operator** generates a random string, configures **the same value** on the server and on every client that uses `Authorization: Bearer <token>`.

```bash
openssl rand -hex 32
```

Store the value in your secrets manager or deployment environment—**not** in git.

On the server, set **`orlojd --api-key=...`** or **`ORLOJ_API_TOKEN=...`** (or **`ORLOJ_API_TOKENS`** for multiple `name:token:role` entries; legacy `token:role` remains supported). See [Control plane API tokens](../operations/security.md#control-plane-api-tokens) for details.

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

## `orlojctl auth login` (native mode)

When the server runs **`--auth-mode=native`**, you can authenticate the CLI directly with your username and password instead of manually configuring tokens:

```bash
orlojctl config set-profile production --profile-server https://orloj.example.com
orlojctl config use production
orlojctl auth login
# Username: admin
# Password: ********
# logged in as admin (role=admin) on https://orloj.example.com
# token saved to profile "production"
```

This calls `POST /v1/auth/cli-token` on the server, which validates your credentials and mints a new bearer token. The token is automatically saved to the active profile in `config.json`.

You can also pass credentials non-interactively (useful in CI):

```bash
orlojctl auth login -u admin -p "$MY_PASSWORD"
```

After login, all subsequent commands use the saved bearer token. Run `orlojctl auth whoami` to verify.

## Local UI auth vs API tokens

If you use **`--auth-mode=native`**, the web UI uses an **admin username/password** and **session cookies**. The CLI uses **bearer tokens**. You can obtain a CLI token in two ways:

1. **`orlojctl auth login`** — authenticates with your username/password and saves a token to the active profile (recommended).
2. **Manual token configuration** — an operator generates a token with `ORLOJ_API_TOKEN` / `--api-key` on the server, and you configure it with `--token-env` or `--token` in a profile.

See [Control plane API tokens](../operations/security.md#control-plane-api-tokens) and [CLI reference: orlojctl](../reference/cli.md#orlojctl).

## Related docs

- [CLI reference](../reference/cli.md) — full command list and flags
- [Configuration](../operations/configuration.md) — `orlojd` / `orlojworker` environment variables
- [VPS deployment](./vps.md) — single-node Compose + systemd
- [Kubernetes deployment](./kubernetes.md) — Helm and manifests
