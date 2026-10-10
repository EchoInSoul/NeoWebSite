#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

go run entgo.io/ent/cmd/ent generate ./internal/database/ent/schema
