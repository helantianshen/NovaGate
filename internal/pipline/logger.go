package pipline

import (
	"NovaGate/internal/logger"
	"time"

	"go.uber.org/zap"
)

// AsyncLoggerMiddleware 异步访问日志插件
func AsyncLoggerMiddleware() HandlerFunc {
	return func(c *Context) {
		startTime := time.Now()

		// 往下执行核心业务
		c.Next()

		cost := time.Since(startTime)

		// 拷贝需要的数据
		// 因为 c.Release() 马上就要被调用了
		// 不能把 Context 指针传进协程
		method := c.Method
		path := c.Path
		query := c.RawQuery

		// Zap 的精髓：强类型字段 (zap.String, zap.Duration)
		// 绝对不要用 fmt.Sprintf 拼接字符串
		logger.Log.Info("Access Log",
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Duration("cost", cost),
		)

		// 将磁盘 I/O 耗时操作，丢进专属的高性能协程池
		// 彻底解放 Netpoll 的主工作协程
		// gopool.Go(func() {
		// 	// TODO: 在生产环境中，这里将日志发往 Kafka 或 ElasticSearch
		// })
	}
}
