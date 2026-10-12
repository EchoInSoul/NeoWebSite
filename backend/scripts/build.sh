#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(awk -F'"' '/^var Version = "/{print $2; exit}' internal/version/version.go)"
[ -n "$VERSION" ] || { echo "cannot read version from internal/version/version.go" >&2; exit 1; }

TARGET_OS="${GOOS:-$(go env GOOS)}"
TARGET_ARCH="${GOARCH:-$(go env GOARCH)}"
EXT=""
if [ "$TARGET_OS" = "windows" ]; then EXT=".exe"; fi

OUT="${1:-bin/neoblog-${VERSION}-${TARGET_OS}-${TARGET_ARCH}${EXT}}"

mkdir -p "$(dirname "$OUT")"

CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" go build \
  -trimpath \
  -ldflags "-s -w -X website/backend/internal/config.DefaultEnvironment=production" \
  -o "$OUT" .

BYTES="$(wc -c < "$OUT" | tr -d ' ')"
echo "built: $OUT (${BYTES} B)"
echo "APP_ENV default: production"
echo "version: $VERSION"
