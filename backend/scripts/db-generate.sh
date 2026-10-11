#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

go run entgo.io/ent/cmd/ent generate --feature sql/lock ./internal/database/ent/schema
