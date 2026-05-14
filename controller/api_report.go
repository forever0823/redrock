package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

/*
 * summarizeResults 统计巡检结果中的 INFO/WARNING/ERROR 数量
 *
 * Go 语法要点：
 *   - strings.Count(s, sub)  统计子串出现次数
 *   - for _, item := range slice  用 _ 忽略索引
 */
func summarizeResults(results []map[string]interface{}) map[string]int {
	summary := map[string]int{"info": 0, "warning": 0, "error": 0}
	for _, item := range results {
		out := coalesce(item["output"], item["error"], "")
		summary["info"] += strings.Count(out, "[INFO]")
		summary["warning"] += strings.Count(out, "[WARNING]")
		summary["error"] += strings.Count(out, "[ERROR]")
	}
	return summary
}

/*
 * extractDetailInfo 从巡检结果中提取详情信息
 *
 * 返回三组数据（system/security/process），每组包含：
 *   - warning_lines: WARNING 行（最多 50 条）
 *   - error_lines: ERROR 行（最多 50 条）
 *   - result_lines: 有效结果行，已过滤掉运行日志噪声（最多 500 条）
 *
 * Go 语法要点：
 *   - []*regexp.Regexp  正则指针切片
 *   - var warnings, errors, resultLines []string  多变量声明
 *   - strings.Split(s, sep)  分割字符串
 *   - append(slice, elem)  追加元素到切片
 *   - slice[:50]  切片截取前 50 个元素
 */
func extractDetailInfo(results []map[string]interface{}) map[string]interface{} {
	details := map[string]interface{}{}
	// 噪声匹配规则：这些行属于脚本运行日志，对用户无意义，需要过滤
	noisePatterns := []*regexp.Regexp{
		regexp.MustCompile(`^\[INFO\]\s*主机系统类型:`),
		regexp.MustCompile(`^\[WARNING\]\s*命令缺失:`),
		regexp.MustCompile(`^\[INFO\]\s*命令安装成功:`),
		regexp.MustCompile(`^\[ERROR\]\s*命令安装失败:`),
	}

	for _, item := range results {
		script, _ := item["script"].(string)
		output := fixMojibake(coalesce(item["output"], item["error"], ""))

		var warnings, errors, resultLines []string
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// 分类收集 WARNING 和 ERROR 行
			if strings.Contains(line, "[WARNING]") {
				warnings = append(warnings, line)
			}
			if strings.Contains(line, "[ERROR]") {
				errors = append(errors, line)
			}
			// 判断是否为噪声行
			noisy := false
			for _, pat := range noisePatterns {
				if pat.MatchString(line) {
					noisy = true
					break
				}
			}
			if !noisy {
				resultLines = append(resultLines, line)
			}
		}
		// 限制数量，避免数据过大
		if len(warnings) > 50 {
			warnings = warnings[:50]
		}
		if len(errors) > 50 {
			errors = errors[:50]
		}
		if len(resultLines) > 500 {
			resultLines = resultLines[:500]
		}

		status, _ := item["status"].(string)
		dur, _ := item["duration_sec"].(float64)
		info := map[string]interface{}{
			"script":        script,
			"status":        status,
			"duration_sec":  dur,
			"warning_lines": warnings,
			"error_lines":   errors,
			"result_lines":  resultLines,
		}

		// 根据脚本名归类到对应指标类别
		switch script {
		case "system_check.sh":
			details["system"] = info
		case "security_baseline_check.sh":
			details["security"] = info
		case "process_check.sh":
			details["process"] = info
		}
	}
	return details
}

