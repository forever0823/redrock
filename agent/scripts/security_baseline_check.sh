#!/bin/bash
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=/dev/null
. "$SCRIPT_DIR/common_compat.sh"
# shellcheck source=/dev/null
. "$SCRIPT_DIR/check_config.sh"

: "${SSH_CONFIG_FILE:=/etc/ssh/sshd_config}"
: "${MAX_PASSWORD_DAYS:=90}"
: "${MIN_PASSWORD_LENGTH:=8}"

OS_FAMILY="$(detect_os_family)"
ARCH="$(detect_arch)"

echo "[INFO] 主机系统类型: $OS_FAMILY, 架构: $ARCH"
ensure_cmds awk grep sed stat wc ps pgrep find sort uniq head tr cat date hostname uname || {
  echo "[ERROR] 关键命令准备失败，终止安全基线巡检"
  exit 1
}

log()   { echo "[INFO] $1"; }
warn()  { echo "[WARNING] $1"; }
error() { echo "[ERROR] $1"; }

section() {
  echo ""
  echo "========== $1 =========="
}

join_lines() {
  sed '/^[[:space:]]*$/d' | tr '\n' ' ' | sed 's/[[:space:]]*$//'
}

lower() {
  printf "%s" "$1" | tr '[:upper:]' '[:lower:]'
}

read_kv_value() {
  local file="$1" key="$2"
  [ -r "$file" ] || return 1
  awk -v k="$(lower "$key")" '
    /^[[:space:]]*#/ { next }
    {
      line=$0
      sub(/[[:space:]]+#.*/, "", line)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line == "") next
      split(line, a, /[[:space:]=]+/)
      if (tolower(a[1]) == k && a[2] != "") print a[2]
    }
  ' "$file" | tail -1
}

get_perm() {
  stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1" 2>/dev/null
}

get_owner_group() {
  stat -c '%U:%G' "$1" 2>/dev/null || stat -f '%Su:%Sg' "$1" 2>/dev/null
}

perm_no_more_permissive() {
  local perm="$1" max="$2"
  [ -n "$perm" ] || return 1
  [ -n "$max" ] || return 1
  [ "$perm" = "0" ] && perm="000"
  [ "$max" = "0" ] && max="000"
  [ $((8#$perm & ~8#$max)) -eq 0 ] 2>/dev/null
}

check_file_perm_owner() {
  local file="$1" max_perm="$2" owners="$3" desc="$4"
  local perm owner

  if [ ! -e "$file" ]; then
    warn "$desc 不存在: $file"
    return
  fi

  perm="$(get_perm "$file")"
  owner="$(get_owner_group "$file")"

  if perm_no_more_permissive "$perm" "$max_perm"; then
    log "$desc 权限正常 ($file: $perm)"
  else
    error "$desc 权限异常（$file 当前:$perm，建议不高于:$max_perm）"
  fi

  case " $owners " in
    *" $owner "*) log "$desc 属主属组正常 ($file: $owner)" ;;
    *) warn "$desc 属主属组异常（$file 当前:$owner，建议:$owners）" ;;
  esac
}

check_dir_perm_owner() {
  local dir="$1" max_perm="$2" owners="$3" desc="$4"
  local perm owner

  [ -d "$dir" ] || return
  perm="$(get_perm "$dir")"
  owner="$(get_owner_group "$dir")"

  if perm_no_more_permissive "$perm" "$max_perm"; then
    log "$desc 目录权限正常 ($dir: $perm)"
  else
    warn "$desc 目录权限过宽（$dir 当前:$perm，建议不高于:$max_perm）"
  fi

  case " $owners " in
    *" $owner "*) log "$desc 目录属主属组正常 ($dir: $owner)" ;;
    *) warn "$desc 目录属主属组异常（$dir 当前:$owner，建议:$owners）" ;;
  esac
}

check_file_not_writable_by_group_other() {
  local file="$1" desc="$2"
  local perm
  [ -e "$file" ] || return
  perm="$(get_perm "$file")"
  if [ $((8#$perm & 8#022)) -eq 0 ] 2>/dev/null; then
    log "$desc 不可被 group/other 写入 ($file: $perm)"
  else
    error "$desc 存在 group/other 写权限（$file 当前:$perm）"
  fi
}

sysctl_value() {
  local path="$1"
  [ -r "$path" ] || return 1
  cat "$path" 2>/dev/null
}

check_sysctl_eq() {
  local name="$1" path="$2" expected="$3" level="$4" reason="$5"
  local val
  val="$(sysctl_value "$path")"
  if [ -z "$val" ]; then
    warn "$name 无法读取，跳过"
    return
  fi
  if [ "$val" = "$expected" ]; then
    log "$name=$val（符合基线）"
  else
    case "$level" in
      error) error "$name=$val（建议=$expected，$reason）" ;;
      *) warn "$name=$val（建议=$expected，$reason）" ;;
    esac
  fi
}

check_sysctl_min() {
  local name="$1" path="$2" min="$3" level="$4" reason="$5"
  local val
  val="$(sysctl_value "$path")"
  if [ -z "$val" ]; then
    warn "$name 无法读取，跳过"
    return
  fi
  if [ "$val" -ge "$min" ] 2>/dev/null; then
    log "$name=$val（符合基线）"
  else
    case "$level" in
      error) error "$name=$val（建议>=$min，$reason）" ;;
      *) warn "$name=$val（建议>=$min，$reason）" ;;
    esac
  fi
}

existing_pam_files() {
  local f
  for f in /etc/pam.d/system-auth /etc/pam.d/password-auth /etc/pam.d/common-password /etc/pam.d/common-auth /etc/pam.d/sshd; do
    [ -r "$f" ] && printf "%s\n" "$f"
  done
}

