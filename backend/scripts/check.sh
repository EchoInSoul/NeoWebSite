#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

existing_go_files=()
while IFS= read -r -d '' file; do
  if [ -f "$file" ]; then
    existing_go_files+=("$file")
  fi
done < <({ git ls-files -z '*.go'; git ls-files --others --exclude-standard -z '*.go'; })

if [ "${#existing_go_files[@]}" -gt 0 ]; then
  unformatted="$(gofmt -l "${existing_go_files[@]}")"
  if [ -n "$unformatted" ]; then
    echo "gofmt needed:" >&2
    printf '%s\n' "$unformatted" >&2
    exit 1
  fi
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
