package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

/*
 * nowTS 返回当前 Unix 时间戳（float64 秒）
 *
 * Go 语法要点：
 *   - time.Now()  获取当前时间
 *   - .UnixNano()  纳秒时间戳（int64）
 *   - / 1e9  除以 10 亿转为秒，1e9 是科学计数法 = 1000000000
 *   - float64(...)  类型转换（Go 没有隐式类型转换，必须显式转换）
 */
func nowTS() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

/*
 * toString 将 interface{} 安全转为 string
 *
 * Go 语法要点：
 *   - interface{} 是空接口，可以表示任何类型（类似 Java Object / Python any）
 *   - switch val := v.(type)  类型 switch，根据 v 的实际类型分支
 *   - fmt.Sprintf("%v", val)  用默认格式将任意值转字符串
 */
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val) // 字节切片转字符串
	default:
		return fmt.Sprintf("%v", val) // 兜底转换
	}
}

/*
 * fixMojibake 修复乱码
 *
 * 问题背景：
 *   Bash 脚本的输出可能包含中文，如果被错误地按 latin1 编码解释，
 *   UTF-8 的多字节字符会变成乱码（如 "异" → "å¼‚"）。
 *   本函数检测并尝试修复这种编码错乱。
 *
 * Go 语法要点：
 *   - "\u00e5"  Unicode 转义（å 的码点 U+00E5）
 *   - []string{...}  字符串切片字面量
 *   - make([]byte, 0, len(text))  创建指定容量的字节切片
 *     make 用于 slice/map/chan，参数 (类型, 长度, 容量)
 *   - append(slice, elem...)  向切片追加元素
 *   - regexp.MustCompile  编译正则（失败时 panic，适合用作全局常量）
 *   - [\x{4e00}-\x{9fff}]  匹配中文字符的 Unicode 范围
 */
func fixMojibake(text string) string {
	if text == "" {
		return text
	}
	// 乱码特征字符：UTF-8 被 latin1 解释后的常见字节模式
	suspects := []string{"\u00e5", "\u00e4", "\u00e7", "\u00e8", "\u00e9", "\u00e6", "\u00ef", "\u00bc", "\u009f"}
	hasSuspect := false
	for _, s := range suspects {
		if strings.Contains(text, s) {
			hasSuspect = true
			break
		}
	}
	if !hasSuspect {
		return text // 没有乱码迹象，原样返回
	}
	// 尝试 latin1 → UTF-8 修复
	latin1Bytes := make([]byte, 0, len(text))
	for _, r := range text {
		if r <= 0xFF {
			latin1Bytes = append(latin1Bytes, byte(r))
		} else {
			latin1Bytes = append(latin1Bytes, '?')
		}
	}
	repaired := string(latin1Bytes)
	// 质量判断：修复后中文字符变多，说明修复有效
	cnRe := regexp.MustCompile(`[\x{4e00}-\x{9fff}]`)
	oldCN := len(cnRe.FindAllString(text, -1))
	newCN := len(cnRe.FindAllString(repaired, -1))
	if newCN > oldCN {
		return repaired
	}
	return text
}

// safeText 将值转字符串（函数别名，语义化命名）
func safeText(v interface{}) string {
	return toString(v)
}

/*
 * coalesce 返回第一个非空字符串（类似 SQL 的 COALESCE）
 * 依次尝试 a, b，都为空则返回 def 默认值
 */
func coalesce(a, b interface{}, def string) string {
	s := toString(a)
	if s != "" {
		return s
	}
	s = toString(b)
	if s != "" {
		return s
	}
	return def
}

/*
 * parseFloat 从字符串解析浮点数
 * fmt.Sscanf 类似 C 的 sscanf，从字符串按格式读取
 */
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(strings.TrimSpace(s), "%f", &f)
	return f
}

/*
 * sendJSON 发送 JSON 响应
 *
 * http.ResponseWriter 是 HTTP 响应接口：
 *   1. Header().Set()  设置响应头
 *   2. WriteHeader(code)  写状态码（必须在 Write 之前调用）
 *   3. Write(data)  写响应体
 */
func sendJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	data, _ := json.Marshal(payload) // 序列化为 JSON 字节
	w.WriteHeader(code)               // 设置 HTTP 状态码
	w.Write(data)                     // 写入响应体
}

/*
 * readJSON 读取并解析请求中的 JSON 数据
 *
 * Go 语法要点：
 *   - io.ReadAll(r.Body)  读取请求体全部字节
 *   - json.Unmarshal(data, &payload)  反序列化，&payload 传指针
 *   - if err := fn(); err != nil { }  将函数调用和错误判断写在一行（Go 常用模式）
 */
func readJSON(r *http.Request) (map[string]interface{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	return payload, nil
}