pwquality_value() {
  local key="$1" f
  for f in /etc/security/pwquality.conf /etc/security/pwquality.conf.d/*.conf; do
    [ -r "$f" ] || continue
    read_kv_value "$f" "$key"
  done | tail -1
}

mount_options() {
  local mount_point="$1"
  if command -v findmnt >/dev/null 2>&1; then
    findmnt -n -o OPTIONS --target "$mount_point" 2>/dev/null | head -1
    return
  fi
  mount 2>/dev/null | awk -v m="$mount_point" '$3 == m {gsub(/[()]/,"",$6); print $6; exit}'
}

check_mount_option() {
  local mount_point="$1" option="$2"
  local opts
  opts="$(mount_options "$mount_point")"
  if [ -z "$opts" ]; then
    warn "$mount_point 未单独挂载或无法读取挂载参数，跳过 $option 检查"
    return
  fi
  if printf ",%s," "$opts" | grep -q ",$option,"; then
    log "$mount_point 已启用 $option"
  else
    warn "$mount_point 未启用 $option（当前: $opts）"
  fi
}

service_active() {
  local svc="$1"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl is-active --quiet "$svc" 2>/dev/null && return 0
  fi
  pgrep -x "$svc" >/dev/null 2>&1
}

echo "=========================================="
echo "      安全基线检查开始"
echo "=========================================="

# ============================================================
# 1. 账户与身份认证安全
# ============================================================
section "1. 账户与身份认证安全"

if [ -r /etc/shadow ]; then
  EMPTY_PASS_USERS="$(awk -F: '($2==""){print $1}' /etc/shadow | join_lines)"
  [ -n "$EMPTY_PASS_USERS" ] && error "存在空密码账户: $EMPTY_PASS_USERS" || log "未发现空密码账户"

  LOCKED_USERS="$(awk -F: '($2~/^[!*]/){print $1}' /etc/shadow | wc -l | tr -d ' ')"
  log "已锁定或禁用密码的账户数量: ${LOCKED_USERS:-0}"

  LONG_AGING_USERS="$(awk -F: -v max="$MAX_PASSWORD_DAYS" '
    ($2!="" && $2!~/^[!*]/) && ($5=="" || $5>max) {
      printf "%s(max=%s)\n", $1, ($5=="" ? "未设置" : $5)
    }
  ' /etc/shadow | join_lines)"
  [ -n "$LONG_AGING_USERS" ] && warn "存在密码最长使用期超过 ${MAX_PASSWORD_DAYS} 天或未设置的账户: $LONG_AGING_USERS" || log "账户密码最长使用期未发现超限"

  NO_WARN_USERS="$(awk -F: '($2!="" && $2!~/^[!*]/) && ($6=="" || $6<7) {printf "%s(warn=%s)\n", $1, ($6=="" ? "未设置" : $6)}' /etc/shadow | join_lines)"
  [ -n "$NO_WARN_USERS" ] && warn "存在密码到期提醒不足 7 天的账户: $NO_WARN_USERS" || log "账户密码到期提醒配置正常"
else
  warn "/etc/shadow 不可读，跳过影子口令检查"
fi

PASSWD_EMPTY_USERS="$(awk -F: '($2==""){print $1}' /etc/passwd 2>/dev/null | join_lines)"
[ -n "$PASSWD_EMPTY_USERS" ] && error "/etc/passwd 存在空密码字段账户: $PASSWD_EMPTY_USERS" || log "/etc/passwd 未发现空密码字段"

UID0_USERS="$(awk -F: '($3==0){print $1}' /etc/passwd 2>/dev/null | join_lines)"
UID0_COUNT="$(printf "%s\n" "$UID0_USERS" | sed 's/[[:space:]]\+/\n/g' | sed '/^$/d' | wc -l | tr -d ' ')"
[ "$UID0_COUNT" -gt 1 ] && error "存在多个 UID=0 账户: $UID0_USERS" || log "UID=0 账户个数: $UID0_COUNT"

SYS_LOGIN="$(awk -F: '$3>=100 && $3<1000 && $7!~/(nologin|false|sync|shutdown|halt)$/ {printf "%s(uid=%s,shell=%s)\n", $1,$3,$7}' /etc/passwd 2>/dev/null | join_lines)"
[ -n "$SYS_LOGIN" ] && warn "存在可登录的系统账户: $SYS_LOGIN" || log "未发现可登录的系统账户"

DUP_UID="$(awk -F: '{print $3}' /etc/passwd 2>/dev/null | sort -n | uniq -d | join_lines)"
[ -n "$DUP_UID" ] && error "存在重复 UID: $DUP_UID" || log "UID 唯一性检查正常"

DUP_GID="$(awk -F: '{print $3}' /etc/group 2>/dev/null | sort -n | uniq -d | join_lines)"
[ -n "$DUP_GID" ] && warn "存在重复 GID: $DUP_GID" || log "GID 唯一性检查正常"

DUP_USER="$(awk -F: '{print $1}' /etc/passwd 2>/dev/null | sort | uniq -d | join_lines)"
[ -n "$DUP_USER" ] && error "存在重复用户名: $DUP_USER" || log "用户名唯一性检查正常"

DUP_GROUP="$(awk -F: '{print $1}' /etc/group 2>/dev/null | sort | uniq -d | join_lines)"
[ -n "$DUP_GROUP" ] && error "存在重复用户组名: $DUP_GROUP" || log "用户组名唯一性检查正常"

if [ -r /etc/shadow ]; then
  ROOT_LOCK="$(awk -F: '$1=="root" && $2~/^[!*]/ {print "locked"}' /etc/shadow)"
  [ -n "$ROOT_LOCK" ] && log "root 账户已锁定（密码字段已锁定）" || warn "root 账户密码未锁定，需确保 SSH 禁止 root 远程登录并限制本地使用"
fi

awk -F: '$3>=1000 && $1!="nobody" && $7!~/(nologin|false)$/ {print $1 ":" $3 ":" $6}' /etc/passwd 2>/dev/null |
while IFS=: read -r user uid home; do
  [ -n "$user" ] || continue
  if [ ! -d "$home" ]; then
    warn "登录账户 $user 的家目录不存在: $home"
    continue
  fi
  home_uid="$(stat -c %u "$home" 2>/dev/null)"
  [ "$home_uid" = "$uid" ] && log "账户 $user 家目录属主正常 ($home)" || warn "账户 $user 家目录属主异常 ($home owner_uid=$home_uid expected_uid=$uid)"
  home_perm="$(get_perm "$home")"
  if [ -n "$home_perm" ] && [ $((8#$home_perm & 8#002)) -ne 0 ] 2>/dev/null; then
    warn "账户 $user 家目录允许 other 写入 ($home: $home_perm)"
  fi
done

# ============================================================
# 2. SSH 远程访问安全
# ============================================================
section "2. SSH 远程访问安全"

SSHD_T_OUTPUT=""
SSHD_SOURCE="配置文件解析"
if command -v sshd >/dev/null 2>&1; then
  SSHD_HOST="$(hostname 2>/dev/null || echo localhost)"
  SSHD_T_OUTPUT="$(sshd -T -C "user=root,host=$SSHD_HOST,addr=127.0.0.1" 2>/dev/null)"
  [ -n "$SSHD_T_OUTPUT" ] && SSHD_SOURCE="sshd -T 有效配置"
fi

ssh_value() {
  local key="$1" key_lc
  key_lc="$(lower "$key")"
  if [ -n "$SSHD_T_OUTPUT" ]; then
    printf "%s\n" "$SSHD_T_OUTPUT" | awk -v k="$key_lc" 'tolower($1)==k {print $2; exit}'
    return
  fi
  [ -r "$SSH_CONFIG_FILE" ] || return 1
  awk -v k="$key_lc" '
    /^[[:space:]]*#/ { next }
    {
      line=$0
      sub(/[[:space:]]+#.*/, "", line)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      split(line, a, /[[:space:]]+/)
      if (tolower(a[1]) == k && a[2] != "") print a[2]
    }
  ' "$SSH_CONFIG_FILE" | tail -1
}

