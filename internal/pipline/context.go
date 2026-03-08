package pipline

import (
	"math"
	"strconv"
	"sync"

	"github.com/cloudwego/netpoll"
)

// abortIndex 用于标记责任链被提前终止
const abortIndex int8 = math.MaxInt8 / 2

// Context 贯穿整个请求生命周期的上下文
// 不仅要携带底层的网络连接（Netpoll Connection）和请求参数
// 还要负责控制责任链的执行进度
type Context struct {
	Conn     netpoll.Connection
	Method   string
	Path     string
	Params   map[string]string // 动态路由参数，如 :id
	RawQuery string            // 保存原始的 Query String，比如 "token=abc&name=1"
	Handlers []HandlerFunc     // 当前请求需要经过的拦截器链 + 最终的业务 Handler
	Index    int8              // 当前执行到了第几个拦截器
}

// 定义全局的 Context 对象池
var contextPool = sync.Pool{
	New: func() any {
		return &Context{}
	},
}

// 从池子中获取对象
func AllocateContext() *Context {
	return contextPool.Get().(*Context)
}

// 用完后清理脏数据，并放回池子
func (c *Context) Release() {
	c.Conn = nil
	c.Method = ""
	c.Path = ""
	c.RawQuery = ""
	c.Params = nil
	c.Handlers = nil
	c.Index = -1
	contextPool.Put(c)
}

// Next 执行下一个拦截器
// 核心原理：Next() 是用一个 for 循环按顺序执行 Handler
// 如果某个前置中间件（如 Auth）调用了 Abort()
// c.index 会被设置成极其大的值，导致剩余的 Handler 被全部跳过
func (c *Context) Next() {
	c.Index++
	for c.Index < int8(len(c.Handlers)) {
		c.Handlers[c.Index](c)
		c.Index++
	}
}

// Abort 终止责任链的继续往下执行（通常在鉴权失败或限流时调用）
func (c *Context) Abort() {
	c.Index = abortIndex
}

// 封装几个极其常用的便捷响应方法，避免每次都要手写 HTTP 报文
// AbortWithStatus 中断并返回指定状态码和内容
func (c *Context) AbortWithStatus(status int, body string) {
	c.String(status, body)
	c.Abort()
}

// String 写回普通的纯文本响应
func (c *Context) String(status int, body string) {
	// 这里简单写死一些常见的状态码文本，生产中可以用 map 维护
	statusText := "OK"
	if status == 401 {
		statusText = "Unauthorized"
	} else if status == 403 {
		statusText = "Forbidden"
	} else if status == 429 {
		statusText = "Too Many Requests"
	} else if status == 500 {
		statusText = "Internal Server Error"
	}

	response := "HTTP/1.1 " + strconv.Itoa(status) + " " + statusText + "\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body

	writer := c.Conn.Writer()
	writer.WriteString(response)
	writer.Flush()
}
