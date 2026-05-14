/*
 * ===== 类型定义 (Go struct) =====
 *
 * Go 没有 class，用 struct 定义数据结构。
 * 语法：type 类型名 struct { 字段名 类型 `json:"json字段名"` }
 *
 * 反引号 `json:"xxx"` 叫 struct tag（结构体标签），
 * json 包会根据 tag 做序列化/反序列化的字段映射。
 * 例如 `json:"agent_id"` 表示 JSON 中的 "agent_id" 对应 Go 的 AgentID 字段。
 */

package main

import (
	"sync"
)

// Config 主控端配置，从 config.env 文件加载
type Config struct {
	Host             string // 监听地址，如 "0.0.0.0"
	Port             string // 监听端口，如 "8578"
	AliveTimeout     int    // 心跳超时判离线阈值（秒）
	StateFile        string // 状态持久化文件路径
	ReportArchiveDir string // 巡检报告归档目录
	ProcessTargets   string // 默认进程监控列表（逗号分隔），可在 Web 面板中动态修改
	CheckEveryN      int    // 巡检频率（每 N 次心跳巡检一次），用于逾期检测
	AdminPassword    string // 管理员密码（默认 changeme，可通过 Web 修改）
}

// AgentState 单个代理机的状态，会被序列化为 JSON 返回给前端
type AgentState struct {
	AgentID              string                   `json:"agent_id"`                // 代理机唯一标识
	IP                   string                   `json:"ip"`                      // 代理机 IP 地址
	Hostname             string                   `json:"hostname"`                // 代理机主机名
	LastHeartbeatTS      float64                  `json:"last_heartbeat_ts"`       // 最后一次心跳的时间戳（Unix 秒）
	LastHeartbeatPayload map[string]interface{}   `json:"last_heartbeat_payload"`  // 最后一次心跳的原始数据
	HeartbeatCount       int                      `json:"heartbeat_count"`         // 累计心跳次数
	LastReportTS         float64                  `json:"last_report_ts"`          // 最后一次巡检的时间戳
	LastReport           map[string]interface{}   `json:"last_report"`             // 最后一次巡检的完整报告
	LastReportSummary    map[string]int           `json:"last_report_summary"`     // 最近巡检的 INFO/WARNING/ERROR 计数
	LastStructured       map[string]interface{}   `json:"last_structured"`         // 最近巡检的结构化指标
	LastDetails          map[string]interface{}   `json:"last_details"`            // 最近巡检的详情（用于面板展开）
	LastArchiveFiles     []string                 `json:"last_archive_files"`      // 最近归档文件路径列表
	Reports              []map[string]interface{} `json:"reports"`                 // 最近 10 次巡检报告历史
	PendingCommand       map[string]interface{}   `json:"pending_command"`         // 待下发的命令（手动触发用）
	NextCheckSeq         int                      `json:"next_check_seq"`          // agent 预告的下次巡检心跳序号（逾期检测用）
	LastCheckSeq         int                      `json:"last_check_seq"`          // 最近一次收到巡检报告时的心跳序号
	CheckOverdue         bool                     `json:"check_overdue"`           // 巡检是否逾期未上报
}

// State 全局状态，包含所有 agent
type State struct {
	Agents            map[string]*AgentState `json:"agents"`              // key: agent_id, value: 状态指针
	ProcessTargets    string                 `json:"process_targets"`     // 全局进程监控列表（逗号分隔）
	AgentExtraTargets map[string]string      `json:"agent_extra_targets"` // 各 agent 独有的额外进程（key: agent_id, value: 逗号分隔）
}

/*
 * Server 主控服务器
 *
 * Go 语言中没有"类"的概念，但通过 struct + 方法可以实现类似效果。
 * 方法的接收者 (receiver) 写在 func 关键字和方法名之间：
 *   func (s *Server) 方法名() { ... }
 * 这里的 s 类似其他语言的 this/self，习惯用类型首字母小写。
 * *Server 表示指针接收者，可以修改 struct 字段。
 *
 * sync.RWMutex 是读写互斥锁，读多写少场景用 RLock/RUnlock 允许并发读。
 */
type Server struct {
	mu                sync.RWMutex       // 读写锁，保护 state 的并发访问
	state             State              // 全局状态
	cfg               Config             // 配置
	dashboard         []byte             // 仪表盘 HTML 内容（启动时加载到内存）
	processTargets    string             // 全局进程监控列表（Web 可动态修改，心跳下发至 agent）
	agentExtraTargets map[string]string  // 各 agent 独有的进程配置（key: agent_id）
	adminPassword     string             // 管理员密码
	sessionTokens     map[string]bool    // 有效的 session token 集合
	loginHTML         []byte             // 登录页面 HTML
}
