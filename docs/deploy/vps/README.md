# VPS Deployment Assets

This directory contains Docker Compose and systemd assets used by the Deployment docs.

## Files

- `docker-compose.vps.yml`: single-node production-style stack.
- `.env.vps.example`: required environment variables.
- `orloj-compose.service`: systemd unit for stack lifecycle management.

Use the operator guide at `docs/pages/deployment/vps.md` for the full runbook.
