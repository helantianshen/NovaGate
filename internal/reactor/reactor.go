package reactor

import (
	"NovaGate/internal/connection"
	"NovaGate/internal/router"
	"context"

	"github.com/cloudwego/netpoll"
)

// 负责网络 I/O 的接入
type Reactor struct {
	// 包装底层的 epoll/kqueue 事件循环，当有新的读写事件到来时，触发 handleRequest
	loop netpoll.EventLoop
	// 将固定的 router 变为一个获取 router 的函数指针
	getRouter func() *router.Router
}

func NewReactor(routerGetter func() *router.Router) (*Reactor, error) {
	reactor := &Reactor{
		getRouter: routerGetter,
	}
	loop, err := netpoll.NewEventLoop(reactor.handleRequest())
	if err != nil {
		return nil, err
	}
	reactor.loop = loop
	return reactor, nil
}

func (r *Reactor) Serve(listener netpoll.Listener) error {
	return r.loop.Serve(listener)
}

func (r *Reactor) handleRequest() netpoll.OnRequest {
	return func(ctx context.Context, conn netpoll.Connection) error {
		// 每次来新请求，都动态获取当前的最新路由树
		currentRouter := r.getRouter()
		// 直接在 Netpoll 调度的协程中执行业务
		// 这样只要 Process 不 return，底层就不会回收 Buffer，保证内存安全
		connection.Process(conn, currentRouter)
		return nil
	}
}
