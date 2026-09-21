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

exec "$TIMEOUTX" run --idle 2s --heartbeat-on-output -- bash -c 'i=0; while [ "$i" -lt 8 ]; do echo tick; i=$((i+1)); sleep 0.4; done'
