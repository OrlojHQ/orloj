#!/bin/zsh

set -eu

export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"

CURL_BIN="${CURL_BIN:-/usr/bin/curl}"
JQ_BIN="${JQ_BIN:-/opt/homebrew/bin/jq}"

if [ "$#" -lt 3 ]; then
  echo "usage: $0 <namespace> <task> <verdict> [memory ...]" >&2
  exit 1
fi

NS="$1"
TASK="$2"
VERDICT="$3"
shift 3

API_BASE="${API_BASE:-http://localhost:8080}"
ARTIFACT_ROOT="${ARTIFACT_ROOT:-testing/artifacts/real}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${ARTIFACT_ROOT}/${NS}/${TASK}/${STAMP}"

mkdir -p "${OUT_DIR}"

capture_json() {
  local path="$1"
  local outfile="$2"
  if "${CURL_BIN}" -sSf "${API_BASE}${path}" | "${JQ_BIN}" . > "${OUT_DIR}/${outfile}" 2>/dev/null; then
    return 0
  fi
  printf '{}\n' > "${OUT_DIR}/${outfile}"
  return 0
}

capture_json "/v1/tasks/${TASK}?namespace=${NS}" "task.json"
capture_json "/v1/tasks/${TASK}/messages?namespace=${NS}" "messages.json"
capture_json "/v1/tasks/${TASK}/metrics?namespace=${NS}" "metrics.json"

for memory in "$@"; do
  if [ -n "${memory}" ]; then
    capture_json "/v1/memories/${memory}/entries?namespace=${NS}&limit=100" "memory-${memory}.json"
  fi
done

printf '%s\n' "${VERDICT}" > "${OUT_DIR}/verdict.txt"
printf '%s\n' "${OUT_DIR}"
