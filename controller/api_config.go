package main

import (
	"fmt"
	"net/http"
	"time"
)

// → 单机进程配置管理（追加到全局配置之后）
func (s *Server) handleAgentProcessConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
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
	extraStr, _ := payload["extra_targets"].(string)
	s.mu.Lock()
	if s.agentExtraTargets == nil {
		s.agentExtraTargets = map[string]string{}
	}
	if extraStr == "" {
		delete(s.agentExtraTargets, agentID) // 空 = 清空该机器独有配置
	} else {
		s.agentExtraTargets[agentID] = extraStr
	}
	merged := mergeTargets(s.processTargets, extraStr)
	s.saveState()
	s.mu.Unlock()
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"agent_id":        agentID,
		"extra_targets":   extraStr,
		"merged_targets":  merged,
	})
}

// POST /api/process-config → 修改全局进程监控列表
func (s *Server) handleProcessConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	payload, err := readJSON(r)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	newTargets, _ := payload["process_targets"].(string)
	if newTargets == "" {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "process_targets_required"})
		return
	}
	s.mu.Lock()
	s.processTargets = newTargets
	s.saveState()
	s.mu.Unlock()
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"process_targets": s.processTargets,
	})
}

// POST /trigger-check → 手动触发巡检
func (s *Server) handleTriggerCheck(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
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
	s.mu.Lock()
	agent := s.ensureAgent(agentID, payload)
	// 创建一个"立即巡检"命令放入待执行队列
	cmdID := fmt.Sprintf("cmd-%d", time.Now().UnixMilli())
	cmd := map[string]interface{}{
		"type":       "run_check",
		"reason":     "manual_trigger",
		"created_ts": nowTS(),
		"command_id": cmdID,
	}
	agent.PendingCommand = cmd
	s.saveState()
	s.mu.Unlock()
	sendJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "queued_command": cmd})
}
