package upstream

import (
	"NovaGate/internal/logger"
	"sync/atomic"
	"time"

	"github.com/cloudwego/netpoll"
	"go.uber.org/zap"
)

// HostPool 负责管理单个后端节点的长连接池
type HostPool struct {
	Addr  string                  // 后端节点地址
	conns chan netpoll.Connection // 使用 channel 充当对象池
	Alive atomic.Bool             // 节点存活状态标志
}

func NewHostPool(addr string, maxIdle int) *HostPool {
	p := &HostPool{
		Addr:  addr,
		conns: make(chan netpoll.Connection, maxIdle),
	}
	// 默认上线状态为存货
	p.Alive.Store(true)

	return p
}

// Get 获取一个可用的连接
func (p *HostPool) Get() (netpoll.Connection, error) {
	select {
	case conn := <-p.conns:
		// 检查连接是否仍然存活
		if conn.IsActive() {
			return conn, nil
		}
		// 已经是死连接了，关掉并递归再拿一个
		conn.Close()
		return p.Get()
	default:
		// 池子空了，或者初次启动，动态新建一条连接
		logger.Log.Debug("动态新建上游连接", zap.String("addr", p.Addr))
		return netpoll.DialConnection("tcp", p.Addr, 3*time.Second)
	}
}

// Put 用完后将健康连接归还到池子
func (p *HostPool) Put(conn netpoll.Connection) {
	// 如果连接已经失效（比如被后端主动断开了），直接丢弃
	if !conn.IsActive() {
		conn.Close()
		return
	}

	select {
	case p.conns <- conn:
		// 成功放入池中
	default:
		// 池子满了（超出 maxIdle），直接关掉，避免内存泄漏
		conn.Close()
	}
}
