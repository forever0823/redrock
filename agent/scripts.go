package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func extractScripts(cfg Config) (string, error) {
	scriptsDir, err := os.MkdirTemp("", "bash_check_scripts")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	entries, err := embeddedScripts.ReadDir("scripts")
	if err != nil {
		return "", fmt.Errorf("read embedded scripts: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := embeddedScripts.ReadFile("scripts/" + entry.Name())
		if err != nil {
			return "", fmt.Errorf("read embedded %s: %w", entry.Name(), err)
		}
		dst := filepath.Join(scriptsDir, entry.Name())
		if err := os.WriteFile(dst, data, 0755); err != nil {
			return "", fmt.Errorf("write %s: %w", entry.Name(), err)
		}
	}

	checkConfigContent := generateCheckConfig(cfg)
	checkConfigPath := filepath.Join(scriptsDir, "check_config.sh")
	if err := os.WriteFile(checkConfigPath, []byte(checkConfigContent), 0755); err != nil {
		return "", fmt.Errorf("write check_config.sh: %w", err)
	}
	fmt.Printf("[agent] scripts extracted to: %s\n", scriptsDir)
	return scriptsDir, nil
}

func runSingleScript(scriptsDir, scriptName, processName string) map[string]interface{} {
	scriptPath := filepath.Join(scriptsDir, scriptName)
	start := time.Now()
	result := map[string]interface{}{
		"script":       scriptName,
		"started_at":   float64(start.UnixNano()) / 1e9,
		"ended_at":     float64(start.UnixNano()) / 1e9,
		"duration_sec": 0.0,
		"status":       "ok",
		"output":       "",
		"error":        "",
	}

	if _, err := os.Stat(scriptPath); err != nil {
		result["status"] = "error"
		result["error"] = fmt.Sprintf("script not found: %s", scriptPath)
		elapsed := time.Since(start).Seconds()
		result["ended_at"] = float64(start.UnixNano())/1e9 + elapsed
		result["duration_sec"] = float64(int(elapsed*100)) / 100
		return result
	}

	args := []string{scriptPath}
	if scriptName == "process_check.sh" && processName != "" {
		args = append(args, processName)
	}

	cmd := exec.Command("bash", args...)
	cmd.Dir = scriptsDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		stdoutStr := string(stdout.Bytes())
		stderrStr := string(stderr.Bytes())
		result["output"] = stdoutStr + stderrStr
		if err != nil {
			result["status"] = "error"
			if result["output"].(string) == "" {
				if exitErr, ok := err.(*exec.ExitError); ok {
					result["error"] = fmt.Sprintf("exit code %d", exitErr.ExitCode())
				} else {
					result["error"] = err.Error()
				}
			}
		}
	case <-time.After(300 * time.Second):
		cmd.Process.Kill()
		result["status"] = "error"
		result["error"] = "script timeout after 300s"
	}

	elapsed := time.Since(start).Seconds()
	result["ended_at"] = float64(start.UnixNano())/1e9 + elapsed
	result["duration_sec"] = float64(int(elapsed*100)) / 100
	return result
}

func runChecks(scriptsDir, processName, agentID, ip, host string, seq int, controllerURL string) {
	scriptSeq := []string{
		"system_check.sh",
		"security_baseline_check.sh",
		"process_check.sh",
	}
	var results []map[string]interface{}
	for _, name := range scriptSeq {
		r := runSingleScript(scriptsDir, name, processName)
		results = append(results, r)
	}
	runID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), seq)
	payload := map[string]interface{}{
		"agent_id":  agentID,
		"ip":        ip,
		"hostname":  host,
		"timestamp": isoNow(),
		"seq":       seq,
		"run_id":    runID,
		"results":   results,
	}
	_, err := postJSON(controllerURL+"/report", payload)
	if err != nil {
		fmt.Printf("[agent] report failed: %v\n", err)
	}
}
