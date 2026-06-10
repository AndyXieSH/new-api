
# New API 开发者指南

## 概述

New API 是一个基于 Go 构建的 AI API 网关/代理，聚合了 40+ 上游 AI 提供商，提供统一的 API 接口。本文档旨在帮助开发者理解项目结构、进行二次开发和扩展。

---

## 1. 项目结构

```
new-api/
├── main.go                    # 入口文件
├── go.mod                     # Go 模块依赖
├── go.sum                     # 依赖校验文件
├── .env.example               # 环境变量示例
├── docker-compose.yml         # Docker Compose 配置
├── Dockerfile                 # Docker 构建文件
├── README.md                  # 项目说明
├── AGENTS.md                  # 项目约定规范
├── docs/                      # 文档目录
├── router/                    # 路由层
├── controller/                # 控制器层
├── service/                   # 服务层
├── model/                     # 数据模型层
├── relay/                     # AI API 中继层
│   └── channel/               # 渠道适配器（40+上游）
├── middleware/                # 中间件
├── setting/                   # 配置管理
├── common/                    # 通用工具
├── constant/                  # 常量定义
├── dto/                       # 数据传输对象
├── types/                     # 类型定义
├── oauth/                     # OAuth 实现
├── pkg/                       # 内部包
├── logger/                    # 日志模块
├── i18n/                      # 国际化支持
└── web/                       # 前端资源
    ├── default/               # 默认前端（React 19）
    └── classic/               # 经典前端（React 18）
```

### 1.1 模块职责说明

