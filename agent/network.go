package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "127.0.0.1"
	}
	return addr.IP.String()
}

func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "unknown"
	}
	return h
}

func isoNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func postJSON(url string, payload map[string]interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("post failed: %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func findConfigPath() string {
	for _, p := range []string{"config.env", filepath.Join("..", "config.env")} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "config.env"
}

func handleCommand(cmd map[string]interface{}, agentID, ip, host string, seq int, scriptsDir, processName, controllerURL string) {
	if cmd == nil {
		return
	}
	cmdType, _ := cmd["type"].(string)
	if cmdType != "run_check" {
		return
	}
	fmt.Printf("[agent] receive command run_check: %v\n", cmd["command_id"])
	runChecks(scriptsDir, processName, agentID, ip, host, seq, controllerURL)
	fmt.Printf("[agent] command run_check done: %v\n", cmd["command_id"])
}
