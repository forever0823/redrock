package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

/*
 * mergeTargets 合并全局和单机独有进程列表（去重，全局在前）
 * 例如：全局 "a,b,c" + 独有 "c,d" → "a,b,c,d"
 */
func mergeTargets(global, extra string) string {
	seen := map[string]bool{}
	var result []string
	for _, p := range strings.Split(global, ",") {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	for _, p := range strings.Split(extra, ",") {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return strings.Join(result, ",")
}

/*
 * loadConfig 从 key=value 格式的配置文件加载配置
 *
 * Go 语法要点：
 *   - func 函数名(参数 类型) 返回类型 { ... }
 *   - cfg := Config{...}  短变量声明 + 复合字面量初始化
 *   - f, err := os.Open(path)  多返回值，Go 习惯用第二个返回值表示错误
 *   - if err != nil { ... }  Go 没有 try/catch，显式检查错误
 *   - defer f.Close()  延迟执行，在函数返回前自动调用（类似 finally）
 *   - bufio.NewScanner  逐行读取
 *   - strings.SplitN(s, sep, n)  分割字符串，最多分 n 份
 *   - fmt.Sscanf  从字符串按格式读取数值
 */
func loadConfig(path string) Config {
	// 先设置默认值
	cfg := Config{
		Host:             "0.0.0.0",
		Port:             "8578",
		AliveTimeout:     35,
		StateFile:        "controller_state.json",
		ReportArchiveDir: "report_archive",
		ProcessTargets:   "sshd,nginx,mysqld,redis-server",
		CheckEveryN:      40,
		AdminPassword:    "changeme",
	}
	// os.Open 返回两个值：*File 和 error
	f, err := os.Open(path)
	if err != nil {
		return cfg // 文件打不开就用默认值
	}
	defer f.Close() // 函数返回时自动关闭文件

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和 # 开头的注释行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 按第一个 = 分割成 [key, value]
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// switch 语句匹配配置项
		switch key {
		case "CONTROLLER_HOST":
			cfg.Host = val
		case "CONTROLLER_PORT":
			cfg.Port = val
		case "ALIVE_TIMEOUT_SECONDS":
			fmt.Sscanf(val, "%d", &cfg.AliveTimeout) // %d 按十进制整数解析
		case "STATE_FILE":
			cfg.StateFile = val
		case "REPORT_ARCHIVE_DIR":
			cfg.ReportArchiveDir = val
		case "PROCESS_TARGETS":
			cfg.ProcessTargets = val
		case "CHECK_EVERY_N_HEARTBEATS":
			fmt.Sscanf(val, "%d", &cfg.CheckEveryN)
		case "ADMIN_PASSWORD":
			cfg.AdminPassword = val
		}
	}
	return cfg
}
