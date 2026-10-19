#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

unformatted="$(
  { git ls-files -z '*.go'; git ls-files --others --exclude-standard -z '*.go'; } |
    xargs -0 -r gofmt -l
)"
if [ -n "$unformatted" ]; then
  echo "gofmt needed:" >&2
  printf '%s\n' "$unformatted" >&2
  exit 1
fi

import_violations="$(grep -R -l 'website/backend/internal/common/apierror' --include='repository.go' internal/modules || true)"
if [ -n "$import_violations" ]; then
  echo "repository must not import apierror:" >&2
  printf '%s\n' "$import_violations" >&2
  exit 1
fi

go vet ./...
go build ./...
git diff --check
git diff --cached --check

echo "check ok"
