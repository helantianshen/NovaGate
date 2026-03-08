package pipline

import (
	"NovaGate/internal/config"
	"NovaGate/internal/logger"
)

// RateLimitMiddleware 模拟一个网关限流器
func RateLimitMiddleware() HandlerFunc {
	return func(c *Context) {
		logger.Log.Debug("[RateLimit Plugin] 检查令牌桶...")

		// 对接 Nacos 动态判断
		isLimit := config.Manager.GetConfig().RateLimit.Enabled
		if isLimit {
			logger.Log.Debug("[RateLimit Plugin] 触发限流！")
			c.AbortWithStatus(429, "Too Many Requests")
			return
		}

		c.Next()
	}
}
