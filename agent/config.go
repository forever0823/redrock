package main

import (
	"fmt"
	"os"
	"strings"
)

// Config 统一配置结构体
type Config struct {
	ControllerBaseURL     string
	HeartbeatIntervalSec  int
	CheckEveryNHeartbeats int
	AgentID               string
	ProcessName           string
	CPUWarning            int
	MemWarning            int
	DiskWarning           int
	InodeWarning          int
	SwapWarningMB         int
	CPUIdleThreshold      int
	CheckInterval         int
	ThreadBlockThreshold  int
	FDWarningThreshold    int
	ProcessTargets        string
	SSHConfigFile         string
	MaxPasswordDays       int
	MinPasswordLength     int
}

func loadConfig(path string) Config {
	cfg := Config{
		ControllerBaseURL:     "http://127.0.0.1:8578",
		HeartbeatIntervalSec:  15,
		CheckEveryNHeartbeats: 40,
		ProcessName:           "",
		CPUWarning:            80,
		MemWarning:            85,
		DiskWarning:           85,
		InodeWarning:          80,
		SwapWarningMB:         100,
		CPUIdleThreshold:      1,
		CheckInterval:         3,
		ThreadBlockThreshold:  5,
		FDWarningThreshold:    10000,
		ProcessTargets:        "sshd,nginx,mysqld,redis-server",
		SSHConfigFile:         "/etc/ssh/sshd_config",
		MaxPasswordDays:       90,
		MinPasswordLength:     8,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "CONTROLLER_BASE_URL":
			cfg.ControllerBaseURL = val
		case "HEARTBEAT_INTERVAL_SECONDS":
			fmt.Sscanf(val, "%d", &cfg.HeartbeatIntervalSec)
		case "CHECK_EVERY_N_HEARTBEATS":
			fmt.Sscanf(val, "%d", &cfg.CheckEveryNHeartbeats)
		case "AGENT_ID":
			cfg.AgentID = val
		case "PROCESS_NAME":
			cfg.ProcessName = val
		case "CPU_WARNING":
			fmt.Sscanf(val, "%d", &cfg.CPUWarning)
		case "MEM_WARNING":
			fmt.Sscanf(val, "%d", &cfg.MemWarning)
		case "DISK_WARNING":
			fmt.Sscanf(val, "%d", &cfg.DiskWarning)
		case "INODE_WARNING":
			fmt.Sscanf(val, "%d", &cfg.InodeWarning)
		case "SWAP_WARNING_MB":
			fmt.Sscanf(val, "%d", &cfg.SwapWarningMB)
		case "CPU_IDLE_THRESHOLD":
			fmt.Sscanf(val, "%d", &cfg.CPUIdleThreshold)
		case "CHECK_INTERVAL":
			fmt.Sscanf(val, "%d", &cfg.CheckInterval)
		case "THREAD_BLOCK_THRESHOLD":
			fmt.Sscanf(val, "%d", &cfg.ThreadBlockThreshold)
		case "FD_WARNING_THRESHOLD":
			fmt.Sscanf(val, "%d", &cfg.FDWarningThreshold)
		case "PROCESS_TARGETS":
			cfg.ProcessTargets = val
		case "SSH_CONFIG_FILE":
			cfg.SSHConfigFile = val
		case "MAX_PASSWORD_DAYS":
			fmt.Sscanf(val, "%d", &cfg.MaxPasswordDays)
		case "MIN_PASSWORD_LENGTH":
			fmt.Sscanf(val, "%d", &cfg.MinPasswordLength)
		}
	}
	return cfg
}

func generateCheckConfig(cfg Config) string {
	return fmt.Sprintf(`#!/bin/bash

# 通用阈值
CPU_WARNING=%d
MEM_WARNING=%d
DISK_WARNING=%d
INODE_WARNING=%d
SWAP_WARNING_MB=%d

# 进程巡检参数
CPU_IDLE_THRESHOLD=%d
CHECK_INTERVAL=%d
THREAD_BLOCK_THRESHOLD=%d
FD_WARNING_THRESHOLD=%d

# 进程配置（逗号分隔，可自行扩展）
# 由 config.env 自动生成，修改配置请编辑 config.env 后重启 agent
PROCESS_TARGETS="%s"

# 安全基线配置
SSH_CONFIG_FILE="%s"
MAX_PASSWORD_DAYS=%d
MIN_PASSWORD_LENGTH=%d
`,
		cfg.CPUWarning, cfg.MemWarning, cfg.DiskWarning,
		cfg.InodeWarning, cfg.SwapWarningMB,
		cfg.CPUIdleThreshold, cfg.CheckInterval,
		cfg.ThreadBlockThreshold, cfg.FDWarningThreshold,
		cfg.ProcessTargets,
		cfg.SSHConfigFile, cfg.MaxPasswordDays, cfg.MinPasswordLength,
	)
}
