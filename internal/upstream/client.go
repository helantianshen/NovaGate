package upstream

import (
	"NovaGate/internal/logger"
	"NovaGate/internal/pipline"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// ForwardTo 是反向代理的核心逻辑：将请求透传给指定的后端地址
func ForwardTo(c *pipline.Context, pool *HostPool) {
	// 作为客户端，主动去连接真实的后端微服务
	// 从池子里极速拿出一个连接 (零 TCP 握手开销)
	upstreamConn, err := pool.Get()
	if err != nil {
		logger.Log.Error("获取上游连接失败", zap.Error(err))
		c.String(502, "Bad Gateway")
		return
	}

	// 状态机
	// 决定请求处理完后，连接是复用还是销毁
	shouldKeepAlive := true
	defer func() {
		if shouldKeepAlive {
			pool.Put(upstreamConn) // 安全回收
		} else {
			upstreamConn.Close() // 暴力销毁
		}
	}()

	// 路径重写
	// 我们的路由是 /api/order/:id，真实的微服务只需要拿到 id 即可
	// 我们利用之前解析好的 params，重新拼接发给后端的路径
	// 直接使用 Context 中的 Path，如果存在 Query 参数，原样拼接回去
	targetURI := c.Path
	if c.RawQuery != "" {
		targetURI += "?" + c.RawQuery
	}

	// 组装发给后端的 HTTP 请求报文
	// 真实的网关会做 Header 的透传（Zero-Copy），并追加 X-Forwarded-For 等字段
	// 这里基于 Context 里的信息重新拼装
	upWriter := upstreamConn.Writer()
	upWriter.WriteString(c.Method + " " + targetURI + " HTTP/1.1\r\n")
	upWriter.WriteString("Host: " + pool.Addr + "\r\n")
	// 通知微服务，不要断开 TCP
	upWriter.WriteString("Connection: keep-alive\r\n\r\n")
	upWriter.Flush()

	// 读取后端的响应，并以流式（Stream）的方式透传回给真实的客户端
	upReader := upstreamConn.Reader()
	clientWriter := c.Conn.Writer()

	contentLength := -1
	isChunked := false

	// 逐行解析 HTTP 响应头，寻找报文边界
	for {

		lineBytes, err := upReader.Until('\n')
		if err != nil {
			shouldKeepAlive = false
			break
		}

		// 使用 WriteBinary 写入字节切片 (此时只是链接指针，尚未发送)
		clientWriter.WriteBinary(lineBytes)

		line := string(lineBytes)
		if line == "\r\n" || line == "\n" {
			break // 响应头结束，准备处理 Body
		}

		// 统一转小写匹配，提取长度
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "content-length:") {
			parts := strings.Split(lowerLine, ":")
			if len(parts) == 2 {
				contentLength, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		} else if strings.HasPrefix(lowerLine, "transfer-encoding: chunked") {
			isChunked = true
		}

	}

	// 根据报文长度，精准提取 Body
	if contentLength > 0 {
		// 完美情况：明确知道长度，零拷贝取出并安全释放
		buf, _ := upReader.Next(contentLength)
		clientWriter.WriteBinary(buf)
		// 必须先 Flush，确保数据已经安全发送
		clientWriter.Flush()
		// 然后才能释放上游的内存池，否则会内存越界 panic
		upReader.Release()
	} else if isChunked || contentLength < 0 {
		// 边缘情况：对于 chunked 或者无长度的流，为了保证下一次复用的安全
		// 最稳妥的降级策略是盲读并废弃这条 TCP 连接
		shouldKeepAlive = false
		for {
			_, err := upReader.Peek(1)
			if err != nil {
				break
			}
			buf, _ := upReader.Next(upReader.Len())
			clientWriter.WriteBinary(buf)

			clientWriter.Flush()
			upReader.Release()
		}
	} else {
		// contentLength == 0 的情况（比如某些 200 OK 或 404 没 Body）
		clientWriter.Flush()
		upReader.Release()
	}

	clientWriter.Flush()

	// 代理完成，终止网关 Context 责任链的后续操作
	c.Abort()
}
