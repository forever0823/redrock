#!/usr/bin/env bash
set -euo pipefail

APP_NAME="bash-check-controller"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="${CONTROLLER_BIN:-$SCRIPT_DIR/bin/controller-linux-amd64}"
LOG_DIR="${CONTROLLER_LOG_DIR:-$SCRIPT_DIR/log}"
LOG_FILE="${CONTROLLER_LOG:-$LOG_DIR/controller.log}"
PID_FILE="${CONTROLLER_PID:-$SCRIPT_DIR/controller.pid}"

usage() {
  cat <<EOF
Usage: ./start_controller.sh {start|stop|restart|status|logs}

Environment overrides:
  CONTROLLER_BIN=/path/to/controller-linux-amd64
  CONTROLLER_LOG_DIR=/path/to/log
  CONTROLLER_LOG=/path/to/log/controller.log
  CONTROLLER_PID=/path/to/controller.pid
EOF
}

is_running() {
  local pid
  if [ ! -f "$PID_FILE" ]; then
    return 1
  fi
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

start_controller() {
  if [ ! -x "$BIN" ]; then
    echo "[ERROR] controller binary not found or not executable: $BIN"
    echo "Run: chmod +x $BIN"
    exit 1
  fi

  if is_running; then
    echo "[INFO] $APP_NAME is already running, pid=$(cat "$PID_FILE")"
    exit 0
  fi

  mkdir -p "$(dirname "$LOG_FILE")"
  cd "$SCRIPT_DIR"

  nohup "$BIN" >> "$LOG_FILE" 2>&1 &
  echo $! > "$PID_FILE"

  sleep 1
  if is_running; then
    echo "[INFO] $APP_NAME started, pid=$(cat "$PID_FILE")"
    echo "[INFO] log file: $LOG_FILE"
  else
    echo "[ERROR] $APP_NAME failed to start. Recent log:"
    tail -n 40 "$LOG_FILE" 2>/dev/null || true
    exit 1
  fi
}

stop_controller() {
  if ! is_running; then
    echo "[INFO] $APP_NAME is not running"
    rm -f "$PID_FILE"
    return 0
  fi

  local pid
  pid="$(cat "$PID_FILE")"
  echo "[INFO] stopping $APP_NAME, pid=$pid"
  kill "$pid" 2>/dev/null || true

  for _ in $(seq 1 10); do
    if ! kill -0 "$pid" 2>/dev/null; then
      rm -f "$PID_FILE"
      echo "[INFO] stopped"
      return 0
    fi
    sleep 1
  done

  echo "[WARNING] graceful stop timed out, killing pid=$pid"
  kill -9 "$pid" 2>/dev/null || true
  rm -f "$PID_FILE"
  echo "[INFO] stopped"
}

status_controller() {
  if is_running; then
    local pid
    pid="$(cat "$PID_FILE")"
    echo "[INFO] $APP_NAME is running, pid=$pid"
    ps -p "$pid" -o pid,ppid,etime,cmd
    echo "[INFO] listening ports:"
    ss -lntp 2>/dev/null | grep "$pid" || true
  else
    echo "[INFO] $APP_NAME is not running"
    [ -f "$PID_FILE" ] && echo "[WARNING] stale pid file: $PID_FILE"
    return 1
  fi
}

logs_controller() {
  mkdir -p "$(dirname "$LOG_FILE")"
  touch "$LOG_FILE"
  echo "[INFO] tailing log file: $LOG_FILE"
  tail -f "$LOG_FILE"
}

case "${1:-}" in
  start)
    start_controller
    ;;
  stop)
    stop_controller
    ;;
  restart)
    stop_controller
    start_controller
    ;;
  status)
    status_controller
    ;;
  logs)
    logs_controller
    ;;
  *)
    usage
    exit 1
    ;;
esac
