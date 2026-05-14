package main

import (
	"net/http"
	"sort"
)

/*
 * buildStatusPayload 构建 API 响应的状态数据
 *
 * Go 语法要点：
 *   - make([]string, 0, len(m))  创建切片：类型, 初始长度 0, 容量
 *   - sort.Strings(slice)  字符串切片原地排序
 *   - for _, k := range keys  遍历切片或 map
 */
func (s *Server) buildStatusPayload() map[string]interface{} {
	current := nowTS()
	var agents []map[string]interface{}

	// map 遍历顺序是随机的，先收集 key 再排序保证输出稳定
	keys := make([]string, 0, len(s.state.Agents))
	for k := range s.state.Agents {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		a := s.state.Agents[k]
		// 超过 AliveTimeout 秒没心跳 = 离线
		alive := false
		if a.LastHeartbeatTS > 0 {
			alive = (current - a.LastHeartbeatTS) <= float64(s.cfg.AliveTimeout)
		}
		extra := s.agentExtraTargets[k]
		entry := map[string]interface{}{
			"agent_id":             a.AgentID,
			"ip":                   a.IP,
			"hostname":             a.Hostname,
			"heartbeat_count":      a.HeartbeatCount,
			"last_heartbeat_ts":    a.LastHeartbeatTS,
			"alive":                alive,
			"last_report_ts":       a.LastReportTS,
			"last_report_summary":  a.LastReportSummary,
			"last_structured":      a.LastStructured,
			"last_details":         a.LastDetails,
			"last_archive_files":   a.LastArchiveFiles,
			"pending_command":      a.PendingCommand,
			"extra_process_targets": extra,                                 // 该机器独有进程列表
			"process_targets":       mergeTargets(s.processTargets, extra), // 合并后的最终列表
			"next_check_seq":        a.NextCheckSeq,                        // agent 预告的下次巡检序号
			"last_check_seq":        a.LastCheckSeq,                        // 最近收到报告时的序号
			"check_overdue":         a.CheckOverdue,                        // 巡检是否逾期
		}
		agents = append(agents, entry)
	}
	return map[string]interface{}{
		"server_time":          current,
		"alive_timeout_seconds": s.cfg.AliveTimeout,
		"agents":               agents,
		"process_targets":      s.processTargets, // 当前进程配置，前端用于展示和编辑
	}
}

// GET /api/status → 返回所有 agent 状态
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	s.mu.RLock()
	payload := s.buildStatusPayload()
	s.mu.RUnlock()
	sendJSON(w, http.StatusOK, payload)
}
