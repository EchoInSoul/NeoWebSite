#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ -z "${DATABASE_URL_TEST:-}" ]; then
  if [ $# -eq 0 ]; then
    echo "DATABASE_URL_TEST is not set; integration tests would silently skip" >&2
    exit 1
  fi
  echo "warning: DATABASE_URL_TEST is not set; integration tests under $* will silently skip" >&2
fi

if [ $# -eq 0 ]; then
  PACKAGES=(./...)
else
  PACKAGES=("$@")
fi

go test "${PACKAGES[@]}" -count=1 -p 1
