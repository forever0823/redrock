/*
 * ============================================================
 * 主控端 HTTP 服务器 (controller)
 * ============================================================
 * 功能：
 *   1. GET  /            → 提供仪表盘 HTML 页面
 *   2. GET  /api/status  → 返回所有 agent 的状态 JSON
 *   3. POST /heartbeat   → 接收 agent 心跳，返回待执行命令
 *   4. POST /report      → 接收巡检报告，解析指标并归档
 *   5. POST /trigger-check → 手动触发指定 agent 立即巡检
 *
 * 状态通过 controller_state.json 持久化，重启可恢复。
 * ============================================================
 */

// Go 语言规范：每个 .go 文件必须以 package 声明开头。
// main 包生成可执行文件，其他名字的包生成库文件。
package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

/*
 * ServeHTTP 实现 http.Handler 接口，处理所有 HTTP 请求
 *
 * Go 语法要点：
 *   - switch { case cond1: ... case cond2: ... }  条件 switch（无表达式，逐个 case 判断）
 *   - s.mu.RLock() / s.mu.RUnlock()  读锁（允许多个并发读）
 *   - s.mu.Lock() / s.mu.Unlock()  写锁（独占，与读锁互斥）
 *
 * 路由分发：
 *   GET  /            → 返回仪表盘 HTML
 *   GET  /api/status  → 返回所有 agent 状态
 *   POST /heartbeat   → 处理心跳，返回待执行命令
 *   POST /report      → 处理巡检报告
 *   POST /trigger-check → 手动触发巡检
 */
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 条件 switch：依次判断每个 case 的条件
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/":
		if !s.checkAuth(r) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(s.loginHTML)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(s.dashboard) // 返回缓存的 HTML

	case r.Method == http.MethodGet && r.URL.Path == "/api/status":
		s.handleStatus(w, r)

	// → 登录（无需认证）
	case r.Method == http.MethodPost && r.URL.Path == "/api/login":
		s.handleLogin(w, r)

	// → 修改密码（需认证）
	case r.Method == http.MethodPost && r.URL.Path == "/api/change-password":
		s.handleChangePassword(w, r)

	// → 单机进程配置管理（追加到全局配置之后）
	case r.Method == http.MethodPost && r.URL.Path == "/api/agent-process-config":
		s.handleAgentProcessConfig(w, r)

	case r.Method == http.MethodPost && r.URL.Path == "/api/process-config":
		s.handleProcessConfig(w, r)

	case r.Method == http.MethodPost && r.URL.Path == "/heartbeat":
		s.handleHeartbeat(w, r)

	case r.Method == http.MethodPost && r.URL.Path == "/trigger-check":
		s.handleTriggerCheck(w, r)

	case r.Method == http.MethodPost && r.URL.Path == "/report":
		s.handleReport(w, r)

	default:
		// 未匹配的路由返回 404
		sendJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

/*
 * main 程序入口
 *
 * Go 的可执行程序必须有 main 包里的 main() 函数。
 * main() 无参数、无返回值；命令行参数通过 os.Args 获取。
 *
 * http.ListenAndServe(addr, handler)  启动 HTTP 服务器，阻塞直到出错。
 * 第二个参数是实现 http.Handler 接口的对象（有 ServeHTTP 方法）。
 */
func main() {
	cfg := loadConfig("config.env")

	// 查找 dashboard.html：先在当前目录找，再在上级目录找
	dashPath := ""
	for _, p := range []string{"dashboard.html", filepath.Join("..", "dashboard.html")} {
		if _, err := os.Stat(p); err == nil { // os.Stat 检查文件是否存在
			dashPath = p
			break
		}
	}
	dashData, _ := os.ReadFile(dashPath) // 加载 HTML 到内存

	// 加载登录页面
	loginPath := ""
	for _, p := range []string{"login.html", filepath.Join("..", "login.html")} {
		if _, err := os.Stat(p); err == nil {
			loginPath = p
			break
		}
	}
	loginData, _ := os.ReadFile(loginPath)

	// &Server{...}  创建 Server 并取指针
	srv := &Server{
		state:             State{Agents: map[string]*AgentState{}},
		cfg:               cfg,
		dashboard:         dashData,
		loginHTML:         loginData,
		processTargets:    cfg.ProcessTargets,
		agentExtraTargets: map[string]string{},
		adminPassword:     cfg.AdminPassword,
		sessionTokens:     map[string]bool{},
	}
	srv.loadState() // 加载上次持久化的状态

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port) // "0.0.0.0:8578"
	fmt.Printf("[controller] listening on http://%s\n", addr)

	// 启动 HTTP 服务器，阻塞运行
	if err := http.ListenAndServe(addr, srv); err != nil {
		fmt.Fprintf(os.Stderr, "[controller] failed: %v\n", err)
		os.Exit(1) // 异常退出，状态码 1
	}
}