ssh_check_eq() {
  local key="$1" expected="$2" level="$3" msg="$4"
  local val
  val="$(ssh_value "$key")"
  if [ -z "$val" ]; then
    log "$key: 未显式配置（来源: $SSHD_SOURCE）"
    return
  fi
  if [ "$(lower "$val")" = "$(lower "$expected")" ]; then
    log "$key=$val（符合基线，来源: $SSHD_SOURCE）"
  else
    case "$level" in
      error) error "$key=$val（建议=$expected，$msg）" ;;
      *) warn "$key=$val（建议=$expected，$msg）" ;;
    esac
  fi
}

ssh_check_num_le() {
  local key="$1" max="$2" level="$3" msg="$4"
  local val
  val="$(ssh_value "$key")"
  if [ -z "$val" ]; then
    warn "$key 未配置，建议不超过 $max"
    return
  fi
  if [ "$val" -le "$max" ] 2>/dev/null; then
    log "$key=$val（符合基线）"
  else
    case "$level" in
      error) error "$key=$val（建议不超过 $max，$msg）" ;;
      *) warn "$key=$val（建议不超过 $max，$msg）" ;;
    esac
  fi
}

ssh_check_weak_algorithms() {
  local key="$1" pattern="$2" desc="$3"
  local val
  val="$(ssh_value "$key")"
  [ -n "$val" ] || return
  if printf "%s" "$val" | grep -Eiq "$pattern"; then
    warn "$key 包含弱算法（$desc）: $val"
  else
    log "$key 未发现已知弱算法"
  fi
}

if [ -f "$SSH_CONFIG_FILE" ] || [ -n "$SSHD_T_OUTPUT" ]; then
  ROOT_LOGIN="$(ssh_value "PermitRootLogin")"
  case "$(lower "$ROOT_LOGIN")" in
    no) log "PermitRootLogin=$ROOT_LOGIN（禁止 root 远程登录）" ;;
    yes) error "PermitRootLogin=yes（允许 root 远程登录）" ;;
    without-password|prohibit-password|forced-commands-only) warn "PermitRootLogin=$ROOT_LOGIN（仍允许 root 通过受限方式远程登录，建议 no）" ;;
    "") log "PermitRootLogin: 未显式配置（来源: $SSHD_SOURCE）" ;;
    *) warn "PermitRootLogin=$ROOT_LOGIN（建议 no）" ;;
  esac

  ssh_check_eq "PasswordAuthentication" "no" error "建议禁用口令登录，改用密钥或更强认证"
  ssh_check_eq "PermitEmptyPasswords" "no" error "允许空密码登录"
  ssh_check_eq "PubkeyAuthentication" "yes" warn "建议启用公钥认证"
  ssh_check_eq "HostbasedAuthentication" "no" warn "禁止基于主机信任认证"
  ssh_check_eq "IgnoreRhosts" "yes" warn "忽略 rhosts 信任文件"
  ssh_check_eq "PermitUserEnvironment" "no" warn "允许用户环境变量，存在提权风险"
  ssh_check_eq "X11Forwarding" "no" warn "建议关闭 X11 转发"
  ssh_check_eq "AllowTcpForwarding" "no" warn "允许 TCP 端口转发，存在跳板风险"
  ssh_check_eq "AllowAgentForwarding" "no" warn "关闭 SSH agent 转发"
  ssh_check_eq "GatewayPorts" "no" warn "禁止远程主机绑定转发端口"
  ssh_check_eq "PermitTunnel" "no" warn "禁止 SSH 隧道设备"
  ssh_check_eq "UsePAM" "yes" warn "关闭 PAM 可能削弱认证强度"

  KBD_AUTH="$(ssh_value "KbdInteractiveAuthentication")"
  [ -z "$KBD_AUTH" ] && KBD_AUTH="$(ssh_value "ChallengeResponseAuthentication")"
  case "$(lower "$KBD_AUTH")" in
    no) log "KbdInteractiveAuthentication/ChallengeResponseAuthentication=$KBD_AUTH（符合基线）" ;;
    yes) warn "KbdInteractiveAuthentication/ChallengeResponseAuthentication=yes（如未配合 MFA，可能绕过口令禁用策略）" ;;
    "") log "KbdInteractiveAuthentication/ChallengeResponseAuthentication: 未显式配置" ;;
    *) warn "KbdInteractiveAuthentication/ChallengeResponseAuthentication=$KBD_AUTH（建议 no 或配合 MFA）" ;;
  esac

  ssh_check_num_le "MaxAuthTries" 4 warn "限制暴力破解尝试次数"
  ssh_check_num_le "LoginGraceTime" 60 warn "缩短未认证连接停留时间"
  ssh_check_num_le "ClientAliveCountMax" 3 warn "限制空闲会话探测次数"
  ssh_check_num_le "MaxSessions" 10 warn "限制单连接会话数量"

  CLIENT_ALIVE_INTERVAL="$(ssh_value "ClientAliveInterval")"
  if [ -n "$CLIENT_ALIVE_INTERVAL" ]; then
    if [ "$CLIENT_ALIVE_INTERVAL" -eq 0 ] 2>/dev/null || [ "$CLIENT_ALIVE_INTERVAL" -gt 900 ] 2>/dev/null; then
      warn "ClientAliveInterval=$CLIENT_ALIVE_INTERVAL（建议 1~900 秒，避免长期空闲会话）"
    else
      log "ClientAliveInterval=$CLIENT_ALIVE_INTERVAL（符合基线）"
    fi
  else
    warn "ClientAliveInterval 未配置，建议设置空闲会话超时"
  fi

  MAX_STARTUPS="$(ssh_value "MaxStartups")"
  [ -n "$MAX_STARTUPS" ] && log "MaxStartups=$MAX_STARTUPS" || warn "MaxStartups 未配置，建议限制未认证并发连接"

  ALLOW_USERS="$(ssh_value "AllowUsers")"
  ALLOW_GROUPS="$(ssh_value "AllowGroups")"
  DENY_USERS="$(ssh_value "DenyUsers")"
  DENY_GROUPS="$(ssh_value "DenyGroups")"
  if [ -n "$ALLOW_USERS$ALLOW_GROUPS$DENY_USERS$DENY_GROUPS" ]; then
    log "SSH 已配置用户/组访问控制（Allow/DenyUsers/Groups）"
  else
    warn "SSH 未配置 AllowUsers/AllowGroups/DenyUsers/DenyGroups，建议按最小权限限制可登录主体"
  fi

  PROTO="$(ssh_value "Protocol")"
  [ -n "$PROTO" ] && [ "$PROTO" != "2" ] && warn "SSH Protocol 非 2: $PROTO"

  ssh_check_weak_algorithms "Ciphers" "3des-cbc|aes[0-9]+-cbc|arcfour|blowfish-cbc|cast128-cbc" "CBC/RC4/3DES"
  ssh_check_weak_algorithms "MACs" "hmac-md5|hmac-sha1($|,)|umac-64|hmac-ripemd160" "MD5/SHA1/64-bit MAC"
  ssh_check_weak_algorithms "KexAlgorithms" "diffie-hellman-group1-sha1|diffie-hellman-group14-sha1|diffie-hellman-group-exchange-sha1" "SHA1/弱 DH 组"
