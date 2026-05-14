/*
 * 代理端心跳 + 巡检客户端 (agent)
 * 功能：定时向主控端发送心跳，每 N 次心跳自动执行巡检并上报结果。
 * Bash 脚本通过 //go:embed 编译时嵌入，运行时释放到临时目录。
 */

package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

//go:embed scripts/*
var embeddedScripts embed.FS

func main() {
	cfgPath := findConfigPath()
	cfg := loadConfig(cfgPath)

	scriptsDir, err := extractScripts(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[agent] failed to extract scripts: %v\n", err)
		os.Exit(1)
	}

	agentID := cfg.AgentID
	if agentID == "" {
		agentID = hostname()
	}
	host := hostname()
	ip := localIP()
	seq := 0
	currentTargets := cfg.ProcessTargets

	fmt.Printf("[agent] start, id=%s, ip=%s, controller=%s\n", agentID, ip, cfg.ControllerBaseURL)

	for {
		seq++
		n := cfg.CheckEveryNHeartbeats
		nextCheckSeq := seq - (seq % n) + n
		if seq%n == 0 {
			nextCheckSeq = seq + n
		}
		heartbeatPayload := map[string]interface{}{
			"agent_id":       agentID,
			"ip":             ip,
			"hostname":       host,
			"timestamp":      isoNow(),
			"seq":            seq,
			"next_check_seq": nextCheckSeq,
		}
		resp, err := postJSON(cfg.ControllerBaseURL+"/heartbeat", heartbeatPayload)
		if err != nil {
			fmt.Printf("[agent] heartbeat failed seq=%d: %v\n", seq, err)
		} else {
			fmt.Printf("[agent] heartbeat seq=%d\n", seq)
			if newTargets, ok := resp["process_targets"].(string); ok && newTargets != "" && newTargets != currentTargets {
				currentTargets = newTargets
				cfg.ProcessTargets = newTargets
				checkConfigContent := generateCheckConfig(cfg)
				checkConfigPath := filepath.Join(scriptsDir, "check_config.sh")
				os.WriteFile(checkConfigPath, []byte(checkConfigContent), 0755)
				fmt.Printf("[agent] process targets updated: %s\n", newTargets)
			}
			if cmd, ok := resp["command"].(map[string]interface{}); ok {
				handleCommand(cmd, agentID, ip, host, seq, scriptsDir, cfg.ProcessName, cfg.ControllerBaseURL)
			}
		}

		if seq%cfg.CheckEveryNHeartbeats == 0 {
			fmt.Printf("[agent] run checks at seq=%d\n", seq)
			runChecks(scriptsDir, cfg.ProcessName, agentID, ip, host, seq, cfg.ControllerBaseURL)
			fmt.Printf("[agent] report sent at seq=%d\n", seq)
		}

		time.Sleep(time.Duration(cfg.HeartbeatIntervalSec) * time.Second)
	}
}
