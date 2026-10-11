#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

go run entgo.io/ent/cmd/ent generate --feature sql/lock,sql/upsert ./internal/database/ent/schema
