#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

BASE_URL="${ORLOJ_CAPTURE_BASE_URL:-http://127.0.0.1:8080}"
SERVER_ADDR="${ORLOJ_CAPTURE_ADDR:-127.0.0.1:8080}"
OUTPUT_DIR="${ORLOJ_CAPTURE_OUT_DIR:-docs/public/readme}"
MANIFEST_DIR="${ORLOJ_CAPTURE_MANIFEST_DIR:-testing/scenarios-real/01-pipeline}"
CAPTURE_NAMESPACE="${ORLOJ_CAPTURE_NAMESPACE:-rr-real-pipeline}"
CAPTURE_SYSTEM="${ORLOJ_CAPTURE_SYSTEM:-rr-real-pipeline-system}"
REFERENCE_TASK="${ORLOJ_CAPTURE_REFERENCE_TASK:-rr-real-pipeline-task}"
READY_TIMEOUT="${ORLOJ_CAPTURE_READY_TIMEOUT:-2m}"
TASK_TIMEOUT="${ORLOJ_CAPTURE_TASK_TIMEOUT:-3m}"

ORLOJD_BIN="${ORLOJD_BIN:-${ROOT_DIR}/orlojd}"
ORLOJCTL_BIN="${ORLOJCTL_BIN:-${ROOT_DIR}/orlojctl}"
SKIP_BUILD="${ORLOJ_CAPTURE_SKIP_BUILD:-0}"

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required"
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "error: python3 is required"
  exit 1
fi

python3 - <<'PY'
import importlib.util
missing = [pkg for pkg in ("PIL", "playwright") if importlib.util.find_spec(pkg) is None]
if missing:
    raise SystemExit(
        "error: missing python dependencies: "
        + ", ".join(missing)
        + ". Install them and run `python3 -m playwright install chromium`."
    )
PY

if [[ "${SKIP_BUILD}" != "1" ]]; then
  echo "Building capture binaries..."
  go build -o "${ORLOJD_BIN}" ./cmd/orlojd
  go build -o "${ORLOJCTL_BIN}" ./cmd/orlojctl
else
  if [[ ! -x "${ORLOJD_BIN}" ]]; then
    echo "error: ORLOJ_CAPTURE_SKIP_BUILD=1 but ${ORLOJD_BIN} is missing"
    exit 1
  fi
  if [[ ! -x "${ORLOJCTL_BIN}" ]]; then
    echo "error: ORLOJ_CAPTURE_SKIP_BUILD=1 but ${ORLOJCTL_BIN} is missing"
    exit 1
  fi
fi

mkdir -p "${OUTPUT_DIR}"
rm -f "${OUTPUT_DIR}/_debug-task-page.html" "${OUTPUT_DIR}/_debug-task-page.png"

SERVER_LOG="$(mktemp -t orloj-readme-server.XXXXXX.log)"
ORLOJD_PID=""

cleanup() {
  if [[ -n "${ORLOJD_PID}" ]] && kill -0 "${ORLOJD_PID}" >/dev/null 2>&1; then
    kill "${ORLOJD_PID}" >/dev/null 2>&1 || true
    wait "${ORLOJD_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "Starting orlojd at ${SERVER_ADDR}..."
"${ORLOJD_BIN}" \
  --storage-backend=memory \
  --task-execution-mode=sequential \
  --embedded-worker \
  --embedded-worker-max-concurrent-tasks=1 \
  --addr="${SERVER_ADDR}" \
  >"${SERVER_LOG}" 2>&1 &
ORLOJD_PID=$!

echo "Waiting for API health..."
for _ in $(seq 1 60); do
  if curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

if ! curl -sf "${BASE_URL}/healthz" >/dev/null 2>&1; then
  echo "error: API did not become healthy."
  echo "--- orlojd log ---"
  cat "${SERVER_LOG}"
  exit 1
fi

echo "Applying capture manifests from ${MANIFEST_DIR}..."
"${ORLOJCTL_BIN}" apply --server "${BASE_URL}" --namespace "${CAPTURE_NAMESPACE}" -f "${MANIFEST_DIR}" --run >/dev/null

task_visible() {
  "${ORLOJCTL_BIN}" get tasks --server "${BASE_URL}" --namespace "${CAPTURE_NAMESPACE}" 2>/dev/null \
    | awk -v t="${REFERENCE_TASK}" 'NR > 1 && $1 == t { found = 1 } END { exit found ? 0 : 1 }'
}

echo "Waiting for task resources to be visible..."
for _ in $(seq 1 30); do
  if task_visible; then
    break
  fi
  sleep 0.5
done

if ! task_visible; then
  echo "error: reference task ${REFERENCE_TASK} was not found in namespace ${CAPTURE_NAMESPACE}"
  exit 1
fi

echo "Waiting for system readiness (${CAPTURE_SYSTEM})..."
"${ORLOJCTL_BIN}" wait \
  --server "${BASE_URL}" \
  --namespace "${CAPTURE_NAMESPACE}" \
  --for condition=Ready \
  --timeout "${READY_TIMEOUT}" \
  "agent-systems/${CAPTURE_SYSTEM}" >/dev/null

echo "Waiting for task success (${REFERENCE_TASK})..."
"${ORLOJCTL_BIN}" wait \
  --server "${BASE_URL}" \
  --namespace "${CAPTURE_NAMESPACE}" \
  --for condition=Succeeded \
  --timeout "${TASK_TIMEOUT}" \
  "tasks/${REFERENCE_TASK}" >/dev/null

echo "Capturing frontend screenshots and GIF..."
python3 "${ROOT_DIR}/scripts/capture_readme_media.py" \
  --base-url "${BASE_URL}" \
  --out-dir "${OUTPUT_DIR}" \
  --reference-task "${REFERENCE_TASK}" \
  --namespace "${CAPTURE_NAMESPACE}" \
  --system-name "${CAPTURE_SYSTEM}"

echo "Captured assets in ${OUTPUT_DIR}:"
ls -1 "${OUTPUT_DIR}" | sed 's/^/  - /'
