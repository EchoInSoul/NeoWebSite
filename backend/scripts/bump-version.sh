#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $(basename "$0") <major|minor|patch>" >&2
  echo "       $(basename "$0") set <x.y.z>" >&2
  exit 2
}

[ $# -ge 1 ] && [ $# -le 2 ] || usage

KIND="$1"
case "$KIND" in
  major|minor|patch) [ $# -eq 1 ] || usage ;;
  set) [ $# -eq 2 ] || usage ;;
  *) usage ;;
esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FILE="$ROOT/internal/version/version.go"
README="$ROOT/README.md"

[ -f "$FILE" ] || { echo "not found: $FILE" >&2; exit 1; }

NONBLANK="$(awk 'NF {n++} END {print n+0}' "$FILE")"
FIRST="$(awk 'NF {print; exit}' "$FILE")"
if [ "$NONBLANK" != "2" ] || [ "$FIRST" != "package version" ]; then
  echo "refusing to rewrite $FILE: unexpected content" >&2
  exit 1
fi

CURRENT="$(awk -F'"' '/^var Version = "/{print $2; exit}' "$FILE")"

if [ "$KIND" = "set" ]; then
  RAW="$2"
else
  RAW="${CURRENT:-}"
fi

if ! [[ "$RAW" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "invalid version: '$RAW' ($KIND)" >&2
  exit 1
fi

X=$((10#${BASH_REMATCH[1]}))
Y=$((10#${BASH_REMATCH[2]}))
Z=$((10#${BASH_REMATCH[3]}))

case "$KIND" in
  major) X=$((X + 1)); Y=0; Z=0 ;;
  minor) Y=$((Y + 1)); Z=0 ;;
  patch) Z=$((Z + 1)) ;;
esac

NEW="$X.$Y.$Z"
printf 'package version\n\nvar Version = "%s"\n' "$NEW" > "$FILE"

if [ -f "$README" ]; then
  sed -i "s/^当前版本：[^\r]*/当前版本：$NEW/" "$README"
  grep -q "当前版本：$NEW" "$README" || echo "warning: README.md 当前版本 line not updated" >&2
else
  echo "warning: $README not found, only version.go was updated" >&2
fi

echo "version: ${CURRENT:-?} -> $NEW"
echo "remember: commit internal/version/version.go (and README.md) together with your change"
