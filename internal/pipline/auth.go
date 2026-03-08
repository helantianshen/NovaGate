package pipline

import (
	"NovaGate/internal/logger"

	"go.uber.org/zap"
)

// AuthMiddleware 模拟一个鉴权中间件
func AuthMiddleware() HandlerFunc {
	return func(c *Context) {
		path := c.Path
		logger.Log.Debug("[Auth Plugin] 开始鉴权拦截:", zap.String("path", path))

		// 模拟从参数或 Header 中获取 Token（暂从 Params 取）
		token := c.Params["token"]

		if token != "admin-secret-key" {
			logger.Log.Warn("[Auth Plugin] 鉴权失败，拦截请求！")
			c.AbortWithStatus(401, "Auth Failed: Invalid Token")
			return
		}

		logger.Log.Debug("[Auth Plugin] 鉴权通过，放行")
		c.Next() // 放行，去往下一个拦截器或业务处理逻辑

		logger.Log.Debug("[Auth Plugin] 业务处理完毕，可以在这里做耗时统计...")
	}
}