| 模块 | 职责 | 说明 |
|------|------|------|
| **router/** | HTTP路由定义 | 定义 API、Relay、Dashboard、Web 等路由 |
| **controller/** | 请求处理 | 参数校验、响应封装、调用 Service 层 |
| **service/** | 业务逻辑 | 计费、渠道选择、配额管理等核心业务 |
| **model/** | 数据访问 | GORM 数据模型、数据库操作 |
| **relay/** | API中继 | 上游渠道适配、请求转发、响应处理 |
| **middleware/** | 中间件 | 认证、限流、CORS、日志等 |
| **setting/** | 配置管理 | 动态配置、设置项管理 |
| **common/** | 工具函数 | JSON、Redis、加密、验证等 |

---

## 2. 开发环境设置

### 2.1 环境要求

| 依赖 | 版本 | 说明 |
|------|------|------|
| Go | ≥ 1.22 | 后端开发语言 |
| Bun | latest | 前端包管理器 |
| MySQL | ≥ 5.7.8 | 可选数据库 |
| PostgreSQL | ≥ 9.6 | 可选数据库 |
| SQLite | - | 默认数据库（内置） |
| Redis | - | 缓存（可选） |

### 2.2 快速开始

```bash
# 1. 克隆项目
git clone https://github.com/QuantumNous/new-api.git
cd new-api

# 2. 复制环境变量配置
cp .env.example .env

# 3. 安装依赖
go mod download

# 4. 运行项目（使用 SQLite）
go run main.go

# 5. 访问
# 前端: http://localhost:3000
# API: http://localhost:3000/v1/chat/completions
```

### 2.3 环境变量配置

关键环境变量说明：

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `PORT` | 服务端口 | 3000 |
| `SQL_DSN` | MySQL/PostgreSQL 连接字符串 | - |
| `REDIS_CONN_STRING` | Redis 连接字符串 | - |
| `SESSION_SECRET` | 会话密钥（多机部署必填） | - |
| `CRYPTO_SECRET` | 加密密钥（Redis 必填） | - |
| `DEBUG` | 调试模式 | false |
| `MEMORY_CACHE_ENABLED` | 启用内存缓存 | false |

---

## 3. 核心工作流程

### 3.1 Relay 请求处理流程

```
客户端请求 → Router → Middleware → Controller → Service → Relay → 上游API
     ↓              ↓             ↓           ↓          ↓
  [路由匹配]   [认证/限流]    [参数校验]   [渠道选择]   [请求转发]
```

### 3.2 详细流程解析

1. **路由层** (`router/relay-router.go`)
   - 匹配请求路径，分发到对应的控制器
   - 支持 OpenAI、Claude、Gemini 等多种 API 格式

2. **中间件层**
   - `TokenAuth()`: API 密钥认证
   - `ModelRequestRateLimit()`: 模型级限流
   - `Distribute()`: 请求分发

3. **控制器层** (`controller/relay.go`)
   - 请求验证和参数解析
   - 敏感词检测
   - Token 预估和价格计算
   - 预扣费处理

4. **服务层** (`service/channel_select.go`)
   - 根据 Token 分组、模型名选择合适的渠道
   - 支持跨分组重试机制

5. **中继层** (`relay/`)
   - 根据渠道类型调用对应的适配器
   - 请求格式转换和转发
   - 响应处理和错误重试

---

## 4. API 使用

### 4.1 基础认证

```bash
# 使用 API Key 认证
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

### 4.2 支持的 API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/models` | GET | 获取模型列表 |
| `/v1/chat/completions` | POST | 对话补全 |
| `/v1/completions` | POST | 文本补全 |
| `/v1/embeddings` | POST | 嵌入向量 |
| `/v1/images/generations` | POST | 图片生成 |
| `/v1/audio/transcriptions` | POST | 语音转文字 |
| `/v1/audio/speech` | POST | 文字转语音 |
| `/v1/rerank` | POST | 排序重排 |

### 4.3 模型名称映射

系统支持模型名称映射，可以通过管理后台配置。例如：
- `gpt-3.5-turbo` → 映射到实际的上游模型
- `claude-3-sonnet` → 映射到 Claude 模型

---

## 5. 扩展开发

### 5.1 添加新的渠道适配器

#### 步骤 1：创建渠道目录

```bash
mkdir -p relay/channel/myprovider
```

#### 步骤 2：实现适配器接口

```go
// relay/channel/myprovider/adaptor.go
package myprovider

import (
    "github.com/QuantumNous/new-api/model"
    "github.com/gin-gonic/gin"
)

type Adaptor struct{}

// 实现 channel.Adaptor 接口
func (a *Adaptor) ChatCompletions(c *gin.Context, channel *model.Channel) error {
    // 实现请求转发逻辑
    return nil
}

func (a *Adaptor) Completions(c *gin.Context, channel *model.Channel) error {
    return nil
}

func (a *Adaptor) Embeddings(c *gin.Context, channel *model.Channel) error {
    return nil
}
```

#### 步骤 3：注册适配器

```go
// relay/relay_adaptor.go
func GetAdaptor(apiType int) channel.Adaptor {
    switch apiType {
    // ... 其他 case
    case constant.APITypeMyProvider:
        return &myprovider.Adaptor{}
    }
    return nil
}
```

#### 步骤 4：添加常量定义

```go
// constant/api_type.go
const (
    // ... 其他常量
    APITypeMyProvider = iota + 100
)
```

### 5.2 添加新的 API 端点

1. 在 `router/` 目录添加路由
2. 在 `controller/` 目录创建处理函数
3. 在 `service/` 目录实现业务逻辑（如需要）

---

## 6. 测试与调试

### 6.1 运行测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./relay/...

# 运行带覆盖率的测试
go test -cover ./...
```

### 6.2 启用调试模式

```bash
# 设置环境变量
export DEBUG=true
export ENABLE_PPROF=true

# 运行项目
go run main.go

# 访问 pprof
# http://localhost:8005/debug/pprof/
```

### 6.3 日志查看

```bash
# 查看系统日志
tail -f /path/to/logs/app.log

# 查看错误日志（需启用 ERROR_LOG_ENABLED）
tail -f /path/to/logs/error.log
```

---

## 7. 部署指南

### 7.1 Docker 部署

```bash
# 使用 Docker Compose
docker-compose up -d

# 或使用 Docker 命令
docker run --name new-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  calciumion/new-api:latest
```

### 7.2 多机部署注意事项

1. **必须设置** `SESSION_SECRET` - 确保登录状态一致
2. **必须设置** `CRYPTO_SECRET` - 确保 Redis 数据可解密
3. **推荐使用** Redis 作为共享缓存
4. **数据库** 使用 MySQL 或 PostgreSQL（不推荐 SQLite）

---

## 8. 常见问题

### Q1: 如何添加新的 AI 模型？

A: 在管理后台的「模型管理」中添加模型配置，设置对应的渠道和价格。

### Q2: 如何配置渠道优先级？

A: 在管理后台的「渠道管理」中设置渠道的优先级数值，数值越小优先级越高。

### Q3: 如何启用 Redis 缓存？

A: 设置环境变量 `REDIS_CONN_STRING=redis://localhost:6379/0`

### Q4: 如何自定义定价策略？

A: 参考 `pkg/billingexpr/expr.md` 了解计费表达式语法，在管理后台配置。

---

## 9. 贡献指南

欢迎贡献代码！请遵循以下流程：

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/my-feature`)
3. 提交代码 (`git commit -m "Add my feature"`)
4. 推送到分支 (`git push origin feature/my-feature`)
5. 创建 Pull Request

### 代码规范

- 遵循 Go 官方代码风格
- 使用 `go fmt` 格式化代码
- 添加必要的测试用例
- 确保所有测试通过

---

## 10. 参考资源

- **官方文档**: https://docs.newapi.pro
- **API 文档**: https://docs.newapi.pro/en/docs/api
- **Issue 反馈**: https://github.com/QuantumNous/new-api/issues
- **社区交流**: https://docs.newapi.pro/en/docs/support/community-interaction

---

**文档版本**: v1.0  
**生成时间**: 2026-06-10  
**项目**: new-api (AI API Gateway)
