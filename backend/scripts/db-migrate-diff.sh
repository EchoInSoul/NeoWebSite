#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

ATLAS_DEV_URL="${ATLAS_DEV_URL:-postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_atlas_dev?sslmode=disable}"

atlas migrate diff totp_and_recovery_codes --dir "file://migrations" --to "ent://internal/database/ent/schema" --dev-url "$ATLAS_DEV_URL"
