#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

export APP_ENV="${APP_ENV:-development}"

exec go run .
