package loadbalance

import (
	"NovaGate/internal/config"
	"NovaGate/internal/logger"
	"NovaGate/internal/upstream"
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"go.uber.org/zap"
)

// LoadBalancer 负载均衡器 (挂载多个节点池)
type LoadBalancer struct {
	serviceName string
	pools       atomic.Value // 🌟 核心升级：通过 atomic.Value 存储 []*upstream.HostPool 实现无锁热更新
	next        uint32
	cancel      context.CancelFunc // 用于销毁后台探活协程
}

// NewLoadBalancer 现在同时接收服务名和静态降级 IP 列表
func NewLoadBalancer(serviceName string, staticBackends []string) *LoadBalancer {
	ctx, cancel := context.WithCancel(context.Background())

	lb := &LoadBalancer{
		serviceName: serviceName,
		cancel:      cancel,
	}

	initialPools := make([]*upstream.HostPool, 0)

	// ==========================================
	// 🌟 动态服务发现逻辑
	// ==========================================
	if serviceName != "" && config.NamingClient != nil {
		// 1. 网关启动时，先主动拉取一次健康的实例列表
		instances, err := config.NamingClient.SelectInstances(vo.SelectInstancesParam{
			ServiceName: serviceName,
			GroupName:   "DEFAULT_GROUP",
			HealthyOnly: true, // 只拉取健康的实例
		})
		if err == nil && len(instances) > 0 {
			for _, inst := range instances {
				addr := fmt.Sprintf("%s:%d", inst.Ip, inst.Port)
				initialPools = append(initialPools, upstream.NewHostPool(addr, 100))
			}
		}

		// 2. 异步订阅 Nacos 节点变更推送
		err = config.NamingClient.Subscribe(&vo.SubscribeParam{
			ServiceName: serviceName,
			GroupName:   "DEFAULT_GROUP",
			SubscribeCallback: func(services []model.Instance, err error) {
				if err != nil {
					logger.Log.Error("订阅 Nacos 服务异常", zap.Error(err))
					return
				}

				logger.Log.Info("⚡ 收到下游服务节点变更推送", zap.String("service", serviceName), zap.Int("nodes", len(services)))

				var newPools []*upstream.HostPool
				for _, inst := range services {
					if inst.Healthy && inst.Enable {
						addr := fmt.Sprintf("%s:%d", inst.Ip, inst.Port)
						newPools = append(newPools, upstream.NewHostPool(addr, 100))
					}
				}

				// 拿到老池子用于后续清理
				oldPools := lb.pools.Load().([]*upstream.HostPool)

				// RCU 无锁替换：瞬间切入新的连接池组！
				lb.pools.Store(newPools)

				// 优雅降级：延迟 3 秒关闭老池子，让没跑完的请求跑完
				time.AfterFunc(3*time.Second, func() {
					for _, p := range oldPools {
						logger.Log.Debug("清理下线节点连接池", zap.String("addr", p.Addr))
						// 此处依赖底层的 Keep-Alive 超时机制或者 GC 回收
						// 严谨的做法是在 pool 里加个 Close 方法把 channel 里的连接全关了
					}
				})
			},
		})
		if err != nil {
			logger.Log.Error("订阅服务失败", zap.Error(err))
		}
	} else {
		// 降级：如果没有配置 service_name，退化为使用 json 里写死的静态 backends
		for _, node := range staticBackends {
			initialPools = append(initialPools, upstream.NewHostPool(node, 100))
		}
	}

	lb.pools.Store(initialPools)
	lb.startHealthCheck(ctx)

	return lb
}

// Select 轮询挑选一个存活的后端节点的连接池
func (lb *LoadBalancer) Select() *upstream.HostPool {
	// 每次调用时，极速拉取当前最新的 slice 快照
	pools := lb.pools.Load().([]*upstream.HostPool)
	length := uint32(len(pools))
	if length == 0 {
		return nil
	}

	for i := uint32(0); i < length; i++ {
		idx := atomic.AddUint32(&lb.next, 1)
		pool := pools[idx%length]
		if pool.Alive.Load() {
			return pool
		}
	}
	return nil
}

// Destroy 销毁当前负载均衡器
func (lb *LoadBalancer) Destroy() {
	if lb.cancel != nil {
		lb.cancel()
	}
	// 🌟 极端严谨：路由树被重构时，必须取消老的 Nacos 订阅，防止内存泄漏！
	if lb.serviceName != "" && config.NamingClient != nil {
		config.NamingClient.Unsubscribe(&vo.SubscribeParam{
			ServiceName: lb.serviceName,
			GroupName:   "DEFAULT_GROUP",
		})
	}
}

func (lb *LoadBalancer) startHealthCheck(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 获取最新的池子进行探活
				pools := lb.pools.Load().([]*upstream.HostPool)
				for _, p := range pools {
					conn, err := net.DialTimeout("tcp", p.Addr, 1*time.Second)
					isAliveNow := err == nil
					if isAliveNow {
						conn.Close()
					}
					wasAlive := p.Alive.Load()

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
