#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-"$ROOT_DIR/.env"}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
else
  echo "missing .env: $ENV_FILE" >&2
  exit 1
fi

absolute_path() {
  local value="$1"
  if [[ "$value" == /* ]]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "$ROOT_DIR/${value#./}"
  fi
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

require_command() {
  if ! command_exists "$1"; then
    echo "missing command: $1" >&2
    exit 1
  fi
}

usage() {
  cat >&2 <<EOF
usage: $0 [up|down|restart|status|logs] [options]

Options:
  --cache, --presto-cache               enable Presto persistent response cache
  --no-cache, --no-presto-cache         disable Presto persistent response cache (default)
  --clear-cache, --presto-clear-cache   clear Presto cache before starting
  --cache-dir DIR, --presto-cache-dir DIR
                                       set Presto persistent cache directory
EOF
}

is_truthy() {
  case "${1:-}" in
    1 | true | TRUE | True | yes | YES | Yes | on | ON | On)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

COMMAND="up"
if [[ $# -gt 0 && "$1" != --* ]]; then
  COMMAND="$1"
  shift
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cache | --presto-cache)
      PRESTO_CACHE=1
      ;;
    --no-cache | --no-presto-cache)
      PRESTO_CACHE=0
      ;;
    --clear-cache | --presto-clear-cache)
      PRESTO_CLEAR_CACHE=1
      ;;
    --cache-dir | --presto-cache-dir)
      if [[ $# -lt 2 ]]; then
        echo "$1 requires a directory" >&2
        usage
        exit 2
      fi
      PRESTO_CACHE_DIR="$2"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
  shift
done

JOBAGENT_HOST="${JOBAGENT_HOST:-127.0.0.1}"
JOBAGENT_PORT="${JOBAGENT_PORT:-8090}"
JOBAGENT_ADDR="${JOBAGENT_ADDR:-$JOBAGENT_HOST:$JOBAGENT_PORT}"
JOBAGENT_URL="${JOBAGENT_URL:-http://$JOBAGENT_HOST:$JOBAGENT_PORT}"

PRESTO_HOST="${PRESTO_HOST:-127.0.0.1}"
PRESTO_PORT="${PRESTO_PORT:-8080}"
PRESTO_ADDR="${PRESTO_ADDR:-$PRESTO_HOST:$PRESTO_PORT}"
PRESTO_URL="${PRESTO_URL:-http://$PRESTO_HOST:$PRESTO_PORT}"

START_PRESTO="${START_PRESTO:-1}"
START_JOBAGENT="${START_JOBAGENT:-1}"
LEGATO_USE_PRESTO="${LEGATO_USE_PRESTO:-1}"
LEGATO_PRESTO_URL="${LEGATO_PRESTO_URL:-$PRESTO_URL}"
LEGATO_TIMEOUT_MS="${LEGATO_TIMEOUT_MS:-60000}"
DIAGNOSIS_TIMEOUT_SECONDS="${DIAGNOSIS_TIMEOUT_SECONDS:-120}"
JOB_MATCHING_TIMEOUT_SECONDS="${JOB_MATCHING_TIMEOUT_SECONDS:-600}"
ITEM_BENCHMARK_MAX_REQUESTS="${ITEM_BENCHMARK_MAX_REQUESTS:-30}"
ITEM_BENCHMARK_BATCH_WORKERS="${ITEM_BENCHMARK_BATCH_WORKERS:-30}"

BACKEND_DIR="$(absolute_path "${BACKEND_DIR:-backend}")"
PRESTO_DIR="$(absolute_path "${PRESTO_DIR:-Agents/presto}")"
LEGATO_DIR="$(absolute_path "${LEGATO_DIR:-Agents/legato}")"
FRONTEND_DIR="$(absolute_path "${FRONTEND_DIR:-frontend}")"
MODEL_ROUTING_CONFIG="$(absolute_path "${MODEL_ROUTING_CONFIG:-model-routing.json}")"

RUN_DIR="$(absolute_path "${RUN_DIR:-.run}")"
LOG_DIR="$(absolute_path "${LOG_DIR:-$RUN_DIR/logs}")"
PID_DIR="$(absolute_path "${PID_DIR:-$RUN_DIR/pids}")"
BIN_DIR="$(absolute_path "${BIN_DIR:-$RUN_DIR/bin}")"
GOCACHE="$(absolute_path "${GOCACHE:-$RUN_DIR/gocache}")"
WAIT_TIMEOUT_SECONDS="${WAIT_TIMEOUT_SECONDS:-20}"
PRESTO_CACHE="${PRESTO_CACHE:-0}"
PRESTO_CLEAR_CACHE="${PRESTO_CLEAR_CACHE:-0}"
PRESTO_CACHE_DIR="$(absolute_path "${PRESTO_CACHE_DIR:-$RUN_DIR/presto-cache}")"

mkdir -p "$LOG_DIR" "$PID_DIR" "$BIN_DIR" "$GOCACHE"
if is_truthy "$PRESTO_CACHE" || is_truthy "$PRESTO_CLEAR_CACHE"; then
  mkdir -p "$PRESTO_CACHE_DIR"
fi

health_ok() {
  local url="$1"
  curl -fsS "$url" >/dev/null 2>&1
}

wait_for_health() {
  local name="$1"
  local url="$2"
  local pid="$3"
  local log_file="$4"

  for ((attempt = 1; attempt <= WAIT_TIMEOUT_SECONDS; attempt++)); do
    if health_ok "$url"; then
      echo "$name ready: $url"
      return 0
    fi
    if [[ -n "$pid" ]] && ! kill -0 "$pid" >/dev/null 2>&1; then
      echo "$name exited before becoming healthy. Log:" >&2
      tail -n 80 "$log_file" >&2 || true
      return 1
    fi
    sleep 1
  done

  echo "$name did not become healthy within ${WAIT_TIMEOUT_SECONDS}s. Log:" >&2
  tail -n 80 "$log_file" >&2 || true
  return 1
}

ensure_legato_cli() {
  local python="${LEGATO_PYTHON:-}"
  if [[ -z "$python" ]]; then
    if [[ -x "$LEGATO_DIR/.venv/bin/python" ]]; then
      python="$LEGATO_DIR/.venv/bin/python"
    else
      python="python3"
    fi
  fi

  if ! (cd "$LEGATO_DIR" && PYTHONPATH="$LEGATO_DIR${PYTHONPATH:+:$PYTHONPATH}" "$python" -m legato.cli --help >/dev/null 2>&1); then
    cat >&2 <<EOF
Legato CLI is not available.
Expected directory: $LEGATO_DIR
Python: $python

Install the offline Legato environment with:
  cd "$LEGATO_DIR"
  python3 -m venv .venv
  . .venv/bin/activate
  scripts/install_dev_offline.sh
EOF
    exit 1
  fi

  export LEGATO_PYTHON="$python"
}

start_detached() {
  local log_file="$1"
  local pid_file="$2"
  shift 2

  "$LEGATO_PYTHON" - "$log_file" "$pid_file" "$@" <<'PY'
import os
import subprocess
import sys

log_file = sys.argv[1]
pid_file = sys.argv[2]
command = sys.argv[3:]

with open(log_file, "ab", buffering=0) as log:
    process = subprocess.Popen(
        command,
        stdin=subprocess.DEVNULL,
        stdout=log,
        stderr=subprocess.STDOUT,
        close_fds=True,
        env=os.environ.copy(),
        start_new_session=True,
    )

with open(pid_file, "w", encoding="utf-8") as pid:
    pid.write(f"{process.pid}\n")
PY
}

start_presto() {
  local health_url="$PRESTO_URL/healthz"
  local pid_file="$PID_DIR/presto.pid"
  local log_file="$LOG_DIR/presto.log"

  if is_truthy "$PRESTO_CLEAR_CACHE"; then
    echo "clearing presto cache: $PRESTO_CACHE_DIR"
    rm -rf "$PRESTO_CACHE_DIR/v1"
  fi

  if health_ok "$health_url"; then
    echo "presto already healthy: $health_url"
    return 0
  fi

  if is_truthy "$PRESTO_CACHE"; then
    echo "starting presto on $PRESTO_ADDR with cache $PRESTO_CACHE_DIR"
  else
    echo "starting presto on $PRESTO_ADDR without cache"
  fi
  local bin_file="$BIN_DIR/presto"
  (
    cd "$PRESTO_DIR"
    GOCACHE="$GOCACHE" go build -o "$bin_file" ./cmd/presto
  )
  (
    cd "$PRESTO_DIR"
    export PRESTO_ADDR
    export PRESTO_ROUTE="${PRESTO_ROUTE:-legato.presto}"
    export PRESTO_ASYNC_RUN_TIMEOUT="${PRESTO_ASYNC_RUN_TIMEOUT:-10m}"
    export MODEL_ROUTING_CONFIG
    local args=("$bin_file" "--addr" "$PRESTO_ADDR")
    if is_truthy "$PRESTO_CACHE"; then
      args+=("--cache" "--cache-dir" "$PRESTO_CACHE_DIR")
    fi
    if is_truthy "$PRESTO_CLEAR_CACHE"; then
      args+=("--clear-cache" "--cache-dir" "$PRESTO_CACHE_DIR")
    fi
    start_detached "$log_file" "$pid_file" "${args[@]}"
  )
  local pid
  pid="$(cat "$pid_file")"
  wait_for_health "presto" "$health_url" "$pid" "$log_file"
}

start_jobagent() {
  local health_url="$JOBAGENT_URL/api/healthz"
  local pid_file="$PID_DIR/jobagent.pid"
  local log_file="$LOG_DIR/jobagent.log"

  if health_ok "$health_url"; then
    echo "jobagent already healthy: $health_url"
    return 0
  fi

  echo "starting jobagent on $JOBAGENT_ADDR"
  local bin_file="$BIN_DIR/jobagent"
  (
    cd "$BACKEND_DIR"
    GOCACHE="$GOCACHE" go build -o "$bin_file" .
  )
  (
    cd "$BACKEND_DIR"
    export JOBAGENT_ADDR
    export PRESTO_URL
    export LEGATO_PRESTO_URL
    export LEGATO_USE_PRESTO
    export LEGATO_TIMEOUT_MS
    export DIAGNOSIS_TIMEOUT_SECONDS
    export JOB_MATCHING_TIMEOUT_SECONDS
    export ITEM_BENCHMARK_MAX_REQUESTS
    export ITEM_BENCHMARK_BATCH_WORKERS
    export LEGATO_PYTHON
    export FRONTEND_DIR
    export MODEL_ROUTING_CONFIG
    start_detached "$log_file" "$pid_file" "$bin_file"
  )
  local pid
  pid="$(cat "$pid_file")"
  wait_for_health "jobagent" "$health_url" "$pid" "$log_file"
}

stop_pid_file() {
  local name="$1"
  local pid_file="$2"
  if [[ ! -f "$pid_file" ]]; then
    echo "$name not tracked"
    return 0
  fi

  local pid
  pid="$(cat "$pid_file")"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" >/dev/null 2>&1; then
    echo "$name not running"
    rm -f "$pid_file"
    return 0
  fi

  echo "stopping $name pid=$pid"
  kill "$pid"
  for _ in {1..10}; do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      rm -f "$pid_file"
      return 0
    fi
    sleep 1
  done
  echo "$name did not stop after SIGTERM; leaving pid file at $pid_file" >&2
  return 1
}

up() {
  require_command go
  require_command curl
  ensure_legato_cli

  export PRESTO_URL LEGATO_PRESTO_URL LEGATO_USE_PRESTO LEGATO_TIMEOUT_MS
  export JOB_MATCHING_TIMEOUT_SECONDS
  export ITEM_BENCHMARK_MAX_REQUESTS ITEM_BENCHMARK_BATCH_WORKERS
  export MODEL_ROUTING_CONFIG GOCACHE FRONTEND_DIR

  if [[ "$START_PRESTO" == "1" || "$START_PRESTO" == "true" ]]; then
    start_presto
  fi
  if [[ "$START_JOBAGENT" == "1" || "$START_JOBAGENT" == "true" ]]; then
    start_jobagent
  fi

  echo
  echo "frontend: $JOBAGENT_URL"
  echo "jobagent health: $JOBAGENT_URL/api/healthz"
  if [[ "$START_PRESTO" == "1" || "$START_PRESTO" == "true" ]]; then
    echo "presto health: $PRESTO_URL/healthz"
    if is_truthy "$PRESTO_CACHE"; then
      echo "presto cache: enabled ($PRESTO_CACHE_DIR)"
    else
      echo "presto cache: disabled"
    fi
  fi
  echo "logs: $LOG_DIR"
}

down() {
  stop_pid_file "jobagent" "$PID_DIR/jobagent.pid"
  stop_pid_file "presto" "$PID_DIR/presto.pid"
}

status() {
  if health_ok "$JOBAGENT_URL/api/healthz"; then
    echo "jobagent: up ($JOBAGENT_URL/api/healthz)"
  else
    echo "jobagent: down ($JOBAGENT_URL/api/healthz)"
  fi
  if health_ok "$PRESTO_URL/healthz"; then
    echo "presto: up ($PRESTO_URL/healthz)"
  else
    echo "presto: down ($PRESTO_URL/healthz)"
  fi
}

case "$COMMAND" in
  up)
    up
    ;;
  down)
    down
    ;;
  restart)
    down
    up
    ;;
  status)
    status
    ;;
  logs)
    tail -n 80 -f "$LOG_DIR"/*.log
    ;;
  help)
    usage
    ;;
  *)
    usage
    exit 2
    ;;
esac
