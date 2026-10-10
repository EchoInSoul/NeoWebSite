#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

win_path="$(pwd -W)"
wsl_path="/mnt/$(printf '%s' "$win_path" | sed -E 's#^([A-Za-z]):#\L\1#')"

wsl -d Ubuntu -- bash -lc "cd '$wsl_path' && docker compose up -d"

for _ in $(seq 1 30); do
  status="$(wsl -d Ubuntu -- docker inspect -f '{{.State.Health.Status}}' neoblog-postgres 2>/dev/null || true)"
  [ "$status" = "healthy" ] && { echo "neoblog-postgres is healthy"; exit 0; }
  sleep 2
done

echo "neoblog-postgres did not become healthy in time" >&2
exit 1