else
  warn "未找到 SSH 配置文件且无法读取 sshd -T: $SSH_CONFIG_FILE"
fi

# ============================================================
# 3. 密码、PAM 与 sudo 策略
# ============================================================
section "3. 密码、PAM 与 sudo 策略"

if [ -f /etc/login.defs ]; then
  max_days="$(read_kv_value /etc/login.defs PASS_MAX_DAYS)"
  min_days="$(read_kv_value /etc/login.defs PASS_MIN_DAYS)"
  warn_age="$(read_kv_value /etc/login.defs PASS_WARN_AGE)"
  min_len="$(read_kv_value /etc/login.defs PASS_MIN_LEN)"
  encrypt_method="$(read_kv_value /etc/login.defs ENCRYPT_METHOD)"
  default_umask="$(read_kv_value /etc/login.defs UMASK)"

  [ -n "$max_days" ] && [ "$max_days" -le "$MAX_PASSWORD_DAYS" ] 2>/dev/null && log "PASS_MAX_DAYS=$max_days（符合基线）" || warn "PASS_MAX_DAYS=${max_days:-未配置}（建议 <= $MAX_PASSWORD_DAYS）"
  [ -n "$min_days" ] && [ "$min_days" -ge 1 ] 2>/dev/null && log "PASS_MIN_DAYS=$min_days（符合基线）" || warn "PASS_MIN_DAYS=${min_days:-未配置}（建议 >= 1）"
  [ -n "$warn_age" ] && [ "$warn_age" -ge 7 ] 2>/dev/null && log "PASS_WARN_AGE=$warn_age（符合基线）" || warn "PASS_WARN_AGE=${warn_age:-未配置}（建议 >= 7）"
  [ -n "$min_len" ] && [ "$min_len" -ge "$MIN_PASSWORD_LENGTH" ] 2>/dev/null && log "PASS_MIN_LEN=$min_len（符合配置阈值 $MIN_PASSWORD_LENGTH）" || warn "PASS_MIN_LEN=${min_len:-未配置}（建议 >= $MIN_PASSWORD_LENGTH）"

  case "$(lower "${encrypt_method:-}")" in
    yescrypt|sha512) log "ENCRYPT_METHOD=${encrypt_method}（符合基线）" ;;
    "") warn "ENCRYPT_METHOD 未配置，建议 yescrypt 或 SHA512" ;;
    *) warn "ENCRYPT_METHOD=$encrypt_method（建议 yescrypt 或 SHA512，避免 DES/MD5）" ;;
  esac

  case "$default_umask" in
    027|077) log "UMASK=$default_umask（符合基线）" ;;
    "") warn "UMASK 未配置，建议 027 或 077" ;;
    *) warn "UMASK=$default_umask（建议 027 或 077，降低默认文件权限）" ;;
  esac
else
  warn "/etc/login.defs 不存在，跳过登录策略检查"
fi

PAM_FILES="$(existing_pam_files | tr '\n' ' ')"
if [ -n "$PAM_FILES" ]; then
  if grep -Ehs 'pam_(pwquality|cracklib)\.so' $PAM_FILES >/dev/null 2>&1; then
    log "PAM 已启用密码复杂度模块（pam_pwquality/pam_cracklib）"
  else
    warn "PAM 未发现密码复杂度模块（pam_pwquality/pam_cracklib）"
  fi

  if grep -Ehs 'pam_(faillock|tally2)\.so' $PAM_FILES >/dev/null 2>&1; then
    log "PAM 已启用登录失败锁定模块（pam_faillock/pam_tally2）"
  else
    warn "PAM 未发现登录失败锁定模块，建议配置失败锁定策略"
  fi

  if grep -Ehs 'pam_pwhistory\.so|remember=[0-9]+' $PAM_FILES >/dev/null 2>&1; then
    log "PAM 已配置密码历史限制"
  else
    warn "PAM 未发现密码历史限制，建议防止重复使用旧密码"
  fi
