#!/usr/bin/env bash
set -euo pipefail

DIR=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$DIR/../../.." && pwd)

if [ -n "${TIMEOUTX:-}" ] && [ -x "${TIMEOUTX}" ]; then
  :
elif [ -x "$ROOT/bin/timeoutx.exe" ]; then
  TIMEOUTX="$ROOT/bin/timeoutx.exe"
elif [ -x "$ROOT/bin/timeoutx" ]; then
  TIMEOUTX="$ROOT/bin/timeoutx"
else
  echo "timeoutx not found; build bin/timeoutx first" >&2
  exit 1
fi

work=$(mktemp -d)
file="$work/output.bin"
trap 'rm -rf "$work"' EXIT

exec "$TIMEOUTX" run --stall 5s --progress-file "$file" -- bash -c 'sleep 0.3; echo x > "$1"; sleep 0.5; echo yy >> "$1"; sleep 0.2' _ "$file"
