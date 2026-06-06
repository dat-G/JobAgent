#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

PYTHON_BIN="${PYTHON_BIN:-}"
if [ -z "$PYTHON_BIN" ]; then
  if command -v python3 >/dev/null 2>&1; then
    PYTHON_BIN=python3
  else
    PYTHON_BIN=python
  fi
fi

"$PYTHON_BIN" -m pip install \
  --no-index \
  --find-links "$ROOT_DIR/vendor/wheelhouse" \
  -r "$ROOT_DIR/requirements.markitdown.lock"

"$PYTHON_BIN" -m pip install \
  --no-index \
  --find-links "$ROOT_DIR/vendor/wheelhouse" \
  -r "$ROOT_DIR/requirements.build.lock"

"$PYTHON_BIN" -m pip install \
  --no-index \
  --find-links "$ROOT_DIR/vendor/wheelhouse" \
  --no-build-isolation \
  -e "$ROOT_DIR"
