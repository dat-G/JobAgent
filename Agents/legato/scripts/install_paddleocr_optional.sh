#!/usr/bin/env sh
set -eu

PYTHON_BIN="${PYTHON_BIN:-}"
if [ -z "$PYTHON_BIN" ]; then
  if command -v python3 >/dev/null 2>&1; then
    PYTHON_BIN=python3
  else
    PYTHON_BIN=python
  fi
fi

"$PYTHON_BIN" -m pip install -r "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)/requirements.paddleocr.optional.txt"
