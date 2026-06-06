#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

python3 -m pip install \
  --no-index \
  --find-links "$ROOT_DIR/vendor/wheelhouse" \
  -r "$ROOT_DIR/requirements.markitdown.lock"