func containsAnyText(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func countBoolRisks(values ...bool) int {
	count := 0
	for _, v := range values {
		if v {
			count++
		}
	}
	return count
}

func lineContainsAll(output string, parts ...string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matched := true
		for _, part := range parts {
			if !strings.Contains(line, part) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func collectRegexLines(output string, re *regexp.Regexp, limit int) []string {
	var lines []string
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !re.MatchString(line) || seen[line] {
			continue
		}
		seen[line] = true
		lines = append(lines, line)
		if limit > 0 && len(lines) >= limit {
			break
		}
	}
	return lines
}

/*
 * extractStructuredMetrics 从巡检输出中提取结构化指标
 *
 * Go 正则要点：
 *   - regexp.MustCompile  编译正则表达式（失败则 panic）
 *   - .FindStringSubmatch(s)  返回匹配和捕获组，格式 [完整匹配, 组1, 组2, ...]
 *   - .FindAllString(s, -1)  返回所有匹配（-1 表示不限数量）
 *
 * 三类指标：
 *   1. system:   CPU 使用率、内存使用率、磁盘告警数、系统负载
 *   2. security: WARNING/ERROR 计数、空密码、多 UID0、SSH 风险、权限异常
 *   3. process:  WARNING/ERROR 计数、僵尸/假死/死锁标记、检查进程列表、PID 数
 */
func extractStructuredMetrics(results []map[string]interface{}) map[string]interface{} {
	metrics := map[string]interface{}{}
	sysMetrics := map[string]interface{}{}
	secMetrics := map[string]interface{}{}
	procMetrics := map[string]interface{}{}

	for _, item := range results {
		script, _ := item["script"].(string)
		output := fixMojibake(coalesce(item["output"], item["error"], ""))

		switch script {
		case "system_check.sh":
			// 用正则从文本中提取数值指标
			reCPU := regexp.MustCompile(`CPU使用率:\s*([0-9.]+)%`)
			reMEM := regexp.MustCompile(`内存使用率:\s*([0-9.]+)%`)
			reDisk := regexp.MustCompile(`磁盘分区 .* 超过阈值|磁盘分区 .* 使用率.*超过阈值`)
			reLoad := regexp.MustCompile(`系统负载:\s*1分钟=([0-9.]+),\s*5分钟=([0-9.]+),\s*15分钟=([0-9.]+)`)

			// if m := re.FindStringSubmatch(s); m != nil  是 Go 中"if-赋值-判断"的常用模式
			if m := reCPU.FindStringSubmatch(output); m != nil {
				sysMetrics["cpu_usage_percent"] = parseFloat(m[1]) // m[1] 是第一个捕获组
			}
			if m := reMEM.FindStringSubmatch(output); m != nil {
				sysMetrics["mem_usage_percent"] = parseFloat(m[1])
			}
			sysMetrics["disk_warning_count"] = len(reDisk.FindAllString(output, -1))
			if m := reLoad.FindStringSubmatch(output); m != nil {
				sysMetrics["load_1"] = parseFloat(m[1])
				sysMetrics["load_5"] = parseFloat(m[2])
				sysMetrics["load_15"] = parseFloat(m[3])
			}
			metrics["system"] = sysMetrics

		case "security_baseline_check.sh":
			secMetrics["warning_count"] = strings.Count(output, "[WARNING]")
			secMetrics["error_count"] = strings.Count(output, "[ERROR]")

			// 账户安全
			secMetrics["has_empty_password_user"] = strings.Contains(output, "存在空密码账户")
			secMetrics["has_passwd_empty_field"] = strings.Contains(output, "/etc/passwd 存在空密码字段账户")
			secMetrics["has_multi_uid0"] = containsAnyText(output, "存在多个UID=0账户", "存在多个 UID=0 账户")
			secMetrics["has_sys_login_users"] = strings.Contains(output, "存在可登录的系统账户")
			secMetrics["root_locked"] = strings.Contains(output, "root 账户已锁定")
			secMetrics["has_dup_uid"] = containsAnyText(output, "存在重复UID", "存在重复 UID")
			secMetrics["has_dup_gid"] = strings.Contains(output, "存在重复 GID")
			secMetrics["has_dup_user"] = strings.Contains(output, "存在重复用户名")
			secMetrics["has_dup_group"] = strings.Contains(output, "存在重复用户组名")
			secMetrics["password_aging_risk"] = strings.Contains(output, "密码最长使用期超过")
			secMetrics["password_warn_age_risk"] = strings.Contains(output, "密码到期提醒不足")
			secMetrics["home_dir_issue"] = containsAnyText(output, "家目录不存在", "家目录属主异常", "家目录允许 other 写入")

			// SSH 安全
			secMetrics["root_login_risk"] = strings.Contains(output, "PermitRootLogin=yes") || strings.Contains(output, "SSH允许root远程登录")
			secMetrics["password_login_risk"] = strings.Contains(output, "PasswordAuthentication=yes") || strings.Contains(output, "SSH允许密码登录")
			secMetrics["ssh_empty_password"] = strings.Contains(output, "允许空密码登录")
			secMetrics["ssh_pubkey_disabled"] = strings.Contains(output, "建议启用公钥认证")
			secMetrics["ssh_hostbased_risk"] = strings.Contains(output, "禁止基于主机信任认证")
			secMetrics["ssh_ignore_rhosts_off"] = strings.Contains(output, "忽略 rhosts 信任文件")
			secMetrics["ssh_x11_forward"] = strings.Contains(output, "建议关闭 X11 转发")
			secMetrics["ssh_tcp_forward"] = strings.Contains(output, "允许 TCP 端口转发")
			secMetrics["ssh_agent_forward"] = strings.Contains(output, "关闭 SSH agent 转发")
			secMetrics["ssh_gateway_ports"] = strings.Contains(output, "禁止远程主机绑定转发端口")
			secMetrics["ssh_tunnel"] = strings.Contains(output, "禁止 SSH 隧道设备")
			secMetrics["ssh_user_env_risk"] = strings.Contains(output, "允许用户环境变量")
			secMetrics["ssh_pam_off"] = strings.Contains(output, "关闭 PAM")
			secMetrics["ssh_max_auth_high"] = strings.Contains(output, "MaxAuthTries") && strings.Contains(output, "建议不超过")
			secMetrics["ssh_login_grace_high"] = strings.Contains(output, "LoginGraceTime") && strings.Contains(output, "建议不超过")
			secMetrics["ssh_client_alive_unsafe"] = containsAnyText(output, "ClientAliveInterval 未配置", "ClientAliveInterval=") && strings.Contains(output, "避免长期空闲会话")
			secMetrics["ssh_max_sessions_high"] = strings.Contains(output, "MaxSessions") && strings.Contains(output, "建议不超过")
			secMetrics["ssh_keyboard_interactive_risk"] = strings.Contains(output, "KbdInteractiveAuthentication/ChallengeResponseAuthentication=yes")
			secMetrics["ssh_access_control_missing"] = strings.Contains(output, "SSH 未配置 AllowUsers/AllowGroups/DenyUsers/DenyGroups")
			secMetrics["ssh_weak_algorithms"] = strings.Contains(output, "包含弱算法")

			// 密码、PAM 与 sudo
			secMetrics["pass_max_days_risk"] = lineContainsAll(output, "PASS_MAX_DAYS=", "建议 <=")
			secMetrics["pass_min_days_risk"] = lineContainsAll(output, "PASS_MIN_DAYS=", "建议 >= 1")
			secMetrics["pass_warn_age_risk"] = lineContainsAll(output, "PASS_WARN_AGE=", "建议 >= 7")
			secMetrics["pass_min_len_risk"] = lineContainsAll(output, "PASS_MIN_LEN=", "建议 >=")
			secMetrics["weak_encrypt_method"] = lineContainsAll(output, "ENCRYPT_METHOD=", "建议 yescrypt 或 SHA512")
			secMetrics["weak_umask"] = lineContainsAll(output, "UMASK=", "建议 027 或 077")
			secMetrics["pam_complexity_missing"] = strings.Contains(output, "PAM 未发现密码复杂度模块")
			secMetrics["pam_lockout_missing"] = strings.Contains(output, "PAM 未发现登录失败锁定模块")
			secMetrics["pam_history_missing"] = strings.Contains(output, "PAM 未发现密码历史限制")
			secMetrics["pwquality_weak"] = lineContainsAll(output, "pwquality minlen=", "建议 >= 12") ||
				lineContainsAll(output, "pwquality minclass=", "建议 >= 3") ||
				strings.Contains(output, "pwquality 字符类别限制不足")
			secMetrics["faillock_weak"] = lineContainsAll(output, "登录失败锁定 deny=", "建议 <= 5")
			secMetrics["sudo_nopasswd"] = strings.Contains(output, "sudo 存在免密授权")
			secMetrics["sudo_use_pty_missing"] = strings.Contains(output, "sudo 未启用 Defaults use_pty")

			// 文件权限
			rePerm := regexp.MustCompile(`\[ERROR\]\s*.*权限异常[^\n]*`)
			permIssues := collectRegexLines(output, rePerm, 10)
			secMetrics["permission_issue_count"] = len(permIssues)
			secMetrics["permission_issues"] = permIssues
			ownerIssues := collectRegexLines(output, regexp.MustCompile(`\[WARNING\]\s*.*属主属组异常[^\n]*`), 10)
			secMetrics["owner_issue_count"] = len(ownerIssues)
			secMetrics["owner_issues"] = ownerIssues
			secMetrics["mount_option_missing_count"] = len(collectRegexLines(output, regexp.MustCompile(`\[WARNING\].*未启用 (nodev|nosuid|noexec)`), 0))
			secMetrics["world_writable_no_sticky"] = strings.Contains(output, "存在未设置 sticky bit 的全局可写目录")
			secMetrics["unowned_files"] = strings.Contains(output, "存在无属主/无属组文件")
			secMetrics["cron_writable"] = strings.Contains(output, "存在 group/other 可写的计划任务文件")
			secMetrics["authorized_keys_perm_weak"] = strings.Contains(output, "authorized_keys 权限过宽")
			secMetrics["ld_preload_risk"] = strings.Contains(output, "/etc/ld.so.preload 非空")
			secMetrics["path_hijack_risk"] = containsAnyText(output, "PATH 包含空路径", "PATH 目录可被 group/other 写入")

			// 系统加固
			secMetrics["aslr_enabled"] = strings.Contains(output, "ASLR 已完全启用")
			secMetrics["core_dump_disabled"] = strings.Contains(output, "core dump 已禁用")
			secMetrics["has_suid_risk"] = strings.Contains(output, "非标准路径的 SUID")
			secMetrics["fs_suid_dumpable_risk"] = lineContainsAll(output, "fs.suid_dumpable=", "禁止 SUID 程序生成 core dump")
			secMetrics["dmesg_restrict_weak"] = lineContainsAll(output, "kernel.dmesg_restrict=", "限制普通用户读取内核日志")
			secMetrics["kptr_restrict_weak"] = lineContainsAll(output, "kernel.kptr_restrict=", "限制内核指针泄漏")
			secMetrics["ptrace_scope_weak"] = lineContainsAll(output, "kernel.yama.ptrace_scope=", "限制 ptrace 调试范围")
			secMetrics["mac_missing_or_weak"] = containsAnyText(output, "未检测到 SELinux/AppArmor", "SELinux 当前状态:", "AppArmor 未启用")
			secMetrics["high_risk_module_loaded"] = strings.Contains(output, "高风险或非常用内核模块已加载")
			secMetrics["fim_missing"] = strings.Contains(output, "未检测到 AIDE/Tripwire/Wazuh Agent")

			// 网络加固
			secMetrics["ip_forward_enabled"] = strings.Contains(output, "IP 转发已开启")
			secMetrics["ipv6_forward_enabled"] = lineContainsAll(output, "net.ipv6.conf.all.forwarding=", "禁止非路由主机转发 IPv6")
			secMetrics["syn_cookie_on"] = strings.Contains(output, "TCP SYN Cookie 已开启")
			secMetrics["icmp_redirect_risk"] = strings.Contains(output, "ICMP 重定向攻击")
			secMetrics["source_route_enabled"] = strings.Contains(output, "源路由已开启")
			secMetrics["ipv6_redirect_risk"] = strings.Contains(output, "防止 IPv6 重定向攻击")
			secMetrics["rp_filter_weak"] = lineContainsAll(output, "rp_filter", "启用反向路径过滤")
			secMetrics["log_martians_off"] = lineContainsAll(output, "log_martians", "记录异常源地址包")
			secMetrics["firewall_missing"] = strings.Contains(output, "未检测到有效防火墙服务或规则")
			secMetrics["iptables_input_open"] = lineContainsAll(output, "iptables INPUT 默认策略", "建议默认拒绝")
			secMetrics["insecure_ports_listening"] = strings.Contains(output, "监听了明文/高风险传统服务端口")

			// 审计日志
			secMetrics["log_service_running"] = strings.Contains(output, "rsyslogd") && strings.Contains(output, "正在运行") ||
				strings.Contains(output, "syslog-ng") && strings.Contains(output, "正在运行") ||
				strings.Contains(output, "journald") && strings.Contains(output, "正在运行")
			secMetrics["auditd_running"] = strings.Contains(output, "auditd 正在运行")
			secMetrics["audit_kernel_enabled"] = strings.Contains(output, "auditd 内核审计 enabled=")
			secMetrics["audit_rules_missing"] = containsAnyText(output, "审计规则未覆盖", "未找到 audit 规则文件或规则为空")
			secMetrics["secure_log_exists"] = strings.Contains(output, "/var/log/secure 存在") || strings.Contains(output, "/var/log/auth.log 存在")
			secMetrics["journald_persistent_missing"] = strings.Contains(output, "systemd-journald 未确认持久化存储")
			secMetrics["logrotate_missing"] = strings.Contains(output, "未发现 logrotate 配置")
			secMetrics["time_sync_missing"] = strings.Contains(output, "未检测到时间同步服务")

			// 服务暴露与持久化
			secMetrics["risky_legacy_service_running"] = strings.Contains(output, "高风险传统服务进程正在运行")
			secMetrics["ftp_anonymous_enabled"] = strings.Contains(output, "FTP 匿名访问已启用")
			secMetrics["rc_local_executable"] = strings.Contains(output, "可执行，需确认是否存在非预期启动项")
			secMetrics["security_risk_count"] = countBoolRisks(
				secMetrics["has_empty_password_user"].(bool),
				secMetrics["has_passwd_empty_field"].(bool),
				secMetrics["has_multi_uid0"].(bool),
				secMetrics["has_sys_login_users"].(bool),
				secMetrics["has_dup_uid"].(bool),
				secMetrics["has_dup_gid"].(bool),
				secMetrics["has_dup_user"].(bool),
				secMetrics["has_dup_group"].(bool),
				secMetrics["password_aging_risk"].(bool),
				secMetrics["password_warn_age_risk"].(bool),
				secMetrics["home_dir_issue"].(bool),
				secMetrics["root_login_risk"].(bool),
				secMetrics["password_login_risk"].(bool),
				secMetrics["ssh_empty_password"].(bool),
				secMetrics["ssh_pubkey_disabled"].(bool),
				secMetrics["ssh_hostbased_risk"].(bool),
				secMetrics["ssh_ignore_rhosts_off"].(bool),
				secMetrics["ssh_x11_forward"].(bool),
				secMetrics["ssh_tcp_forward"].(bool),
				secMetrics["ssh_agent_forward"].(bool),
				secMetrics["ssh_gateway_ports"].(bool),
				secMetrics["ssh_tunnel"].(bool),
				secMetrics["ssh_user_env_risk"].(bool),
				secMetrics["ssh_pam_off"].(bool),
				secMetrics["ssh_max_auth_high"].(bool),
				secMetrics["ssh_login_grace_high"].(bool),
				secMetrics["ssh_client_alive_unsafe"].(bool),
				secMetrics["ssh_max_sessions_high"].(bool),
				secMetrics["ssh_keyboard_interactive_risk"].(bool),
				secMetrics["ssh_access_control_missing"].(bool),
				secMetrics["ssh_weak_algorithms"].(bool),
				secMetrics["pass_max_days_risk"].(bool),
				secMetrics["pass_min_days_risk"].(bool),
				secMetrics["pass_warn_age_risk"].(bool),
				secMetrics["pass_min_len_risk"].(bool),
				secMetrics["weak_encrypt_method"].(bool),
				secMetrics["weak_umask"].(bool),
				secMetrics["pam_complexity_missing"].(bool),
				secMetrics["pam_lockout_missing"].(bool),
				secMetrics["pam_history_missing"].(bool),
				secMetrics["pwquality_weak"].(bool),
				secMetrics["faillock_weak"].(bool),
				secMetrics["sudo_nopasswd"].(bool),
				secMetrics["sudo_use_pty_missing"].(bool),
				secMetrics["world_writable_no_sticky"].(bool),
				secMetrics["unowned_files"].(bool),
				secMetrics["cron_writable"].(bool),
				secMetrics["authorized_keys_perm_weak"].(bool),
				secMetrics["ld_preload_risk"].(bool),
				secMetrics["path_hijack_risk"].(bool),
				!secMetrics["aslr_enabled"].(bool),
				!secMetrics["core_dump_disabled"].(bool),
				secMetrics["has_suid_risk"].(bool),
				secMetrics["fs_suid_dumpable_risk"].(bool),
				secMetrics["dmesg_restrict_weak"].(bool),
				secMetrics["kptr_restrict_weak"].(bool),
				secMetrics["ptrace_scope_weak"].(bool),
				secMetrics["mac_missing_or_weak"].(bool),
				secMetrics["high_risk_module_loaded"].(bool),
				secMetrics["fim_missing"].(bool),
				secMetrics["ip_forward_enabled"].(bool),
				secMetrics["ipv6_forward_enabled"].(bool),
				!secMetrics["syn_cookie_on"].(bool),
				secMetrics["icmp_redirect_risk"].(bool),
				secMetrics["source_route_enabled"].(bool),
				secMetrics["ipv6_redirect_risk"].(bool),
				secMetrics["rp_filter_weak"].(bool),
				secMetrics["log_martians_off"].(bool),
				secMetrics["firewall_missing"].(bool),
				secMetrics["iptables_input_open"].(bool),
				secMetrics["insecure_ports_listening"].(bool),
				!secMetrics["log_service_running"].(bool),
				!secMetrics["auditd_running"].(bool),
				secMetrics["audit_rules_missing"].(bool),
				!secMetrics["secure_log_exists"].(bool),
				secMetrics["journald_persistent_missing"].(bool),
				secMetrics["logrotate_missing"].(bool),
				secMetrics["time_sync_missing"].(bool),
				secMetrics["risky_legacy_service_running"].(bool),
				secMetrics["ftp_anonymous_enabled"].(bool),
				secMetrics["rc_local_executable"].(bool),
			)

			metrics["security"] = secMetrics

		case "process_check.sh":
			procMetrics["warning_count"] = strings.Count(output, "[WARNING]")
			procMetrics["error_count"] = strings.Count(output, "[ERROR]")
			procMetrics["zombie_detected"] = strings.Contains(output, "僵尸进程（Zombie）")
			// || 逻辑或
			procMetrics["hang_risk"] = strings.Contains(output, "可能假死") || strings.Contains(output, "疑似死锁")

			reChecked := regexp.MustCompile(`进程健康检查:\s*(.+)`)
			rePID := regexp.MustCompile(`检查 PID:\s*(\d+)`)
			chkMatches := reChecked.FindAllStringSubmatch(output, -1)
			var checked []string
			// map[string]bool{} 用作集合（set），Go 没有内置 Set 类型
			seen := map[string]bool{}
			for _, m := range chkMatches {
				if len(m) > 1 {
					s := strings.TrimSpace(m[1])
					if !seen[s] {
						seen[s] = true
						checked = append(checked, s)
					}
				}
			}
			if len(checked) > 10 {
				checked = checked[:10]
			}
			procMetrics["checked_processes"] = checked
			procMetrics["checked_pid_count"] = len(rePID.FindAllString(output, -1))
			metrics["process"] = procMetrics
		}
	}
	return metrics
}

/*
 * archiveReport 将巡检原始输出归档到文件
 *
 * 文件命名格式：YYYYmmdd_HHMMSS_<run_id>_<script>.log
 *
 * Go 语法要点：
 *   - time.Now().Format("20060102_150405")  时间格式化
 *     Go 的时间格式参考 2006-01-02 15:04:05 这个固定时刻（Go 诞生时间）
 *     2006→年 01→月 02→日 15→时(24h) 04→分 05→秒
 *   - filepath.Join  跨平台路径拼接（/ 或 \）
 *   - os.MkdirAll  递归创建目录
 *   - strings.TrimSuffix(s, ".sh")  去掉后缀
 */
func (s *Server) archiveReport(agentID string, report map[string]interface{}) []string {
	var saved []string
	ts := time.Now().Format("20060102_150405") // 例如 "20260427_214129"
	runID := ""
	if v, ok := report["run_id"].(string); ok {
		runID = v
	}
	if runID == "" {
		runID = "no_run_id"
	}
	baseDir := filepath.Join(s.cfg.ReportArchiveDir, agentID)
	os.MkdirAll(baseDir, 0755)

	results, _ := report["results"].([]map[string]interface{})
	for _, rm := range results {
		script := ""
		if v, ok := rm["script"].(string); ok {
			script = strings.TrimSuffix(v, ".sh")
		}
		if script == "" {
			script = "unknown"
		}
		// fmt.Sprintf 格式化字符串
		fname := fmt.Sprintf("%s_%s_%s.log", ts, runID, script)
		fpath := filepath.Join(baseDir, fname)
		text := fixMojibake(coalesce(rm["output"], rm["error"], ""))
		os.WriteFile(fpath, []byte(text), 0644) // []byte(text) 字符串转字节切片
		saved = append(saved, fpath)
	}
	return saved
}

// POST /report → 处理巡检报告
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSON(r)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	agentID, _ := payload["agent_id"].(string)
	// 类型断言 []interface{}：JSON 中的数组反序列化为 []interface{}
	resultsRaw, ok := payload["results"].([]interface{})
	if agentID == "" || !ok {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id_and_results_required"})
		return
	}
	// 将每个结果元素转为 map[string]interface{}
	var typedResults []map[string]interface{}
	for _, r := range resultsRaw {
		if rm, ok := r.(map[string]interface{}); ok {
			typedResults = append(typedResults, rm)
		}
	}
	s.mu.Lock()
	agent := s.ensureAgent(agentID, payload)
	report := map[string]interface{}{
		"received_ts": nowTS(),
		"run_id":      payload["run_id"],
		"timestamp":   payload["timestamp"],
		"results":     typedResults,
	}
	// 依次分析 → 聚合 → 归档
	summary := summarizeResults(typedResults)
	structured := extractStructuredMetrics(typedResults)
	detail := extractDetailInfo(typedResults)
	archived := s.archiveReport(agentID, report)
	report["archive_files"] = archived

	// 更新 agent 状态
	agent.LastReportTS = report["received_ts"].(float64)
	agent.LastReport = report
	agent.LastReportSummary = summary
	agent.LastStructured = structured
	agent.LastDetails = detail
	agent.LastArchiveFiles = archived
	// 记录本次巡检的心跳序号，清除逾期告警
	if rSeq, ok := payload["seq"].(float64); ok {
		agent.LastCheckSeq = int(rSeq)
	}
	agent.CheckOverdue = false
	// 追加到报告历史，最多保留 10 条
	agent.Reports = append(agent.Reports, report)
	if len(agent.Reports) > 10 {
		agent.Reports = agent.Reports[len(agent.Reports)-10:]
	}
	s.saveState()
	s.mu.Unlock()
	sendJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
