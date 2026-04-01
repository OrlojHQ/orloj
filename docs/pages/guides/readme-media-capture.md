# Capture README Orloj in Action Media

This guide documents the reproducible workflow for generating the README media assets used in the **Orloj in Action** section.

The capture pipeline targets the **frontend web console** (not docs pages) and outputs assets to `docs/public/readme/`.

## Prerequisites

- Repository root as current working directory
- Built binaries (`orlojd`, `orlojctl`) or Go toolchain available for fallback build
- Python packages:
  - `playwright`
  - `Pillow`
- Playwright Chromium runtime:

```bash
python3 -m playwright install chromium
```

## One-Command Capture

From repo root:

```bash
scripts/capture_readme_media.sh
```

What the script does:

1. Starts `orlojd` in deterministic local mode (`memory`, `sequential`, embedded worker).
2. Applies `testing/scenarios-real/01-pipeline` by default.
3. Waits for:
   - `agent-systems/<system>` to reach `Ready`
   - `tasks/<reference-task>` to reach `Succeeded`
4. Captures fixed UI routes via Playwright and writes:
   - `docs/public/readme/dashboard-overview.png`
   - `docs/public/readme/system-topology.png`
   - `docs/public/readme/task-detail-graph.png`
   - `docs/public/readme/task-trace-logs.png`
   - `docs/public/readme/task-run-lifecycle.gif`

## Capture Targets and Sizing

- Screenshot viewport: `1728x1080` (desktop-oriented)
- GIF output size: `960x540`
- Theme: light mode
- Locale: `en-US`

The scripts intentionally keep names stable so README links do not need to change.

Default capture inputs are controlled by env vars in `scripts/capture_readme_media.sh`:

- `ORLOJ_CAPTURE_MANIFEST_DIR` (default `testing/scenarios-real/01-pipeline`)
- `ORLOJ_CAPTURE_NAMESPACE` (default `rr-real-pipeline`)
- `ORLOJ_CAPTURE_SYSTEM` (default `rr-real-pipeline-system`)
- `ORLOJ_CAPTURE_REFERENCE_TASK` (default `rr-real-pipeline-task`)
- `ORLOJ_CAPTURE_READY_TIMEOUT` (default `2m`)
- `ORLOJ_CAPTURE_TASK_TIMEOUT` (default `3m`)

## Naming Convention

- `dashboard-overview.png` : Control-plane overview
- `system-topology.png` : AgentSystem resource tree/topology view
- `task-detail-graph.png` : Task detail with graph tab
- `task-trace-logs.png` : Task detail with logs/trace context
- `task-run-lifecycle.gif` : Task lifecycle (created -> running -> succeeded)

## Redaction and Safety Rules

- Use safe demo task input only.
- Do not capture personal usernames, hostnames, or secrets.
- Keep auth in `off` mode for capture runs.
- If any sensitive text appears, regenerate after clearing local state.

## Quality Gate Checklist

Before committing refreshed media:

1. Confirm all five media files were regenerated in `docs/public/readme/`.
2. Confirm README images render in GitHub markdown preview.
3. Confirm status and UI text are legible on desktop and mobile widths.
4. Keep combined media payload reasonably small (target under ~8 MB total).
