package main

import (
	"net/http"
)

// POST /heartbeat → 处理心跳，返回待执行命令
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSON(r)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	agentID, _ := payload["agent_id"].(string)
	if agentID == "" {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id_required"})
		return
	}
	s.mu.Lock() // 写锁：修改 agent 状态
	agent := s.ensureAgent(agentID, payload)
	agent.HeartbeatCount++                     // 心跳计数 +1
	agent.LastHeartbeatTS = nowTS()            // 更新最后心跳时间
	agent.LastHeartbeatPayload = payload       // 保存最新的心跳数据
	// 巡检逾期检测：必须在更新 NextCheckSeq 之前，用旧的预告序号与当前序号比对
	seq, _ := payload["seq"].(float64)
	currentSeq := int(seq)
	if currentSeq > agent.NextCheckSeq+8 && agent.NextCheckSeq > 0 && agent.LastCheckSeq < agent.NextCheckSeq {
		agent.CheckOverdue = true
	}
	// 更新 agent 预告的下次巡检序号（来自心跳包）
	if ns, ok := payload["next_check_seq"].(float64); ok && int(ns) > 0 {
		agent.NextCheckSeq = int(ns)
	}
	// 如果有待执行的命令，取出并下发给 agent
	var cmd map[string]interface{}
	if agent.PendingCommand != nil {
		cmd = agent.PendingCommand
		cmd["delivered_ts"] = nowTS()
		cmd["delivered_heartbeat_seq"] = payload["seq"]
		agent.PendingCommand = nil // 取出后清空
	}
	// 合并全局 + 该 agent 独有进程配置，作为最终下发的 process_targets
	extra := s.agentExtraTargets[agentID]
	mergedTargets := mergeTargets(s.processTargets, extra)
	s.saveState()
	s.mu.Unlock()
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"command":         cmd,
		"process_targets": mergedTargets, // 下发合并后的列表，agent 直接使用
	})
}
