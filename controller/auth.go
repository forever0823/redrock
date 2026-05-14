package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

/*
 * checkAuth 验证请求中的 session cookie
 */
func (s *Server) checkAuth(r *http.Request) bool {
	cookie, err := r.Cookie("auth_token")
	if err != nil || cookie.Value == "" {
		return false
	}
	return s.sessionTokens[cookie.Value]
}

/*
 * genToken 生成随机 session token
 */
func genToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// → 登录（无需认证）
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSON(r)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	username, _ := payload["username"].(string)
	password, _ := payload["password"].(string)
	if username != "admin" || password != s.adminPassword {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return
	}
	token := genToken()
	s.mu.Lock()
	s.sessionTokens[token] = true
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400,
	})
	sendJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// → 修改密码（需认证）
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		sendJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	payload, err := readJSON(r)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	oldPwd, _ := payload["old_password"].(string)
	newPwd, _ := payload["new_password"].(string)
	if oldPwd != s.adminPassword {
		sendJSON(w, http.StatusForbidden, map[string]string{"error": "wrong_old_password"})
		return
	}
	if newPwd == "" {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "new_password_required"})
		return
	}
	s.mu.Lock()
	s.adminPassword = newPwd
	s.sessionTokens = map[string]bool{} // 清除所有 session，强制重新登录
	s.mu.Unlock()
	sendJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
