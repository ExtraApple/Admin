package middleware

import (
	"time"

	"admin/global"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ZapLogger 将每个 HTTP 请求写入运行日志，按状态码区分 Info/Warn/Error。
func ZapLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if userID, ok := c.Get("userID"); ok {
			fields = append(fields, zap.Any("user_id", userID))
		}

		if len(c.Errors) > 0 || c.Writer.Status() >= 500 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
			global.Logger.Error("http request", fields...)
			return
		}

		if c.Writer.Status() >= 400 {
			global.Logger.Warn("http request", fields...)
			return
		}

		global.Logger.Info("http request", fields...)
	}
}
