
# AI API Gateway System Architecture

## Overview

This is an AI API gateway/proxy built with Go that aggregates 40+ upstream AI providers (OpenAI, Claude, Gemini, Azure, AWS Bedrock, etc.) behind a unified API, with user management, billing, rate limiting, and an admin dashboard.

---

## 1. System Architecture

### 1.1 Layered Architecture

The system adopts a classic **4-layer architecture**:

```
┌─────────────────────────────────────────────────────────────────┐
│  Router Layer       │  HTTP routing (API, Relay, Dashboard, Web)│
├─────────────────────────────────────────────────────────────────┤
│  Controller Layer   │  Request handlers, parameter validation   │
├─────────────────────────────────────────────────────────────────┤
│  Service Layer      │  Business logic (billing, channel selection)│
├─────────────────────────────────────────────────────────────────┤
│  Model Layer        │  Data models and DB access (GORM)         │
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 Module Responsibilities

| Module | Responsibility | Core Files |
|--------|----------------|------------|
| **router/** | HTTP route definitions | `api-router.go`, `relay-router.go`, `dashboard.go` |
| **controller/** | Request handlers | `relay.go`, `channel.go`, `user.go`, `billing.go` |
| **service/** | Business logic | `channel_select.go`, `billing.go`, `quota.go` |
| **model/** | Data models | `channel.go`, `user.go`, `pricing.go`, `token.go` |
| **relay/** | AI API relay | `relay_adaptor.go`, `channel/*` (40+ upstream adapters) |
| **middleware/** | Middleware | `auth.go`, `rate-limit.go`, `cors.go` |
| **setting/** | Configuration | `ratio_setting/`, `system_setting/` |
| **common/** | Utilities | `json.go`, `redis.go`, `rate-limit.go` |

---

## 2. Core Workflow

### 2.1 Relay Request Processing Flow

```
Client Request → Router → Middleware → Controller → Service → Relay → Upstream API
     ↓              ↓             ↓           ↓          ↓
  [Route Match]  [Auth/RateLimit] [Param Validate] [Channel Select] [Forward]
```

### 2.2 Detailed Flow (Take `/v1/chat/completions` as example)

1. **Router Layer**: `router/relay-router.go` matches `POST /v1/chat/completions`, calls `controller.Relay(c, types.RelayFormatOpenAI)`

2. **Middleware Layer**:
   - `TokenAuth()` - API key authentication
   - `ModelRequestRateLimit()` - Model-level rate limiting
   - `Distribute()` - Request distribution

3. **Controller Layer**: `controller/relay.go`
   - Request validation: `helper.GetAndValidateRequest()`
   - Relay info generation: `relaycommon.GenRelayInfo()`
   - Sensitive content check: `service.CheckSensitiveText()`
   - Token estimation: `service.EstimateRequestToken()`
   - Price calculation: `helper.ModelPriceHelper()`
   - Pre-consumption: `service.PreConsumeBilling()`

4. **Channel Selection**: `service/CacheGetRandomSatisfiedChannel()`
   - Supports cross-group retry mechanism

5. **Relay Forward**: `relay.TextHelper()` calls corresponding adaptor

6. **Response Handling**:
   - Success: `service.SettleBilling()`
   - Failure: Refund + Error handling + Auto-ban channel

---

## 3. Core Subsystems

### 3.1 Channel Management System

**Channel Adapter Architecture**:

```
relay/channel/
├── openai/         # OpenAI adapter
├── claude/         # Claude adapter  
├── gemini/         # Gemini adapter
├── aws/            # AWS Bedrock adapter
├── zhipu/          # Zhipu adapter
├── ali/            # Alibaba adapter
└── ...             # 40+ upstream adapters
```

**Adapter Interface**:
```go
type Adaptor interface {
    Completions(c *gin.Context, channel *model.Channel) error
    ChatCompletions(c *gin.Context, channel *model.Channel) error
    Embeddings(c *gin.Context, channel *model.Channel) error
}
```

### 3.2 Billing System

**Billing Flow**:
```
Pre-consumption → Request Execution → Settle/Refund
   ↓                 ↓                  ↓
[Freeze Quota]    [Actual Usage]    [Adjustment]
```

**Core Components**:
- `service/pre_consume_quota.go` - Pre-consumption logic
- `service/tiered_settle.go` - Tiered settlement
- `pkg/billingexpr/` - Billing expression engine

### 3.3 Rate Limiting System

**Multi-layer Rate Limiting**:

| Layer | Purpose | Location |
|-------|---------|----------|
| Global | System protection | `middleware/rate-limit.go` |
| Model | Per-model concurrency | `middleware/model-rate-limit.go` |
| User | Per-user/group | `service/quota.go` |

### 3.4 Authentication System

**Authentication Methods**:
- API Key Authentication (`middleware/auth.go`)
- OAuth 2.0 (GitHub, Discord, OIDC)
- WebAuthn/Passkeys (`service/passkey/`)

---

## 4. Data Flow & Context Management

### 4.1 Request Context Keys

| Key | Type | Purpose |
|-----|------|---------|
| `ContextKeyUserGroup` | string | User group |
| `ContextKeyOriginalModel` | string | Original model name |
| `ContextKeyChannelKey` | string | Channel API key |
| `ContextKeyRequestStartTime` | time.Time | Request start time |

### 4.2 Cache Strategy

**Multi-level Cache Architecture**:

```
┌─────────────────────┐
│   Memory Cache      │  ← High-frequency access
├─────────────────────┤
│    Redis Cache      │  ← Channel list, model pricing
├─────────────────────┤
│    Disk Cache       │  ← Large files (images, audio)
└─────────────────────┘
```

---

## 5. High Availability Design

### 5.1 Retry Mechanism

```go
func shouldRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) bool {
    if types.IsChannelError(err) { return true }
    if types.IsSkipRetryError(err) { return false }
    if retryTimes <= 0 { return false }
    return operation_setting.ShouldRetryByStatusCode(code)
}
```

### 5.2 Channel Auto-ban

```go
func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
    if service.ShouldDisableChannel(err) && channelError.AutoBan {
        gopool.Go(func() {
            service.DisableChannel(channelError, err.Error())
        })
    }
}
```

### 5.3 Distributed Deployment

- **Master-Slave**: Identified by `common.IsMasterNode`
- **Load Balancing**: Multi-node deployment support
- **Hot Config Update**: `model.SyncOptions()`

---

## 6. Tech Stack

| Layer | Technology |
|-------|------------|
| Backend | Go 1.22+, Gin, GORM v2 |
| Frontend | React 19, TypeScript, Rsbuild, Base UI, Tailwind CSS |
| Database | SQLite, MySQL, PostgreSQL |
| Cache | Redis + In-memory |
| Auth | JWT, WebAuthn, OAuth |

---

## 7. Key Features Summary

1. **Unified API Entry**: Aggregates 40+ upstream AI providers with OpenAI-compatible interface
2. **Flexible Channel Management**: Auto-selection, priority scheduling, cross-group retry
3. **Comprehensive Billing**: Pre-consumption, tiered settlement, dynamic pricing
4. **High Availability**: Auto-retry, channel ban, failover
5. **Multi-layer Security**: API auth, sensitive content detection, rate limiting

---

**Document Generated**: 2026-06-10
**Project**: new-api (AI API Gateway)
