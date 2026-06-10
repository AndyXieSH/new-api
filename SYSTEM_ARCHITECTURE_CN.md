
# AI API 网关系统架构

## 概述

这是一个基于 Go 构建的 AI API 网关/代理，聚合了 40+ 上游 AI 提供商（OpenAI、Claude、Gemini、Azure、AWS Bedrock 等），提供统一的 API 接口，并包含用户管理、计费、限流和管理仪表板功能。

---

## 1. 系统架构

### 1.1 分层架构

系统采用经典的 **四层架构**：

```
┌─────────────────────────────────────────────────────────────────┐
│  Router 层          │  HTTP路由分发（API、Relay、Dashboard、Web）        │
├─────────────────────────────────────────────────────────────────┤
│  Controller 层      │  请求处理、参数校验、响应封装                    │
├─────────────────────────────────────────────────────────────────┤
│  Service 层         │  业务逻辑（计费、渠道选择、配额管理等）            │
├─────────────────────────────────────────────────────────────────┤
│  Model 层           │  数据模型与数据库访问（GORM）                     │
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 模块职责

| 模块 | 职责 | 核心文件 |
|------|------|----------|
| **router/** | HTTP路由定义 | `api-router.go`, `relay-router.go`, `dashboard.go` |
| **controller/** | 请求处理器 | `relay.go`, `channel.go`, `user.go`, `billing.go` |
| **service/** | 业务逻辑 | `channel_select.go`, `billing.go`, `quota.go` |
| **model/** | 数据模型 | `channel.go`, `user.go`, `pricing.go`, `token.go` |
| **relay/** | AI API中继 | `relay_adaptor.go`, `channel/*`（40+上游适配器） |
| **middleware/** | 中间件 | `auth.go`, `rate-limit.go`, `cors.go` |
| **setting/** | 配置管理 | `ratio_setting/`, `system_setting/` |
| **common/** | 通用工具 | `json.go`, `redis.go`, `rate-limit.go` |

---

## 2. 核心工作流程

### 2.1 Relay 请求处理流程

```
客户端请求 → Router → Middleware → Controller → Service → Relay → 上游API
     ↓              ↓             ↓           ↓          ↓
  [路由匹配]   [认证/限流]    [参数校验]   [渠道选择]   [请求转发]
```

### 2.2 详细流程（以 `/v1/chat/completions` 为例）

1. **路由层**：`router/relay-router.go` 匹配到 `POST /v1/chat/completions`，调用 `controller.Relay(c, types.RelayFormatOpenAI)`

2. **中间件层**：依次经过
   - `TokenAuth()` - API密钥认证
   - `ModelRequestRateLimit()` - 模型级限流
   - `Distribute()` - 请求分发

3. **控制器层**：`controller/relay.go`
   - 请求验证：`helper.GetAndValidateRequest()`
   - 生成中继信息：`relaycommon.GenRelayInfo()`
   - 敏感词检测：`service.CheckSensitiveText()`
   - Token预估：`service.EstimateRequestToken()`
   - 价格计算：`helper.ModelPriceHelper()`
   - 预扣费：`service.PreConsumeBilling()`

4. **渠道选择**：`service/CacheGetRandomSatisfiedChannel()`
   - 支持跨分组重试机制

5. **中继转发**：`relay.TextHelper()` 调用对应的适配器

6. **响应处理**：
   - 成功：结算账单 `service.SettleBilling()`
   - 失败：退款 + 错误处理 + 渠道自动封禁

---

## 3. 核心子系统

### 3.1 渠道管理系统

**渠道适配器架构**：

```
relay/channel/
├── openai/         # OpenAI 适配器
├── claude/         # Claude 适配器  
├── gemini/         # Gemini 适配器
├── aws/            # AWS Bedrock 适配器
├── zhipu/          # 智谱 适配器
├── ali/            # 阿里云 适配器
└── ...             # 40+ 上游适配器
```

**适配器接口**：
```go
type Adaptor interface {
    Completions(c *gin.Context, channel *model.Channel) error
    ChatCompletions(c *gin.Context, channel *model.Channel) error
    Embeddings(c *gin.Context, channel *model.Channel) error
}
```

### 3.2 计费系统

**计费流程**：
```
预扣费 → 请求执行 → 结算/退款
   ↓          ↓          ↓
[冻结配额]  [实际消耗]  [多退少补]
```

**核心组件**：
- `service/pre_consume_quota.go` - 预扣费逻辑
- `service/tiered_settle.go` - 分层结算
- `pkg/billingexpr/` - 计费表达式引擎

### 3.3 限流系统

**多层限流机制**：

| 限流层级 | 作用 | 实现位置 |
|----------|------|----------|
| 全局限流 | 系统级保护 | `middleware/rate-limit.go` |
| 模型限流 | 单模型并发控制 | `middleware/model-rate-limit.go` |
| 用户限流 | 按用户/分组限制 | `service/quota.go` |

### 3.4 认证系统

**多种认证方式**：
- API密钥认证（`middleware/auth.go`）
- OAuth 2.0（GitHub、Discord、OIDC等）
- WebAuthn/Passkeys（`service/passkey/`）

---

## 4. 数据流与状态管理

### 4.1 请求上下文传递

通过 Gin Context 传递请求生命周期数据：

| Context Key | 类型 | 用途 |
|-------------|------|------|
| `ContextKeyUserGroup` | string | 用户分组 |
| `ContextKeyOriginalModel` | string | 原始模型名 |
| `ContextKeyChannelKey` | string | 渠道密钥 |
| `ContextKeyRequestStartTime` | time.Time | 请求开始时间 |

### 4.2 缓存策略

**多级缓存架构**：

```
┌─────────────────────┐
│   内存缓存          │  ← 高频访问数据
├─────────────────────┤
│    Redis 缓存       │  ← 渠道列表、模型定价
├─────────────────────┤
│    磁盘缓存         │  ← 大文件（图片、音频）
└─────────────────────┘
```

---

## 5. 高可用设计

### 5.1 重试机制

```go
func shouldRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) bool {
    if types.IsChannelError(err) { return true }      // 渠道错误重试
    if types.IsSkipRetryError(err) { return false }   // 跳过重试标记
    if retryTimes <= 0 { return false }               // 重试次数耗尽
    return operation_setting.ShouldRetryByStatusCode(code)
}
```

### 5.2 渠道自动封禁

```go
func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
    if service.ShouldDisableChannel(err) && channelError.AutoBan {
        gopool.Go(func() {
            service.DisableChannel(channelError, err.Error())
        })
    }
}
```

### 5.3 分布式部署支持

- **主从节点**：通过 `common.IsMasterNode` 标识
- **负载均衡**：支持多节点部署
- **配置热更新**：定时同步配置 `model.SyncOptions()`

---

## 6. 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.22+, Gin, GORM v2 |
| 前端 | React 19, TypeScript, Rsbuild, Base UI, Tailwind CSS |
| 数据库 | SQLite, MySQL, PostgreSQL |
| 缓存 | Redis + 内存缓存 |
| 认证 | JWT, WebAuthn, OAuth |

---

## 7. 关键特性总结

1. **统一API入口**：聚合40+上游AI提供商，对外提供OpenAI兼容接口
2. **灵活的渠道管理**：支持自动选择、优先级调度、跨分组重试
3. **完善的计费系统**：支持预扣费、分层结算、动态定价表达式
4. **高可用设计**：自动重试、渠道封禁、故障转移
5. **多层安全防护**：API认证、敏感词检测、请求限流

---

**文档生成时间**：2026-06-10
**项目**：new-api (AI API Gateway)
