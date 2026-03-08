package connection

import (
	"NovaGate/internal/logger"
	"NovaGate/internal/pipline"
	"NovaGate/internal/protocol/http"
	"NovaGate/internal/router"
	"net/url"

	"github.com/cloudwego/netpoll"
	"go.uber.org/zap"
)

func Process(conn netpoll.Connection, r *router.Router) {

	reader := conn.Reader()

	// 读取并解析请求行，只读取第一行（请求行）
	// 请求行以 \n 结尾
	line, err := reader.Until('\n')
	if err != nil {
		conn.Close()
		return
	}

	// 初始化一个 Request 结构体并解析
	req := &http.Request{}
	err = http.ParseRequestLine(line, req)
	if err != nil {
		logger.Log.Error("Parse request line failed:", zap.Error(err))
		conn.Close()
		return
	}

	var reqBuf []byte
	// 消耗掉剩下的 Header (防止沾包)
	for {
		headerLine, _ := reader.Until('\n')
		reqBuf = append(reqBuf, headerLine...)
		if len(headerLine) <= 2 { // 遇到空行 \r\n，说明 Header 结束
			break
		}
	}
	// 释放读缓冲区（Netpoll nocopy 机制要求）
	reader.Release()

	// 剥离 Query String (将 "/api/user?id=1" 变成 "/api/user")
	// 解析 Query String (将 "/api/order/index.html?token=123" 拆分)
	// 真正的路由树匹配只看 Path
	// 使用 Go 标准库完美解析 URI
	// url.ParseRequestURI 能够自动剥离 Query 并且处理 %20 等转义字符
	u, err := url.ParseRequestURI(string(req.URI))
	if err != nil {
		conn.Close()
		return
	}

	method := string(req.Method)
	pathStr := u.Path // 这里拿到的是纯净的 "/api/order/index.html"

	// 路由匹配
	handlers, params, ok := r.GetRoute(method, pathStr)

	// 执行路由分发
	if ok {
		// 解析 Query 参数并合并到 params 里
		if params == nil {
			params = make(map[string]string)
		}

		// u.Query() 会把解析好的参数变成一个 map[string][]string
		// 遍历它，把参数塞进咱们的 Context Params 里
		for k, v := range u.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}

		// 匹配成功，初始化责任链 Context
		// c := &pipline.Context{
		// 	Conn:     conn,
		// 	Method:   method,
		// 	Path:     pathStr,
		// 	RawQuery: u.RawQuery,
		// 	Params:   params, // params 里既有动态路径参数，也有 Query 参数
		// 	Handlers: handlers,
		// 	Index:    -1,
		// }

		// 优化：从池子中获取 Context
		c := pipline.AllocateContext()
		c.Conn = conn
		c.Method = method
		c.Path = pathStr
		c.RawQuery = u.RawQuery
		c.Params = params
		c.Handlers = handlers
		c.Index = -1

		// 启动责任链引擎 (开始依次执行 middleware 和 handler)
		c.Next()

		// 优化：请求处理完毕，将 Context 放回池子
		// 注意：只有当确定不再使用 c 时才能 Release
		c.Release()

		// 将长连接管理交给 Netpoll 管理
		// conn.Close()
	} else {
		// 规范的 Netpoll 写入方式
		// 没找到路由，统一返回 404
		writer := conn.Writer()
		writer.WriteString("HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nNot Found")

		// 必须 Flush 才会真正发送到 socket
		writer.Flush()

		// 注意：如果是 HTTP/1.1 Keep-Alive，不应该立刻 Close
		// Demo 阶段，先主动关闭连接，避免 curl 挂起等待 Keep-Alive
		conn.Close()
	}
}
