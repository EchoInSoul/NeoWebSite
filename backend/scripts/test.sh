#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ $# -eq 0 ]; then
  [ -n "${DATABASE_URL_TEST:-}" ] || { echo "DATABASE_URL_TEST is not set; integration tests would silently skip" >&2; exit 1; }
  PACKAGES=(./...)
else
  PACKAGES=("$@")
fi

go test "${PACKAGES[@]}" -count=1 -p 1
