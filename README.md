# 🚀 NovaGate: High-Performance Cloud-Native API Gateway

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg)
![Architecture](https://img.shields.io/badge/Architecture-Reactor-success)

NovaGate 是一个基于 Go 语言和 [CloudWeGo Netpoll](https://github.com/cloudwego/netpoll) 打造的超高性能、支持动态热更新的云原生 API 网关。它完全摒弃了传统的标准库 `net/http`，深入底层实现了真正的零拷贝 (Zero-Copy) 代理和无锁 (Lock-Free) 架构。

## ✨ 核心特性 (Features)

- **⚡ 极致性能底座**: 基于 Netpoll Reactor 模型，采用 Epoll 驱动和 Nocopy Buffer，微基准测试 (Ping) 单机高达 **47,000+ QPS**。
- **🔄 RCU 无锁热更新**: 深度集成 Nacos 作为控制面 (Control Plane)。支持路由规则、限流阈值等配置的毫秒级热重载，**修改配置零停机、零报错**。
- **🛡️ 工业级高可用**: 
  - 内置 Upstream 长连接池，告别高并发下的 `TIME_WAIT` 爆炸。
  - 支持 L7 HTTP 报文边界精准解析。
  - 主动心跳探活 (Active Health Check) 与故障节点平滑摘除，自带旧路由树协程优雅回收 (Graceful Shutdown)，杜绝内存泄漏。
- **⛓️ 洋葱模型责任链**: 类似 Gin 的 Pipeline 架构，内置动态限流 (Token Bucket)、Auth 鉴权中间件。
- **📊 极速可观测性**: 接入 Uber Zap 强类型结构化日志，实现全链路全异步零内存分配记录。

## 🏗️ 架构设计 (Architecture)

NovaGate 严格遵循数据面 (Data Plane) 与控制面 (Control Plane) 分离的设计哲学：
- **数据面**: 负责极致的流量转发，依靠全局原子的 `RouterWrapper` 维护基于 Trie 树的动态路由。
- **控制面**: 监听 Nacos 长轮询，触发配置变更时，在后台构建新树并一键原子替换 (Store)，旧树延迟回收。

## 🚀 性能压测 (Benchmark)

在单台普通云服务器 (4C 8G) 上的真实压测数据：

| 测试场景 | 并发数 (Concurrency) | 请求总数 (Requests) | 成功率 | 平均 QPS |
| :--- | :--- | :--- | :--- | :--- |
| **网关纯路由 (Ping)** | 100 | 200,000 | 100% | **47,293 req/s** |
| **全链路代理透传 (L7)** | 1000 | 500,000 | 100% | **20,276 req/s** |

> *注：全链路压测包含完整的 鉴权 -> 前缀剥离 -> 长连接池获取 -> 真实 Go 后端节点响应 -> 零拷贝回写。*

## 📦 快速开始 (Quick Start)

### 1. 依赖环境
- Go 1.21+
- Nacos 2.x (可通过 Docker 快速启动)

### 2. 启动 Nacos 并配置
在 Nacos 控制台 (`http://localhost:8848/nacos`) 创建 `gateway-config.json`：
```json
{
  "rate_limit": { "enabled": true, "qps": 500 },
  "routes": [
    {
      "method": "GET",
      "path": "/api/v1/order/:id",
      "strip_prefix": "/api/v1",
      "backends": ["127.0.0.1:9091", "127.0.0.1:9092"]
    }
  ]
}
```

### 3. 运行网关
```Bash
go mod tidy
go run cmd/gateway/main.go
```
