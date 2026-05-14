#!/bin/bash
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=/dev/null
. "$SCRIPT_DIR/common_compat.sh"
# shellcheck source=/dev/null
. "$SCRIPT_DIR/check_config.sh"

echo "[INFO] 主机系统类型: $(detect_os_family), 架构: $(detect_arch)"
ensure_cmds awk grep ps pgrep bc ls wc || {
  echo "[ERROR] 关键命令准备失败，终止进程巡检"
  exit 1
}

log() { echo "[INFO] $1"; }
warn() { echo "[WARNING] $1"; }
error() { echo "[ERROR] $1"; }

collect_targets() {
  if [ -n "${1:-}" ]; then
    echo "$1"
    return
  fi
  echo "$PROCESS_TARGETS" | tr ',' ' '
}

check_one_pid() {
  local pid="$1"
  local proc="$2"
  local stat cpu1 cpu2 blocked_threads fd_count

  # 获取进程全名（完整命令行），截取前 80 字符防止过长
  local proc_name
  proc_name="$(ps -p "$pid" -o cmd= 2>/dev/null | head -c 80)"
  [ -z "$proc_name" ] && proc_name="$proc"

  log "检查进程: $proc_name, PID: $pid"
  stat="$(ps -o stat= -p "$pid" 2>/dev/null | tr -d ' ')"

  [[ "$stat" == *Z* ]] && { error "[$proc_name:$pid] 僵尸进程（Zombie）"; return; }
  [[ "$stat" == *D* ]] && warn "[$proc_name:$pid] 进程处于不可中断睡眠（可能IO阻塞或死锁）"
  [[ "$stat" == *R* ]] && log "进程正在运行（Running）"
  [[ "$stat" == *S* ]] && log "进程处于休眠（Sleeping）"

  cpu1="$(ps -p "$pid" -o %cpu= 2>/dev/null | xargs)"
  sleep "$CHECK_INTERVAL"
  cpu2="$(ps -p "$pid" -o %cpu= 2>/dev/null | xargs)"
  if (( $(echo "${cpu1:-0} < $CPU_IDLE_THRESHOLD && ${cpu2:-0} < $CPU_IDLE_THRESHOLD" | bc -l) )); then
    warn "[$proc_name:$pid] CPU占用持续过低（可能假死或无响应）"
  fi

  blocked_threads="$(ps -L -p "$pid" -o stat= 2>/dev/null | grep -c D)"
  [ "${blocked_threads:-0}" -gt "$THREAD_BLOCK_THRESHOLD" ] && error "[$proc_name:$pid] 大量线程阻塞（疑似死锁）: $blocked_threads"

  if [ -d "/proc/$pid/fd" ]; then
    fd_count="$(ls "/proc/$pid/fd" 2>/dev/null | wc -l | tr -d ' ')"
    log "打开文件描述符数量: $fd_count"
    [ "$fd_count" -gt "$FD_WARNING_THRESHOLD" ] && warn "[$proc_name:$pid] 文件描述符过多（可能资源泄漏），当前: $fd_count"
  fi
}

echo "=========================================="
echo "      进程健康检查开始"
echo "=========================================="

TARGETS="$(collect_targets "${1:-}")"
if [ -z "$TARGETS" ]; then
  error "未配置要检查的进程。请在 check_config.sh 的 PROCESS_TARGETS 中配置"
  exit 1
fi

for proc in $TARGETS; do
  echo "------------------------------------------"
  echo "[INFO] 进程健康检查: $proc"
  pids="$(pgrep -f "$proc" 2>/dev/null)"
  if [ -z "$pids" ]; then
    warn "进程未运行: $proc"
    continue
  fi
  for pid in $pids; do
    check_one_pid "$pid" "$proc"
  done
done

echo "=========================================="
echo "      进程健康检查完成"
echo "=========================================="
