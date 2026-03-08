package loadbalance

import (
	"NovaGate/internal/logger"
	"NovaGate/internal/upstream"
	"context"
	"net"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// LoadBalancer 负载均衡器 (挂载多个节点池)
type LoadBalancer struct {
	pools  []*upstream.HostPool
	next   uint32
	cancel context.CancelFunc // 用于 Nacos 重建路由时，销毁老的探活协程
}

func NewLoadBalancer(nodes []string) *LoadBalancer {
	// 创建带有取消功能的上下文
	ctx, cancel := context.WithCancel(context.Background())

	lb := &LoadBalancer{
		pools:  make([]*upstream.HostPool, len(nodes)),
		cancel: cancel,
	}

	for i, node := range nodes {
		// 为每个微服务节点维持最大 100 个长连接
		lb.pools[i] = upstream.NewHostPool(node, 100)
	}

	// 启动后台探活线程
	lb.startHealthCheck(ctx)

	return lb
}

// Select 轮询挑选一个存活的后端节点的连接池
func (lb *LoadBalancer) Select() *upstream.HostPool {
	length := uint32(len(lb.pools))
	if length == 0 {
		return nil
	}

	// 优化：最多尝试一圈，跳过所有 dead 节点
	for i := uint32(0); i < length; i++ {
		idx := atomic.AddUint32(&lb.next, 1)
		pool := lb.pools[idx%length]
		if pool.Alive.Load() { // 只返回健康的节点
			return pool
		}
	}
	return nil // 灾难：所有节点全挂了
}

// Destroy 销毁当前负载均衡器（防止 Nacos 热更新时后台协程堆积内存泄漏）
func (lb *LoadBalancer) Destroy() {
	if lb.cancel != nil {
		lb.cancel()
	}
}

// startHealthCheck 后台主动健康检查 (每 3 秒一次)
func (lb *LoadBalancer) startHealthCheck(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// 收到销毁指令，静默退出协程
				return
			case <-ticker.C:
				// 执行探活
				for _, p := range lb.pools {
					// 简单的 TCP 探活，超时 1 秒
					conn, err := net.DialTimeout("tcp", p.Addr, 1*time.Second)
					isAliveNow := err == nil

					if isAliveNow {
						conn.Close() // 探活成功，关掉测试连接
					}

					wasAlive := p.Alive.Load()

					// 状态发生跳变时才打日志，防止日志刷屏
					if wasAlive && !isAliveNow {
						logger.Log.Warn("❌ 节点宕机，自动摘除流量", zap.String("addr", p.Addr))
						p.Alive.Store(false)
					} else if !wasAlive && isAliveNow {
						logger.Log.Info("✅ 节点恢复，重新接入流量", zap.String("addr", p.Addr))
						p.Alive.Store(true)
					}
				}
			}
		}
	}()
}
