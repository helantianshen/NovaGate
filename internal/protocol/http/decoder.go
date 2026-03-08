package http

import (
	"bytes"
	"errors"
)

var (
	ErrInvalidRequestLine = errors.New("invalid http request line")
)

// Request 用于承载我们解析出来的 HTTP 核心属性
// 注意：这里全部使用 []byte 而不是 string，坚持 Zero-Copy 原则
type Request struct {
	Method  []byte
	URI     []byte
	Version []byte
}

// ParseRequestLine 解析 HTTP 请求首行
// 预期输入形如: []byte("GET /api/user?id=1 HTTP/1.1\r\n")
func ParseRequestLine(line []byte, req *Request) error {
	// 容错处理：剔除尾部可能存在的 \r\n 或 \n
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
	}

	// 寻找第一个空格，切割出 Method
	// bytes.IndexByte 在 Go 底层由汇编实现，速度极快
	idx := bytes.IndexByte(line, ' ')
	if idx == -1 {
		return ErrInvalidRequestLine
	}
	req.Method = line[:idx] // 零拷贝截取
	
	// 将处理过的部分裁掉，保留剩下的部分
	// " /api/user?id=1 HTTP/1.1" -> "/api/user?id=1 HTTP/1.1"
	line = line[idx+1:]

	// 寻找第二个空格，切割出 URI 和 Version
	idx = bytes.IndexByte(line, ' ')
	if idx == -1 {
		return ErrInvalidRequestLine
	}
	req.URI = line[:idx]     // 零拷贝截取
	req.Version = line[idx+1:] // 剩下的自然就是版本号

	return nil
}

// MethodStr 提供一个按需转换为 string 的便利方法（仅在打印日志等非高频场景使用）
func (r *Request) MethodStr() string {
	return string(r.Method)
}

func (r *Request) URIStr() string {
	return string(r.URI)
}