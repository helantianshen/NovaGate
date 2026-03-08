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

### 第二部分：高含金量简历包装

在你的简历**【项目经历】**模块，绝不能只写“实现了一个网关”。大厂面试官看重的是**你解决了什么复杂问题**。你需要用到我们在踩坑过程中积累的硬核概念。

你可以这样写：

#### **项目名称：NovaGate - 高性能云原生微服务网关 (Go 语言)**
**项目描述：**
从零独立开发的一款基于 Reactor 模型和 RCU 机制的高并发微服务 API 网关，旨在解决传统 `net/http` 网关在海量连接下的性能瓶颈及配置变更导致的服务抖动问题。

**核心贡献与技术挑战：**
* **极致网络底座与零拷贝**：弃用标准库，基于 `CloudWeGo/Netpoll` 封装底层网络 Reactor 模型。通过自定义 HTTP 解析器接管 L7 报文边界，利用 `WriteBinary` 与 `LinkBuffer` 实现 Upstream 代理层的全链路 Zero-Copy (零拷贝) 数据透传。压测环境下单机全链路转发性能达 **20,000+ QPS**，`/ping` 路由达 **47,000+ QPS**。
* **无锁热更新 (RCU) 与协程防泄漏**：集成 Nacos 作为动态控制面，利用 `sync/atomic.Value` 实现路由树、限流阈值的 RCU (Read-Copy-Update) 毫秒级无锁替换。独创 `RouterWrapper` 包装器，结合延迟回收机制 (Graceful Shutdown) 掐断废弃探活协程，彻底杜绝了动态重载过程中的 Goroutine 泄漏。
* **高可用长连接池与探活熔断**：重构反向代理模块，设计并发安全的 `HostPool` 连接池复用后端长连接，避免高并发引发的 `TIME_WAIT` 端口耗尽。实现 Active Health Check 机制，当微服务节点宕机时触发毫秒级平滑熔断摘除，节点恢复后自动重连，实现流量转发 0 报错。
* **高性能组件工程化**：设计类似 Gin 的 `Context` 责任链 (Pipeline) 架构，集成对象池 (`sync.Pool`) 实现 Context 零内存分配；引入 `Uber Zap` 强类型结构化日志引擎替换标准库 log，消除日志落盘造成的全局锁阻塞与 OOM 隐患。
