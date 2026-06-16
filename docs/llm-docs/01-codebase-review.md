# New API — 代码库梳理报告

> 生成时间：2026-06-16
> 工具：基于代码静态分析，未运行任何服务

---

## 1. 项目概况

| 项目 | 详情 |
|---|---|
| **名称** | New API (`new-api`) |
| **定位** | 下一代 LLM 网关 & AI 资产管理系统（聚合 40+ 上游 AI 提供商的统一 API 网关） |
| **版本** | 读取自 `VERSION` 文件（当前为空，CI 构建时注入） |
| **主语言** | Go 1.25+（后端） · TypeScript / React 19（前端） |
| **框架** | Gin (HTTP) · GORM v2 (ORM) · React 19 + Rsbuild + TanStack Router + Tailwind CSS |
| **包管理** | Go Modules（后端） · Bun（前端，workspace monorepo） |
| **数据库** | SQLite / MySQL >=5.7.8 / PostgreSQL >=9.6（三库兼容） |
| **缓存** | Redis (go-redis) + 内存缓存 |
| **认证** | JWT · WebAuthn/Passkeys · OAuth (GitHub, Discord, OIDC, LinuxDo, 微信等) |
| **部署** | Docker / Docker Compose · 也支持裸机 `go run main.go` · Electron 桌面客户端 |

---

## 2. 核心目录结构

```
fangxiusun_new-api/
├── main.go                  # ★ 后端唯一入口文件
├── go.mod / go.sum          # Go 依赖管理
├── makefile                 # 构建/开发命令入口
├── Dockerfile / docker-compose.yml  # 容器化部署
├── .env.example             # 环境变量模板
│
├── router/                  # HTTP 路由注册层
│   ├── main.go              # 路由总入口 (SetRouter)
│   ├── api-router.go        # 用户/管理 API 路由
│   ├── relay-router.go      # AI 中继路由 (/v1/chat/completions 等)
│   ├── dashboard-router.go  # 仪表盘路由
│   ├── video-router.go      # 视频相关路由
│   └── web-router.go        # 静态前端文件路由
│
├── controller/              # 请求处理器（75+ 文件）
├── service/                 # 业务逻辑层（60+ 文件）
├── model/                   # 数据模型 & DB 访问层
├── middleware/               # 中间件（认证/限流/CORS/i18n/日志）
├── relay/                   # ★ AI 中继/代理核心
│   ├── channel/             # 36 个上游提供商适配器
│   ├── common_handler/      # 通用处理器
│   ├── helper/              # 计费/流处理辅助
│   └── *.go                 # 聊天/嵌入/图片/音频/视频 handler
│
├── common/                  # 共享工具（JSON/Redis/缓存/加密/IP/限流等）
├── constant/                # 常量定义（API 类型/渠道类型/上下文 key）
├── dto/                     # 数据传输对象（请求/响应结构体）
├── types/                   # 类型定义（中继格式/文件来源/错误）
├── setting/                 # 配置管理（计费/模型/运营/性能/限流等子模块）
├── pkg/                     # 内部包
│   ├── billingexpr/         # 计费表达式引擎（含 expr.md 文档）
│   ├── cachex/              # 混合缓存
│   ├── ionet/               # 客户端/部署/容器类型
│   └── perf_metrics/        # 性能指标采集
│
├── i18n/                    # 后端国际化 (go-i18n, en/zh)
├── oauth/                   # OAuth 提供商实现
├── logger/                  # 日志初始化
├── web/                     # 前端容器
│   ├── default/             # ★ 新前端 (React 19 + Rsbuild + Base UI + Tailwind)
│   │   └── src/
│   │       ├── features/    # 功能模块 (channels/users/dashboard/playground 等)
│   │       ├── components/  # UI 组件
│   │       ├── routes/      # TanStack 路由
│   │       ├── stores/      # 状态管理
│   │       └── i18n/        # 前端国际化 (6 语言)
│   └── classic/             # 经典前端 (React 19 + Rsbuild + Semi Design)
│
├── electron/                # Electron 桌面客户端封装
├── docs/                    # 文档 (渠道/安装/OpenAPI)
└── .github/                 # CI/CD (Docker 构建/PR 检查/Release/Electron)
```

---

## 3. 主要模块职责

