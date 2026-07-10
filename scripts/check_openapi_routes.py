#!/usr/bin/env python3
"""Fail if Go HTTP /v1 routes are missing from openapi/openapi.yaml paths.

Run from the orloj repo root:
  python3 scripts/check_openapi_routes.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path


def mux_paths(server_go: Path) -> set[str]:
    text = server_go.read_text(encoding="utf-8")
    found: set[str] = set()
    for m in re.finditer(r'\.Handle(?:Func)?\(\s*"([^"]+)"', text):
        path = m.group(1)
        if not (
            path.startswith("/v1/")
            or path in {"/healthz", "/metrics", "/a2a"}
            or path.startswith("/.well-known/")
        ):
            continue
        found.add(path.rstrip("/") if path != "/" else path)
    return found


def openapi_paths(openapi_yaml: Path) -> set[str]:
    text = openapi_yaml.read_text(encoding="utf-8")
    paths: set[str] = set()
    # Keys may be quoted: `  "/v1/agents":` or unquoted.
    for m in re.finditer(r'^  "?(/[^"\s:]+)"?:\s*$', text, flags=re.M):
        paths.add(m.group(1))
    return paths


def covered_by_openapi(mux_path: str, spec: set[str]) -> bool:
    """True if an exact OpenAPI path exists, or a templated child path covers a Go tree root."""
    if mux_path in spec:
        return True
    # e.g. mux `/v1/webhook-deliveries` + OpenAPI `/v1/webhook-deliveries/{endpoint_id}`
    prefix = mux_path + "/{"
    return any(p.startswith(prefix) for p in spec)


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    server = root / "api" / "server.go"
    openapi = root / "openapi" / "openapi.yaml"
    if not server.is_file() or not openapi.is_file():
        print(
            "error: run from orloj repo (api/server.go + openapi/openapi.yaml required)",
            file=sys.stderr,
        )
        return 1

    mux = mux_paths(server)
    spec = openapi_paths(openapi)
    missing = sorted(p for p in mux if not covered_by_openapi(p, spec))

    print(f"mux API paths: {len(mux)}")
    print(f"openapi paths: {len(spec)}")
    if missing:
        print("paths registered in server.go but not in openapi.yaml:")
        for p in missing:
            print(f"  - {p}")
        print(
            "\nFAIL: update openapi/build_openapi.py + schemas, then run "
            "`python3 openapi/build_openapi.py`.\n",
            file=sys.stderr,
        )
        return 1
    print("OK: all mux API paths appear in OpenAPI.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
