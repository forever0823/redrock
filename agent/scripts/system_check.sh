#!/bin/bash
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=/dev/null
. "$SCRIPT_DIR/common_compat.sh"
# shellcheck source=/dev/null
. "$SCRIPT_DIR/check_config.sh"

echo "[INFO] 主机系统类型: $(detect_os_family), 架构: $(detect_arch)"
ensure_cmds awk sed grep bc df du ps uptime who top free || {
  echo "[ERROR] 关键命令准备失败，终止系统巡检"
  exit 1
}

HOSTNAME="$(hostname)"
DATE="$(date +"%Y-%m-%d %H:%M:%S")"
REPORT_FILE="/tmp/system_check_$(date +%Y%m%d_%H%M%S).log"

log() { echo "[$(date +"%Y-%m-%d %H:%M:%S")] $1" | tee -a "$REPORT_FILE"; }
warn() { echo "[WARNING] $1" | tee -a "$REPORT_FILE"; }
ok() { echo "[INFO] $1" | tee -a "$REPORT_FILE"; }

get_cpu_usage() {
  top -bn2 -d 1 2>/dev/null | awk '/Cpu\(s\)/{v=100-$8} END{if(v!="") printf "%.2f", v; else print "0"}'
}

get_mem_usage() {
  free -m | awk 'NR==2{if($2>0) printf "%.2f", ($3/$2)*100; else print "0"}'
}

log "主机名: $HOSTNAME"
log "检查时间: $DATE"
log "内核版本: $(uname -r)"
log "系统架构: $(uname -m)"
log "运行时长: $(uptime 2>/dev/null | sed 's/.*up //;s/, [0-9]* users.*//')"

CPU_USAGE="$(get_cpu_usage)"
MEM_USAGE="$(get_mem_usage)"
log "CPU使用率: ${CPU_USAGE}%"
log "内存使用率: ${MEM_USAGE}%"

if (( $(echo "$CPU_USAGE > $CPU_WARNING" | bc -l) )); then
  warn "CPU使用率超过${CPU_WARNING}%"
fi
if (( $(echo "$MEM_USAGE > $MEM_WARNING" | bc -l) )); then
  warn "内存使用率超过${MEM_WARNING}%"
fi

LOADS="$(uptime 2>/dev/null | sed -n 's/.*load average[s]*: //p')"
if [ -n "$LOADS" ]; then
  LOAD_1="$(echo "$LOADS" | awk -F',' '{gsub(/ /,"",$1); print $1}')"
  LOAD_5="$(echo "$LOADS" | awk -F',' '{gsub(/ /,"",$2); print $2}')"
  LOAD_15="$(echo "$LOADS" | awk -F',' '{gsub(/ /,"",$3); print $3}')"
  log "系统负载: 1分钟=${LOAD_1}, 5分钟=${LOAD_5}, 15分钟=${LOAD_15}"
fi

while read -r line; do
  usage="$(echo "$line" | awk '{print $5}' | tr -d '%')"
  mount="$(echo "$line" | awk '{print $6}')"
  [ -z "$usage" ] && continue
  if [ "$usage" -gt "$DISK_WARNING" ] 2>/dev/null; then
    warn "磁盘分区 $mount 使用率${usage}%超过阈值${DISK_WARNING}%"
  fi
done < <(df -h 2>/dev/null | awk 'NR>1')

ok "系统巡检完成，报告文件: $REPORT_FILE"
