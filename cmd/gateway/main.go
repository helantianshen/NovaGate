package main

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/cloudwego/netpoll"
	"go.uber.org/zap"

	"NovaGate/internal/config"
	"NovaGate/internal/loadbalance"
	"NovaGate/internal/logger"
	"NovaGate/internal/pipline"
	"NovaGate/internal/reactor"
	"NovaGate/internal/router"
	"NovaGate/internal/upstream"
)

// 包装器：把路由树和它的销毁函数绑在一起
type RouterWrapper struct {
	Router  *router.Router
	Cleanup func() // 执行清理动作的函数
}

// 全局的动态路由树容器 (保证高并发下读写安全)
// 现在的全局变量存的是 *RouterWrapper
var globalRouter atomic.Value

// buildRouter 从 Nacos 配置中动态生成一棵全新的路由树
func buildRouter(cfg *config.GatewayConfig) (*router.Router, func()) {
	r := router.NewRouter()

	// 收集这棵树下所有 LoadBalancer 的销毁方法
	var cleanups []func()

	// 注册核心的系统接口 (不从 Nacos 读取)
	r.AddRoute("GET", "/ping", func(c *pipline.Context) {
		logger.Log.Debug("ping")
		c.String(200, "pong")
	})

	// 根据 Nacos 配置，动态注册业务路由和负载均衡池
	for _, routeRule := range cfg.Routes {
		// 为每条路由独立生成一个负载均衡池
		pool := loadbalance.NewLoadBalancer(routeRule.Backends)
		stripPrefix := routeRule.StripPrefix

		// 把当前池子的销毁动作，注册到闭包数组里
		cleanups = append(cleanups, pool.Destroy)

		// 将 rule 变量捕获到 Handler 中
		r.AddRoute(routeRule.Method, routeRule.Path,
			pipline.AsyncLoggerMiddleware(), // 异步记录日志
			pipline.RateLimitMiddleware(),   // 动态读取配置的限流器
			pipline.AuthMiddleware(),        // 鉴权器
			func(c *pipline.Context) {
				// 动态剥离前缀
				if stripPrefix != "" {
					c.Path = strings.TrimPrefix(c.Path, stripPrefix)
					if c.Path == "" {
						c.Path = "/"
					}
				}

				// 动态负载均衡
				// 这里拿到的不再是 IP 字符串，而是一整个长连接池
				targetPool := pool.Select()
				if targetPool == nil {
					c.String(503, "Service Unavailable: No backends")
					return
				}

				// 透传给后端
				// 把连接池对象传给代理引擎
				upstream.ForwardTo(c, targetPool)
			},
		)
		logger.Log.Info("[Router Rebuild] 挂载路由成功:",
			zap.String("route_rule_path", routeRule.Path),
			zap.Strings("route_rule_backends", routeRule.Backends))
	}

	// 组装最终的清理函数：遍历执行所有的 Destroy
	cleanupFunc := func() {
		for _, fn := range cleanups {
			fn()
		}
	}

	return r, cleanupFunc
}

func main() {

	// 初始化日志
	// 平时开发传 "info"，如果跑极限压测，改成 "error"
	logger.Init("debug")
	defer logger.Sync() // 优雅退出时刷盘

	logger.Log.Debug("网关日志系统初始化成功", zap.String("version", "v1.0"))

	// 初始化 Nacos
	err := config.InitNacos("127.0.0.1", 8848)
	if err != nil {
		logger.Log.Fatal("Nacos 启动失败", zap.Error(err)) // 遇错直接 Fatal 退出
	}

	// 首次启动，根据基础配置生成初始路由树
	initialConfig := config.Manager.GetConfig()
	r, cleanup := buildRouter(initialConfig)
	globalRouter.Store(&RouterWrapper{
		Router:  r,
		Cleanup: cleanup,
	})

	// 注册 Nacos 配置热更新时的回调
	config.Manager.OnChange = func(newCfg *config.GatewayConfig) {
		logger.Log.Info("[Gateway] 监测到 Nacos 路由配置变更，开始重建路由树和连接池...")

		// 构建新树和新树的清理函数
		newRouter, newCleanup := buildRouter(newCfg)
		newWrapper := &RouterWrapper{
			Router:  newRouter,
			Cleanup: newCleanup,
		}

		// 拿到老树的 Wrapper
		oldWrapper := globalRouter.Load().(*RouterWrapper)

		// 原子操作：瞬间把网关的路由树替换为新树
		globalRouter.Store(newWrapper)
		logger.Log.Info("🚀 路由树无缝热更新完毕！新流量已切入。")

		// 优雅降级
		// 此刻老树可能还有正在处理中的请求
		// 3秒后再彻底销毁老树的后台探活协程
		time.AfterFunc(3*time.Second, func() {
			if oldWrapper.Cleanup != nil {
				oldWrapper.Cleanup()
				logger.Log.Info("♻️ 老路由树的连接池探活协程已成功回收，防止内存泄漏！")
			}
		})
	}

	// 启动网络层
	// 创建 Listener
	listener, err := netpoll.CreateListener("tcp", ":8080")
	if err != nil {
		logger.Log.Fatal("端口监听失败", zap.Error(err))
	}

	// 创建 Reactor
	// 传递一个获取全局路由树的闭包函数给 Reactor
	netReactor, err := reactor.NewReactor(func() *router.Router {
		return globalRouter.Load().(*RouterWrapper).Router
	})
	if err != nil {
		logger.Log.Fatal("Reactor 创建失败", zap.Error(err))
	}

	logger.Log.Info("🚀 Gateway is running", zap.String("port", ":8080"))
	err = netReactor.Serve(listener)
	if err != nil {
		logger.Log.Fatal("Reactor 运行异常", zap.Error(err))
	}
}