else
  warn "未发现可读 PAM 配置文件"
fi

PWQ_MINLEN="$(pwquality_value minlen)"
PWQ_MINCLASS="$(pwquality_value minclass)"
PWQ_DCREDIT="$(pwquality_value dcredit)"
PWQ_UCREDIT="$(pwquality_value ucredit)"
PWQ_LCREDIT="$(pwquality_value lcredit)"
PWQ_OCREDIT="$(pwquality_value ocredit)"

[ -n "$PWQ_MINLEN" ] && [ "$PWQ_MINLEN" -ge 12 ] 2>/dev/null && log "pwquality minlen=$PWQ_MINLEN（符合基线）" || warn "pwquality minlen=${PWQ_MINLEN:-未配置}（建议 >= 12）"
if [ -n "$PWQ_MINCLASS" ]; then
  [ "$PWQ_MINCLASS" -ge 3 ] 2>/dev/null && log "pwquality minclass=$PWQ_MINCLASS（符合基线）" || warn "pwquality minclass=$PWQ_MINCLASS（建议 >= 3）"
else
  CREDIT_SCORE=0
  for v in "$PWQ_DCREDIT" "$PWQ_UCREDIT" "$PWQ_LCREDIT" "$PWQ_OCREDIT"; do
    [ -n "$v" ] && [ "$v" -lt 0 ] 2>/dev/null && CREDIT_SCORE=$((CREDIT_SCORE + 1))
  done
  [ "$CREDIT_SCORE" -ge 3 ] && log "pwquality 字符类别限制已覆盖 $CREDIT_SCORE 类" || warn "pwquality 字符类别限制不足，建议 minclass>=3 或 d/u/l/ocredit 至少三类为负值"
fi

FAILLOCK_DENY="$(grep -Ehs '(^|[[:space:]])deny[[:space:]]*=' /etc/security/faillock.conf $PAM_FILES 2>/dev/null | tail -1 | sed -n 's/.*deny[[:space:]]*=[[:space:]]*\([0-9]\+\).*/\1/p')"
[ -n "$FAILLOCK_DENY" ] && [ "$FAILLOCK_DENY" -le 5 ] 2>/dev/null && log "登录失败锁定 deny=$FAILLOCK_DENY（符合基线）" || warn "登录失败锁定 deny=${FAILLOCK_DENY:-未配置}（建议 <= 5）"

if [ -r /etc/sudoers ] || [ -d /etc/sudoers.d ]; then
  SUDO_NOPASSWD="$(grep -REhs '^[^#].*(NOPASSWD|!authenticate)' /etc/sudoers /etc/sudoers.d 2>/dev/null | head -10)"
  [ -n "$SUDO_NOPASSWD" ] && warn "sudo 存在免密授权（展示前 10 条）: $(printf "%s" "$SUDO_NOPASSWD" | tr '\n' '; ')" || log "sudo 未发现免密授权"

  if grep -REhs '^[[:space:]]*Defaults[[:space:]].*use_pty' /etc/sudoers /etc/sudoers.d >/dev/null 2>&1; then
    log "sudo 已启用 use_pty"
  else
    warn "sudo 未启用 Defaults use_pty，建议降低 TTY 注入风险"
  fi

  SUDO_ADMINS="$(awk -F: '$1=="sudo" || $1=="wheel" {print $1 ":" $4}' /etc/group 2>/dev/null | join_lines)"
  [ -n "$SUDO_ADMINS" ] && log "sudo/wheel 管理组成员: $SUDO_ADMINS" || log "未发现 sudo/wheel 管理组成员或组不存在"
else
  warn "未发现 sudoers 配置"
fi

# ============================================================
# 4. 关键文件、目录和挂载权限
# ============================================================
section "4. 关键文件、目录和挂载权限"

check_file_perm_owner "/etc/passwd" "0644" "root:root" "/etc/passwd"
check_file_perm_owner "/etc/group" "0644" "root:root" "/etc/group"
check_file_perm_owner "/etc/shadow" "0640" "root:root root:shadow" "/etc/shadow"
check_file_perm_owner "/etc/gshadow" "0640" "root:root root:shadow" "/etc/gshadow"
[ -e /etc/passwd- ] && check_file_perm_owner "/etc/passwd-" "0600" "root:root" "/etc/passwd- 备份文件"
[ -e /etc/shadow- ] && check_file_perm_owner "/etc/shadow-" "0600" "root:root root:shadow" "/etc/shadow- 备份文件"
[ -e /etc/group- ] && check_file_perm_owner "/etc/group-" "0600" "root:root" "/etc/group- 备份文件"
[ -e /etc/gshadow- ] && check_file_perm_owner "/etc/gshadow-" "0600" "root:root root:shadow" "/etc/gshadow- 备份文件"
[ -e /etc/sudoers ] && check_file_perm_owner "/etc/sudoers" "0440" "root:root" "/etc/sudoers"
[ -e "$SSH_CONFIG_FILE" ] && check_file_perm_owner "$SSH_CONFIG_FILE" "0600" "root:root" "SSH 主配置"

