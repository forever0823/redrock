package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

/*
 * loadState 从 JSON 文件恢复状态
 *
 * Go 语法要点：
 *   - func (s *Server) loadState()  方法定义，s 是接收者
 *   - os.ReadFile  一次性读取整个文件（Go 1.16+）
 *   - json.Unmarshal(data, &st)  将 JSON 字节反序列化到 Go struct
 *     &st 传指针，因为 Unmarshal 需要修改 st
 */
func (s *Server) loadState() {
	data, err := os.ReadFile(s.cfg.StateFile)
	if err != nil {
		return // 文件不存在就不加载
	}
	var st State
	// json.Unmarshal 返回 error，nil 表示成功
	if json.Unmarshal(data, &st) == nil && st.Agents != nil {
		s.state = st
		if st.ProcessTargets != "" {
			s.processTargets = st.ProcessTargets
		}
		if st.AgentExtraTargets != nil {
			s.agentExtraTargets = st.AgentExtraTargets
		}
	}
}

/*
 * saveState 将当前状态持久化到 JSON 文件
 *
 * Go 语法要点：
 *   - os.MkdirAll  递归创建目录（类似 mkdir -p）
 *   - 0755 是八进制文件权限（Unix 风格，Windows 上仅影响可读属性）
 *   - json.MarshalIndent  序列化为缩进的 JSON 字符串
 *   - os.WriteFile  一次性写入文件
 *   - _ 是空白标识符，用于忽略不需要的返回值
 */
func (s *Server) saveState() {
	s.state.ProcessTargets = s.processTargets
	s.state.AgentExtraTargets = s.agentExtraTargets
	os.MkdirAll(filepath.Dir(s.cfg.StateFile), 0755)
	data, _ := json.MarshalIndent(s.state, "", "  ") // 缩进 2 空格
	os.WriteFile(s.cfg.StateFile, data, 0644)
}

/*
 * ensureAgent 获取或创建 agent 状态（有则返回，无则初始化）
 *
 * Go 语法要点：
 *   - map 取值语法：agent, ok := m[key]；ok 为 bool 表示 key 是否存在
 *   - &AgentState{...}  取结构体字面量的地址，得到指针
 *   - map[string]int{"info": 0, ...}  带初始值的 map 字面量
 *   - 类型断言：v.(string)  将 interface{} 转为具体类型，第二个返回值 ok 表示成功
 */
func (s *Server) ensureAgent(agentID string, payload map[string]interface{}) *AgentState {
	agent, ok := s.state.Agents[agentID]
	if !ok {
		// 首次出现，初始化一个新状态
		agent = &AgentState{
			AgentID:           agentID,
			LastReportSummary: map[string]int{"info": 0, "warning": 0, "error": 0},
			LastStructured:    map[string]interface{}{},
			LastDetails:       map[string]interface{}{},
			LastArchiveFiles:  []string{},
			Reports:           []map[string]interface{}{},
		}
		s.state.Agents[agentID] = agent
	}
	// 更新 IP 和主机名（如果提供了新值）
	if ip, ok := payload["ip"].(string); ok && ip != "" {
		agent.IP = ip
	}
	if host, ok := payload["hostname"].(string); ok && host != "" {
		agent.Hostname = host
	}
	return agent
}
