package pipline

// HandlerFunc 管道中的处理节点（拦截器/中间件/终端路由）
type HandlerFunc func(c *Context)
