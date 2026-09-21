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

if [ -z "${TIMEOUTX_FD:-}" ]; then
  exec "$TIMEOUTX" run --idle 30s --stall 30s -- bash "$DIR/run.sh"
fi

eval "$("$TIMEOUTX" shell-init)"
for i in 1 2 3; do
  timeout_unit "import:$i" :
  timeout_progress \
    --current "$i" \
    --total 3 \
    --stage import-users \
    "imported $i"
done
