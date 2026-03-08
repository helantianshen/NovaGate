package config

import (
	"NovaGate/internal/logger"
	"encoding/json"
	"sync/atomic"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"go.uber.org/zap"
)

// 通过Go标准库 sync/atomic.Value 实现 RCU (Read-Copy-Update) 无锁热更新
// 读（Read）：网关的 Worker 协程在处理请求时，通过 atomic.Value.Load() 直接拿到配置的只读副本，全程无锁，性能极高
// 写（Copy-Update）：当 Nacos 推送了新配置时，后台协程解析这段 JSON，构建出一个全新的配置对象
// 然后通过 atomic.Value.Store() 原子性地替换掉老对象，正在处理中的老请求继续用老配置，新进来的请求会立刻使用新配置

// 路由
type RouteRule struct {
	Method      string   `json:"method"`       // 例如: "GET"
	Path        string   `json:"path"`         // 例如: "/api/v1/order/:id"
	StripPrefix string   `json:"strip_prefix"` // 例如: "/api/v1"
	Backends    []string `json:"backends"`     // 例如: ["127.0.0.1:9091", "127.0.0.1:9092"]
}

// GatewayConfig 定义了网关的动态配置结构
// 我们用 JSON tag 来映射 Nacos 里的内容
type GatewayConfig struct {
	RateLimit struct {
		Enabled bool `json:"enabled"`
		QPS     int  `json:"qps"`
	} `json:"rate_limit"`

	Routes []RouteRule `json:"routes"` // 挂载动态路由表
}

// ConfigManager 负责管理动态配置
type ConfigManager struct {
	// currentConfig 存储 *GatewayConfig，使用 RCU 无锁机制
	currentConfig atomic.Value
	// OnChange 配置变更时的回调函数
	OnChange func(newCfg *GatewayConfig)
}

var (
	// 全局单例管理器
	Manager *ConfigManager
)

// InitNacos 初始化 Nacos 客户端并开启监听
func InitNacos(nacosAddr string, port uint64) error {
	Manager = &ConfigManager{}

	// 初始化一个默认配置，防止 Nacos 还没连上时网关报错
	defaultConfig := &GatewayConfig{}
	defaultConfig.RateLimit.Enabled = false
	defaultConfig.RateLimit.QPS = 1000
	Manager.currentConfig.Store(defaultConfig)

	// 配置 Nacos 服务端和客户端参数
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr: nacosAddr,
			Port:   port,
		},
	}
	clientConfig := constant.ClientConfig{
		NamespaceId:         "", // 默认 public 命名空间
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "/tmp/nacos/log",
		CacheDir:            "/tmp/nacos/cache",
		LogLevel:            "info",
	}

	// 创建动态配置客户端 (ConfigClient)
	configClient, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return err
	}

	dataId := "gateway_config.json"
	group := "DEFAULT_GROUP"

	// 启动时主动拉取一次配置
	content, err := configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err == nil && len(content) > 0 {
		Manager.updateConfig(content)
		logger.Log.Info("[Config] 成功从 Nacos 拉取初始配置")
	} else {
		logger.Log.Debug("[Config] Nacos 初始配置为空或拉取失败，使用默认配置")
	}

	// 开启配置变更监听
	err = configClient.ListenConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
		OnChange: func(namespace, group, dataId, data string) {
			logger.Log.Info("[Config] ⚡ 收到 Nacos 配置更新推送！")
			Manager.updateConfig(data)
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// updateConfig 将 Nacos 推送来的 JSON 字符串解析并原子替换
func (m *ConfigManager) updateConfig(content string) {
	newConfig := &GatewayConfig{}
	err := json.Unmarshal([]byte(content), newConfig)
	if err != nil {
		logger.Log.Error("[Config] ❌ Nacos 配置解析失败，保持旧配置不变: %v\n", zap.Error(err))
		return
	}

	// RCU 核心操作：原子替换
	m.currentConfig.Store(newConfig)
	logger.Log.Info("[Config] ✅ 网关配置热更新成功！",
		zap.Bool("rate_limit_enabled", newConfig.RateLimit.Enabled),
		zap.Int("rate_limit_qps", newConfig.RateLimit.QPS),
	)

	// 如果外部注册了回调函数，触发它去重建路由树！
	if m.OnChange != nil {
		m.OnChange(newConfig)
	}
}

// GetConfig 供网关工作协程获取当前最新的配置 (无锁极速读取)
func (m *ConfigManager) GetConfig() *GatewayConfig {
	return m.currentConfig.Load().(*GatewayConfig)
}
