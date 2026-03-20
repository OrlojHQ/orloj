# GitHub Pages (project docs site)

This runbook is for **maintainers** who publish the Vocs documentation to **GitHub Pages**, including a **custom domain**.

## One-time repository setup

1. **Pages source:** Repository **Settings → Pages → Build and deployment**  
   Set **Source** to **GitHub Actions** (not “Deploy from a branch”).

2. **Custom domain (optional):** **Settings → Pages → Custom domain**  
   Enter your hostname (e.g. `docs.example.com`). Add the **DNS records** GitHub shows (usually `A` / `AAAA` for apex, or `CNAME` for a subdomain). Wait for the **DNS check** to pass and **HTTPS** to provision.

3. **First deploy:** The workflow uses the **`github-pages` environment**. If GitHub prompts for approval, allow it under **Settings → Environments**.

## Publish on demand

Use **[`.github/workflows/docs-pages-deploy.yml`](../../../.github/workflows/docs-pages-deploy.yml)** — **Actions → “docs-pages-deploy” → Run workflow**.

| Input        | When to use |
| ------------ | ----------- |
| **base_url** | Required. The public site URL with **no trailing slash**, e.g. `https://docs.example.com`. This sets Vocs **`baseUrl`** (canonical and Open Graph links). Use the same URL visitors will use (custom domain or default `https://<org>.github.io/<repo>`). |
| **base_path** | Usually **empty** for a **custom domain** at the site root. Set only if the site lives under a path prefix (e.g. `/orloj` for `https://<org>.github.io/orloj/`). |

### Custom domain at root (typical)

- **base_url:** `https://docs.yourdomain.com` (or whatever you configured in Pages)  
- **base_path:** leave **empty**

### Default `github.io` project URL (no custom domain)

- **base_url:** `https://<org>.github.io/<repo>` (no trailing slash)  
- **base_path:** `/<repo>` (leading slash, e.g. `/orloj`)

Local preview is unchanged: run `bun run docs:dev` without `VOCS_BASE_*` env vars.

## Related

- [Deployment overview](./index.md)
- [Install](../getting-started/install.md) — reading docs locally
