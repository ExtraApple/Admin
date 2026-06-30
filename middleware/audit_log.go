package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"admin/model"
	"admin/service"

	"github.com/gin-gonic/gin"
)

const maxAuditBodySize = 2000

// sensitiveAuditFields 定义写入审计日志前必须脱敏的 JSON 字段名。
var sensitiveAuditFields = map[string]struct{}{
	"password":         {},
	"old_password":     {},
	"new_password":     {},
	"confirm_password": {},
	"captcha_code":     {},
	"access_token":     {},
	"refresh_token":    {},
	"token":            {},
}

// AuditLog 记录每个 /api 请求的审计信息，并在请求结束后异步写入数据库。
func AuditLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		body := readAuditBody(c)

		c.Next()

		log := model.AuditLog{
			UserID:    c.GetUint("userID"),
			Username:  c.GetString("username"),
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			Query:     c.Request.URL.RawQuery,
			Body:      sanitizeAuditBody(body),
			Status:    c.Writer.Status(),
			Duration:  time.Since(start).Milliseconds(),
			ClientIP:  c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Category:  auditCategory(c.Request.Method, c.Request.URL.Path),
		}

		go service.CreateAuditLog(&log)
	}
}

// readAuditBody 安全读取请求体，并把 Body 放回请求，避免后续 handler 无法再次读取。
func readAuditBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "multipart/form-data") {
		return []byte("[multipart omitted]")
	}
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		return nil
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Request.Body = io.NopCloser(bytes.NewBuffer(nil))
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return bodyBytes
}

// sanitizeAuditBody 对请求体做脱敏和长度限制，避免审计日志泄露敏感信息或无限膨胀。
func sanitizeAuditBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if string(body) == "[multipart omitted]" {
		return string(body)
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return truncateAuditBody(string(body))
	}

	maskSensitiveFields(data)
	sanitized, err := json.Marshal(data)
	if err != nil {
		return truncateAuditBody(string(body))
	}
	return truncateAuditBody(string(sanitized))
}

// truncateAuditBody 将过长的请求体截断到审计日志允许的最大长度。
func truncateAuditBody(body string) string {
	if len(body) <= maxAuditBodySize {
		return body
	}
	return body[:maxAuditBodySize] + "..."
}

// maskSensitiveFields 递归脱敏 JSON 对象中的敏感字段，支持嵌套对象和对象数组。
func maskSensitiveFields(data map[string]any) {
	for key, value := range data {
		if _, ok := sensitiveAuditFields[strings.ToLower(key)]; ok {
			data[key] = "***"
			continue
		}

		switch v := value.(type) {
		case map[string]any:
			maskSensitiveFields(v)
		case []any:
			for _, item := range v {
				if nested, ok := item.(map[string]any); ok {
					maskSensitiveFields(nested)
				}
			}
		}
	}
}
