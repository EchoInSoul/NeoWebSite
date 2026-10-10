#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

atlas migrate apply --dir "file://migrations" --url "$DATABASE_URL"
atlas migrate apply --dir "file://migrations" --url "$DATABASE_URL_TEST"