for grub_file in /boot/grub/grub.cfg /boot/grub2/grub.cfg /boot/efi/EFI/*/grub.cfg; do
  [ -e "$grub_file" ] && check_file_perm_owner "$grub_file" "0600" "root:root" "GRUB 配置"
done

for key_file in /etc/ssh/ssh_host_*_key; do
  [ -e "$key_file" ] || continue
  case "$key_file" in
    *.pub) check_file_perm_owner "$key_file" "0644" "root:root" "SSH host 公钥" ;;
    *) check_file_perm_owner "$key_file" "0600" "root:root" "SSH host 私钥" ;;
  esac
done

check_dir_perm_owner "/etc/cron.hourly" "0700" "root:root" "cron.hourly"
check_dir_perm_owner "/etc/cron.daily" "0700" "root:root" "cron.daily"
check_dir_perm_owner "/etc/cron.weekly" "0700" "root:root" "cron.weekly"
check_dir_perm_owner "/etc/cron.monthly" "0700" "root:root" "cron.monthly"
check_dir_perm_owner "/etc/cron.d" "0700" "root:root" "cron.d"
[ -e /etc/crontab ] && check_file_perm_owner "/etc/crontab" "0600" "root:root" "/etc/crontab"

for f in /etc/profile /etc/bashrc /etc/bash.bashrc /etc/environment /etc/ld.so.conf; do
  check_file_not_writable_by_group_other "$f" "全局启动/链接配置"
done
for f in /etc/profile.d/*; do
  [ -e "$f" ] && check_file_not_writable_by_group_other "$f" "profile.d 配置"
done

check_mount_option "/tmp" "nodev"
check_mount_option "/tmp" "nosuid"
check_mount_option "/tmp" "noexec"
check_mount_option "/var/tmp" "nodev"
check_mount_option "/var/tmp" "nosuid"
check_mount_option "/var/tmp" "noexec"
check_mount_option "/dev/shm" "nodev"
check_mount_option "/dev/shm" "nosuid"
check_mount_option "/dev/shm" "noexec"

if command -v find >/dev/null 2>&1; then
  WORLD_WRITABLE_NO_STICKY="$(find / -xdev -type d -perm -0002 ! -perm -1000 2>/dev/null | head -20)"
  [ -n "$WORLD_WRITABLE_NO_STICKY" ] && warn "存在未设置 sticky bit 的全局可写目录: $(printf "%s" "$WORLD_WRITABLE_NO_STICKY" | tr '\n' ' ')" || log "未发现缺失 sticky bit 的全局可写目录"

  UNOWNED_FILES="$(find / -xdev \( -nouser -o -nogroup \) -print 2>/dev/null | head -20)"
  [ -n "$UNOWNED_FILES" ] && warn "存在无属主/无属组文件: $(printf "%s" "$UNOWNED_FILES" | tr '\n' ' ')" || log "未发现无属主/无属组文件"

  SUID_FILES="$(find / -xdev -type f \( -perm -4000 -o -perm -2000 \) 2>/dev/null | grep -vE '^/(usr/)?(bin|sbin|lib|lib64|libexec)/' | head -20)"
  [ -n "$SUID_FILES" ] && warn "发现非标准路径的 SUID/SGID 文件（需人工确认）: $(printf "%s" "$SUID_FILES" | tr '\n' ' ')" || log "未发现非标准路径的 SUID/SGID 文件"
else
  warn "find 命令不可用，跳过文件系统深度检查"
fi

# ============================================================
# 5. 内核、系统加固与安全模块
# ============================================================
section "5. 内核、系统加固与安全模块"

ASLR_VALUE="$(sysctl_value /proc/sys/kernel/randomize_va_space)"
if [ -n "$ASLR_VALUE" ]; then
  [ "$ASLR_VALUE" = "2" ] && log "ASLR 已完全启用 (randomize_va_space=2)" || error "ASLR 未完全启用 (randomize_va_space=$ASLR_VALUE，建议=2)"
else
  warn "无法读取 ASLR 状态"
fi
check_sysctl_eq "fs.suid_dumpable" "/proc/sys/fs/suid_dumpable" "0" warn "禁止 SUID 程序生成 core dump"
check_sysctl_min "kernel.dmesg_restrict" "/proc/sys/kernel/dmesg_restrict" "1" warn "限制普通用户读取内核日志"
check_sysctl_min "kernel.kptr_restrict" "/proc/sys/kernel/kptr_restrict" "1" warn "限制内核指针泄漏"
[ -r /proc/sys/kernel/yama/ptrace_scope ] && check_sysctl_min "kernel.yama.ptrace_scope" "/proc/sys/kernel/yama/ptrace_scope" "1" warn "限制 ptrace 调试范围"

CORE_LIMIT="$(ulimit -c 2>/dev/null)"
[ "$CORE_LIMIT" = "0" ] && log "当前 shell core dump 已禁用" || warn "当前 shell core dump 未禁用 (ulimit=$CORE_LIMIT)"

if command -v getenforce >/dev/null 2>&1; then
  SELINUX_STATE="$(getenforce 2>/dev/null)"
  [ "$SELINUX_STATE" = "Enforcing" ] && log "SELinux 处于 Enforcing" || warn "SELinux 当前状态: ${SELINUX_STATE:-未知}（建议 Enforcing）"
elif command -v aa-status >/dev/null 2>&1; then
  if aa-status --enabled >/dev/null 2>&1; then
    log "AppArmor 已启用"
  else
    warn "AppArmor 未启用"
  fi
else
  warn "未检测到 SELinux/AppArmor 状态工具，建议确认强制访问控制是否启用"
fi

if command -v lsmod >/dev/null 2>&1; then
  for mod in cramfs freevxfs jffs2 hfs hfsplus udf usb-storage firewire-core; do
    if lsmod 2>/dev/null | awk '{print $1}' | grep -qx "$mod"; then
      warn "高风险或非常用内核模块已加载: $mod（如非业务需要建议禁用）"
    fi
  done
fi

if command -v aide >/dev/null 2>&1 || command -v tripwire >/dev/null 2>&1 || pgrep -x wazuh-agent >/dev/null 2>&1; then
  log "检测到文件完整性/主机安全组件（AIDE/Tripwire/Wazuh Agent 之一）"
else
  warn "未检测到 AIDE/Tripwire/Wazuh Agent，建议启用文件完整性监控"
fi

# ============================================================
# 6. 网络协议栈与防火墙
# ============================================================
section "6. 网络协议栈与防火墙"

IP_FWD="$(sysctl_value /proc/sys/net/ipv4/ip_forward)"
if [ -n "$IP_FWD" ]; then
  [ "$IP_FWD" = "0" ] && log "IP 转发已关闭" || error "IP 转发已开启（本机可能被用作路由跳板）"
else
  warn "net.ipv4.ip_forward 无法读取，跳过"
fi
[ -r /proc/sys/net/ipv6/conf/all/forwarding ] && check_sysctl_eq "net.ipv6.conf.all.forwarding" "/proc/sys/net/ipv6/conf/all/forwarding" "0" error "禁止非路由主机转发 IPv6"
SYN_COOKIE="$(sysctl_value /proc/sys/net/ipv4/tcp_syncookies)"
if [ -n "$SYN_COOKIE" ]; then
  [ "$SYN_COOKIE" = "1" ] && log "TCP SYN Cookie 已开启" || warn "TCP SYN Cookie 未开启（可能遭受 SYN Flood）"
else
  warn "net.ipv4.tcp_syncookies 无法读取，跳过"
fi
check_sysctl_eq "net.ipv4.icmp_echo_ignore_broadcasts" "/proc/sys/net/ipv4/icmp_echo_ignore_broadcasts" "1" warn "忽略广播 ICMP"
check_sysctl_eq "net.ipv4.icmp_ignore_bogus_error_responses" "/proc/sys/net/ipv4/icmp_ignore_bogus_error_responses" "1" warn "忽略伪造 ICMP 错误"

for scope in all default; do
  check_sysctl_eq "net.ipv4.conf.$scope.accept_redirects" "/proc/sys/net/ipv4/conf/$scope/accept_redirects" "0" error "防止 ICMP 重定向攻击"
  check_sysctl_eq "net.ipv4.conf.$scope.secure_redirects" "/proc/sys/net/ipv4/conf/$scope/secure_redirects" "0" warn "禁止安全重定向信任网关列表"
  check_sysctl_eq "net.ipv4.conf.$scope.send_redirects" "/proc/sys/net/ipv4/conf/$scope/send_redirects" "0" warn "禁止发送 ICMP 重定向"
  check_sysctl_eq "net.ipv4.conf.$scope.accept_source_route" "/proc/sys/net/ipv4/conf/$scope/accept_source_route" "0" error "禁止源路由"
  check_sysctl_min "net.ipv4.conf.$scope.rp_filter" "/proc/sys/net/ipv4/conf/$scope/rp_filter" "1" warn "启用反向路径过滤"
  check_sysctl_eq "net.ipv4.conf.$scope.log_martians" "/proc/sys/net/ipv4/conf/$scope/log_martians" "1" warn "记录异常源地址包"

  [ -r "/proc/sys/net/ipv6/conf/$scope/accept_redirects" ] && check_sysctl_eq "net.ipv6.conf.$scope.accept_redirects" "/proc/sys/net/ipv6/conf/$scope/accept_redirects" "0" error "防止 IPv6 重定向攻击"
  [ -r "/proc/sys/net/ipv6/conf/$scope/accept_source_route" ] && check_sysctl_eq "net.ipv6.conf.$scope.accept_source_route" "/proc/sys/net/ipv6/conf/$scope/accept_source_route" "0" error "禁止 IPv6 源路由"
done

SRC_RT_ALL="$(sysctl_value /proc/sys/net/ipv4/conf/all/accept_source_route)"
if [ -n "$SRC_RT_ALL" ]; then
  [ "$SRC_RT_ALL" = "0" ] && log "源路由已关闭" || error "源路由已开启（可能被用于 IP 欺骗）"
fi

FIREWALL_FOUND=0
if service_active firewalld || pgrep -x firewalld >/dev/null 2>&1; then
  FIREWALL_FOUND=1
  log "firewalld 正在运行"
fi
if service_active ufw || pgrep -x ufw >/dev/null 2>&1; then
  FIREWALL_FOUND=1
  log "ufw 正在运行"
fi
if service_active nftables || { command -v nft >/dev/null 2>&1 && nft list ruleset 2>/dev/null | grep -q 'table'; }; then
  FIREWALL_FOUND=1
  log "nftables 规则已配置"
fi
if command -v iptables >/dev/null 2>&1 && iptables -S 2>/dev/null | grep -q '^-P'; then
  FIREWALL_FOUND=1
  IPT_INPUT_POLICY="$(iptables -S INPUT 2>/dev/null | awk '/^-P INPUT/ {print $3; exit}')"
  [ "$IPT_INPUT_POLICY" = "DROP" ] || [ "$IPT_INPUT_POLICY" = "REJECT" ] && log "iptables INPUT 默认策略: $IPT_INPUT_POLICY" || warn "iptables INPUT 默认策略: ${IPT_INPUT_POLICY:-未知}（建议默认拒绝并按需放行）"
fi
[ "$FIREWALL_FOUND" = "0" ] && warn "未检测到有效防火墙服务或规则（firewalld/ufw/nftables/iptables）"

LISTEN_OUTPUT=""
if command -v ss >/dev/null 2>&1; then
  LISTEN_OUTPUT="$(ss -tulnH 2>/dev/null)"
elif command -v netstat >/dev/null 2>&1; then
  LISTEN_OUTPUT="$(netstat -tuln 2>/dev/null | awk 'NR>2')"
fi
if [ -n "$LISTEN_OUTPUT" ]; then
  INSECURE_PORTS="$(printf "%s\n" "$LISTEN_OUTPUT" | grep -E '(:|\.)((21|23|69|512|513|514))([[:space:]]|$)' | head -10)"
  [ -n "$INSECURE_PORTS" ] && warn "监听了明文/高风险传统服务端口（需确认业务必要性）: $(printf "%s" "$INSECURE_PORTS" | tr '\n' '; ')" || log "未发现 21/23/69/512-514 等高风险端口监听"
else
  warn "未找到 ss/netstat，跳过监听端口检查"
fi

# ============================================================
# 7. 审计、日志与时间同步
# ============================================================
section "7. 审计、日志与时间同步"

LOGD_RUNNING=0
pgrep -x "rsyslogd" >/dev/null 2>&1 && { LOGD_RUNNING=1; log "rsyslogd 正在运行"; }
pgrep -x "syslog-ng" >/dev/null 2>&1 && { LOGD_RUNNING=1; log "syslog-ng 正在运行"; }
pgrep -x "systemd-journald" >/dev/null 2>&1 && { LOGD_RUNNING=1; log "systemd-journald 正在运行"; }
[ "$LOGD_RUNNING" = "0" ] && warn "未检测到系统日志服务（rsyslogd/syslog-ng/journald）"

if pgrep -x "auditd" >/dev/null 2>&1; then
  log "auditd 正在运行"
else
  warn "auditd 未运行（建议开启审计记录）"
fi

if command -v auditctl >/dev/null 2>&1; then
  AUDIT_ENABLED="$(auditctl -s 2>/dev/null | awk '/enabled/ {print $2; exit}')"
  [ "$AUDIT_ENABLED" = "1" ] || [ "$AUDIT_ENABLED" = "2" ] && log "auditd 内核审计 enabled=$AUDIT_ENABLED" || warn "auditd 内核审计未启用或无法确认 enabled=${AUDIT_ENABLED:-未知}"
fi

AUDIT_RULES="$(cat /etc/audit/audit.rules /etc/audit/rules.d/*.rules 2>/dev/null)"
if [ -n "$AUDIT_RULES" ]; then
  printf "%s\n" "$AUDIT_RULES" | grep -Eq '(-w[[:space:]]+/etc/passwd|-a[[:space:]].*path=/etc/passwd)' && log "审计规则覆盖 /etc/passwd" || warn "审计规则未覆盖 /etc/passwd"
  printf "%s\n" "$AUDIT_RULES" | grep -Eq '(-w[[:space:]]+/etc/shadow|-a[[:space:]].*path=/etc/shadow)' && log "审计规则覆盖 /etc/shadow" || warn "审计规则未覆盖 /etc/shadow"
  printf "%s\n" "$AUDIT_RULES" | grep -Eq '(-w[[:space:]]+/etc/sudoers|-a[[:space:]].*path=/etc/sudoers)' && log "审计规则覆盖 sudoers" || warn "审计规则未覆盖 sudoers"
  printf "%s\n" "$AUDIT_RULES" | grep -Eq '(-S[[:space:]]+unlink|-S[[:space:]]+rename|-S[[:space:]]+chmod|-S[[:space:]]+chown)' && log "审计规则覆盖关键文件变更/删除系统调用" || warn "审计规则未覆盖关键文件变更/删除系统调用"
else
  warn "未找到 audit 规则文件或规则为空"
fi

if [ -f /var/log/secure ]; then
  log "/var/log/secure 存在"
elif [ -f /var/log/auth.log ]; then
  log "/var/log/auth.log 存在"
else
  warn "未找到安全日志文件（/var/log/secure 或 /var/log/auth.log）"
fi

if [ -d /var/log/journal ] || grep -Ehs '^[[:space:]]*Storage[[:space:]]*=[[:space:]]*persistent' /etc/systemd/journald.conf /etc/systemd/journald.conf.d/*.conf >/dev/null 2>&1; then
  log "systemd-journald 已启用或存在持久化日志目录"
else
  warn "systemd-journald 未确认持久化存储，建议启用 Storage=persistent"
fi

[ -f /etc/logrotate.conf ] || [ -d /etc/logrotate.d ] && log "logrotate 配置存在" || warn "未发现 logrotate 配置，日志可能无限增长或保留不足"

if pgrep -x chronyd >/dev/null 2>&1 || pgrep -x ntpd >/dev/null 2>&1 || pgrep -x systemd-timesyncd >/dev/null 2>&1; then
  log "时间同步服务正在运行（chronyd/ntpd/systemd-timesyncd）"
else
  warn "未检测到时间同步服务，审计时间线可能不可信"
fi

# ============================================================
# 8. 服务暴露、持久化与环境风险
# ============================================================
section "8. 服务暴露、持久化与环境风险"

for proc in telnetd in.telnetd rshd in.rshd rexecd in.rexecd rlogind in.rlogind tftpd in.tftpd xinetd; do
  if pgrep -x "$proc" >/dev/null 2>&1; then
    warn "高风险传统服务进程正在运行: $proc"
  fi
done

if [ -r /etc/vsftpd/vsftpd.conf ] || [ -r /etc/vsftpd.conf ]; then
  FTP_ANON="$(grep -Ehs '^[[:space:]]*anonymous_enable[[:space:]]*=[[:space:]]*YES' /etc/vsftpd/vsftpd.conf /etc/vsftpd.conf)"
  [ -n "$FTP_ANON" ] && warn "FTP 匿名访问已启用" || log "FTP 匿名访问未启用"
fi

if [ -s /etc/ld.so.preload ]; then
  error "/etc/ld.so.preload 非空，可能存在动态链接劫持风险"
else
  [ -e /etc/ld.so.preload ] && log "/etc/ld.so.preload 为空" || log "/etc/ld.so.preload 不存在"
fi

PATH_RISK=0
OLD_IFS="$IFS"
IFS=':'
for p in $PATH; do
  if [ -z "$p" ] || [ "$p" = "." ]; then
    warn "PATH 包含空路径或当前目录，存在命令劫持风险"
    PATH_RISK=1
    continue
  fi
  if [ -d "$p" ]; then
    p_perm="$(get_perm "$p")"
    if [ -n "$p_perm" ] && [ $((8#$p_perm & 8#022)) -ne 0 ] 2>/dev/null; then
      warn "PATH 目录可被 group/other 写入: $p ($p_perm)"
      PATH_RISK=1
    fi
  fi
done
IFS="$OLD_IFS"
[ "$PATH_RISK" = "0" ] && log "PATH 未发现明显命令劫持风险"

if command -v find >/dev/null 2>&1; then
  CRON_WRITABLE="$(find /etc/cron* -type f -perm /022 2>/dev/null | head -20)"
  [ -n "$CRON_WRITABLE" ] && warn "存在 group/other 可写的计划任务文件: $(printf "%s" "$CRON_WRITABLE" | tr '\n' ' ')" || log "计划任务文件未发现 group/other 写权限"

  AUTH_KEYS="$(find /root /home -maxdepth 3 -type f -name authorized_keys 2>/dev/null | head -50)"
  if [ -n "$AUTH_KEYS" ]; then
    printf "%s\n" "$AUTH_KEYS" | while read -r key_file; do
      [ -n "$key_file" ] || continue
      key_perm="$(get_perm "$key_file")"
      if perm_no_more_permissive "$key_perm" "0600"; then
        log "authorized_keys 权限正常 ($key_file: $key_perm)"
      else
        warn "authorized_keys 权限过宽 ($key_file: $key_perm，建议不高于 0600)"
      fi
    done
  else
    log "未发现 authorized_keys 文件"
  fi
fi

for rc_file in /etc/rc.local /etc/rc.d/rc.local; do
  [ -e "$rc_file" ] || continue
  if [ -x "$rc_file" ]; then
    warn "$rc_file 可执行，需确认是否存在非预期启动项"
  else
    log "$rc_file 存在但不可执行"
  fi
done

echo "=========================================="
echo "      安全基线检查完成"
echo "=========================================="