| 模块 | 职责 |
|---|---|
| `router/` | 将 HTTP 请求路由到对应 controller，分 API/Relay/Dashboard/Video/Web 五组 |
| `controller/` | 请求处理：用户管理、渠道 CRUD、Token 管理、计费/充值/订阅、日志、Midjourney/视频任务、OAuth 等 |
| `service/` | 业务逻辑：配额计算、HTTP 客户端、渠道选择/亲和性、计费结算、Token 编码、任务轮询、订阅重置 |
| `model/` | 数据模型（User/Token/Channel/Log/Option/Redemption/Task 等）+ GORM 数据库操作 + 缓存同步 |
| `relay/` | **核心中继层**：接收 OpenAI 兼容请求 -> 路由到对应渠道适配器 -> 转换格式 -> 调用上游 -> 流式/非流式响应处理 |
| `relay/channel/*` | 36 个上游 AI 提供商的适配器（openai, claude, gemini, aws, deepseek, coze, siliconflow 等） |
| `middleware/` | 认证(JWT/WebAuthn)、速率限制、CORS、请求 ID、i18n 注入、gzip、分发器（渠道路由） |
| `common/` | 底层工具：JSON 封装、Redis 客户端、磁盘缓存、加密/哈希、IP 工具、系统监控、pprof |
| `setting/` | 运行时配置管理：计费比例、模型映射、运营参数、性能参数、限流规则等 |
| `dto/` | 与前端/上游交互的请求/响应 DTO 结构体 |
| `constant/` | 全局常量：API 类型枚举、渠道类型枚举、缓存 key、上下文 key |
| `pkg/billingexpr/` | 计费表达式引擎：支持分层/动态定价的表达式语言和编译执行 |
| `oauth/` | OAuth 登录实现：GitHub、Discord、OIDC、LinuxDo、自定义 OAuth |
| `i18n/` | 后端国际化（go-i18n）|
| `web/default/` | 新版前端：React 19 + Rsbuild + TanStack Router + Base UI + Tailwind + VChart |

---

## 4. 请求处理流程

```
HTTP 请求
  -> router/main.go (路由分发)
    -> middleware/ (认证/限流/i18n)
      -> controller/ (参数校验/业务入口)
        -> service/ (业务逻辑/配额/计费)
          -> model/ (数据持久化)
      -> relay/ (AI 中继核心，仅 /v1/* 路由)
        -> relay/channel/<provider>/ (适配器转换)
          -> 上游 AI API
```

---

## 5. 最应该优先阅读的文件（推荐顺序）

### 后端核心

| 优先级 | 文件 | 原因 |
|---|---|---|
| ★★★ | `main.go` | 全局入口，理解启动流程和资源初始化顺序 |
| ★★★ | `router/main.go` | 路由总入口，理解请求如何被分发 |
| ★★★ | `router/api-router.go` | 所有业务 API 路由定义 |
| ★★★ | `router/relay-router.go` | AI 中继路由，理解 `/v1/*` 端点映射 |
| ★★★ | `relay/relay_adaptor.go` | 中继适配器核心，理解如何选择上游渠道 |
| ★★☆ | `middleware/distributor.go` | 渠道分配中间件，理解负载均衡逻辑 |
| ★★☆ | `middleware/auth.go` | 认证中间件 |
| ★★☆ | `model/main.go` | 数据库初始化、迁移、跨库兼容 |
| ★★☆ | `model/channel.go` | 渠道模型，理解多上游管理 |
| ★★☆ | `common/json.go` | JSON 封装（必须使用，不可直接用 encoding/json） |
| ★★☆ | `common/init.go` | 环境变量和全局配置初始化 |
| ★★☆ | `dto/openai_request.go` | OpenAI 兼容请求结构体，理解 API 表面 |
| ★☆☆ | `constant/api_type.go` | API 类型枚举 |
| ★☆☆ | `constant/channel.go` | 渠道类型枚举 |
| ★☆☆ | `pkg/billingexpr/expr.md` | 计费表达式系统设计文档 |

### 前端核心

| 优先级 | 文件 | 原因 |
|---|---|---|
| ★★★ | `web/default/src/main.tsx` | 前端入口 |
| ★★★ | `web/default/src/routes/__root.tsx` | 根路由组件 |
| ★★☆ | `web/default/src/lib/api.ts` | API 请求封装 |
| ★★☆ | `web/default/src/stores/auth-store.ts` | 认证状态管理 |
| ★★☆ | `web/default/src/features/` | 各功能模块入口 |

### 运维/配置

| 优先级 | 文件 | 原因 |
|---|---|---|
| ★★☆ | `.env.example` | 所有环境变量说明 |
| ★★☆ | `AGENTS.md` | 项目编码规范 |
| ★★☆ | `Dockerfile` | 构建和部署流程 |

---

## 6. 关键数据与配置

- **环境变量**：通过 `.env` 文件 + 系统环境变量配置，见 `.env.example`
- **数据库**：默认 SQLite，通过 `SQL_DSN` 切换 MySQL/PostgreSQL
- **Redis**：可选，通过 `REDIS_CONN_STRING` 启用
- **前端主题**：支持 Default（新）和 Classic 两套前端，通过 `THEME` 环境变量切换
- **VERSION**：当前为空文件，CI 构建时注入具体版本号
- **Go 版本**：本地已安装 Go 1.25.1；go.mod 声明 1.25.1；Dockerfile 使用 golang:1.26.1-alpine
- **数据库迁移**：无独立迁移策略，使用 GORM AutoMigrate 自动管理
- **自定义部署配置**：无 Nginx/SSL 等自定义部署配置
