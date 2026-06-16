# New API — 项目总览文档

> 生成时间：2026-06-16
> 适用对象：新加入项目的技术人员
> 文档性质：基于代码静态分析生成，部分内容为推测（已标注）

---

## 1. 项目用途推测

### 核心定位

New API 是一个 **LLM（大语言模型）API 网关与 AI 资产管理系统**。

它解决的核心问题是：当团队/组织需要使用多个 AI 提供商（OpenAI、Claude、Gemini、AWS Bedrock 等 40+ 家）时，每个提供商的 API 格式、认证方式、计费模型各不相同。New API 在上游 AI API 和下游用户之间提供：

1. **统一 API 接口**：所有请求以 OpenAI 兼容格式发送，后端自动转换为对应提供商的原生格式
2. **渠道管理**：同一提供商可配置多个渠道（不同 API Key），系统自动负载均衡和故障转移
3. **用户管理**：多用户系统，支持注册/登录/OAuth/Passkey，每个用户有独立的 API Token
4. **计费与配额**：基于 Token 用量的计费系统，支持充值、兑换码、订阅制、分层定价
5. **管理后台**：Web 管理界面，用于渠道管理、用户管理、日志查看、模型配置等
6. **任务系统**：支持异步任务（Midjourney 图片生成、视频生成等）

### 推测的使用场景

- 企业内部 AI 网关：统一管理各部门对 AI API 的使用和配额
- AI API 转售/分发平台：向终端用户出售 AI API 访问额度
- 开发测试平台：提供 Playground 界面调试不同模型的 prompt

---

## 2. 技术栈

### 后端

| 技术 | 用途 | 备注 |
|---|---|---|
| **Go 1.25+** | 主语言 | 本地已安装 1.25.0；go.mod 声明 1.25.1 |
| **Gin** | HTTP 框架 | github.com/gin-gonic/gin v1.9.1 |
| **GORM v2** | ORM | 支持 SQLite/MySQL/PostgreSQL 三库 |
| **go-redis/v8** | Redis 客户端 | 可选，用于缓存和分布式锁 |
| **JWT (golang-jwt/v5)** | 认证 Token | |
| **go-webauthn** | Passkey/WebAuthn 认证 | |
| **go-i18n/v2** | 后端国际化 | 支持 en, zh-CN, zh-TW |
| **gorilla/websocket** | WebSocket | 用于实时通信 |
| **aws-sdk-go-v2** | AWS Bedrock 调用 | |
| **pyroscope-go** | 性能分析 | 可选 |
| **validator/v10** | 参数校验 | |

### 前端（Default — 推荐开发）

| 技术 | 用途 | 备注 |
|---|---|---|
| **React 19** | UI 框架 | |
| **TypeScript** | 类型安全 | |
| **Rsbuild** | 构建工具 | 字节跳动出品，Rspack 生态 |
| **TanStack Router** | 路由 | 文件路由 + 类型安全 |
| **TanStack Query** | 数据请求 | |
| **TanStack Table** | 表格 | |
| **Base UI** | 无样式组件库 | |
| **Tailwind CSS v4** | 样式 | |
| **VChart** | 图表 | 字节跳动 VisActor 生态 |
| **i18next** | 前端国际化 | 支持 en, zh, fr, ru, ja, vi |
| **Bun** | 包管理 & 脚本运行 | workspace monorepo |

### 前端（Classic — 维护模式）

| 技术 | 用途 |
|---|---|
| React 19 | UI 框架 |
| Rsbuild | 构建工具 |
| Semi Design | 组件库（抖音出品） |

### 基础设施

| 技术 | 用途 |
|---|---|
| **Docker** | 容器化部署 |
| **Docker Compose** | 本地开发编排（含 PostgreSQL/MySQL + Redis） |
| **GitHub Actions** | CI/CD（Docker 构建、PR 检查、Release、Electron 构建） |
| **Electron** | 桌面客户端封装（推测：用于本地运行场景） |

---

## 3. 目录结构说明

```
new-api/
│
├── main.go                      # 后端入口文件，包含启动流程和资源初始化
├── go.mod / go.sum              # Go 模块依赖
├── makefile                     # 开发构建命令
├── Dockerfile / Dockerfile.dev  # 生产/开发镜像构建
├── docker-compose.yml           # 生产编排（应用 + PostgreSQL + Redis）
├── docker-compose.dev.yml       # 开发编排
├── .env.example                 # 环境变量模板（所有可配置项的说明）
├── VERSION                      # 版本号文件（CI 构建时注入，本地为空）
├── AGENTS.md                    # 项目编码规范与约束
├── LICENSE / NOTICE             # 许可证文件
│
├── router/                      # 路由层
│   ├── main.go                  #   SetRouter() 路由总入口
│   ├── api-router.go            #   业务 API 路由（用户/渠道/Token/日志等）
│   ├── relay-router.go          #   AI 中继路由（/v1/chat/completions 等 OpenAI 兼容端点）
│   ├── dashboard-router.go      #   仪表盘数据路由
│   ├── video-router.go          #   视频任务路由
│   └── web-router.go            #   前端静态文件路由
│
├── controller/                  # 控制器层（请求处理器）
│   ├── user.go                  #   用户注册/登录/信息管理
│   ├── channel.go               #   渠道 CRUD（AI 提供商配置）
│   ├── token.go                 #   API Token 管理
│   ├── billing.go               #   计费相关
│   ├── relay.go                 #   AI 中继请求处理入口
│   ├── midjourney.go            #   Midjourney 任务管理
│   ├── option.go                #   系统配置项管理
│   ├── log.go                   #   日志查看
│   └── ...                      #   （共 75+ 文件）
│
├── service/                     # 业务逻辑层
│   ├── channel.go               #   渠道查询与选择
│   ├── channel_select.go        #   渠道选择算法
│   ├── channel_affinity.go      #   渠道亲和性（会话粘性）
│   ├── billing.go               #   计费逻辑
│   ├── billing_session.go       #   计费会话管理
│   ├── quota.go                 #   配额计算
│   ├── pre_consume_quota.go     #   预扣配额
│   ├── tiered_settle.go         #   分层结算
│   ├── convert.go               #   请求格式转换
│   ├── http.go                  #   HTTP 客户端封装
│   ├── tokenizer.go             #   Token 编码器
│   ├── task_polling.go          #   异步任务轮询
│   └── ...                      #   （共 60+ 文件）
│
├── model/                       # 数据模型 & 数据库操作
│   ├── main.go                  #   数据库初始化、迁移、跨库兼容逻辑
│   ├── user.go                  #   用户模型
│   ├── channel.go               #   渠道模型
│   ├── token.go                 #   Token 模型
│   ├── log.go                   #   日志模型
│   ├── option.go                #   配置项模型
│   ├── redemption.go            #   兑换码模型
│   ├── ability.go               #   渠道能力表（模型 -> 可用渠道映射）
│   ├── channel_cache.go         #   渠道内存缓存
│   ├── user_cache.go            #   用户内存缓存
│   ├── token_cache.go           #   Token 内存缓存
│   ├── pricing.go               #   定价模型
│   └── ...                      #   （共 40+ 文件）
│
├── relay/                       # AI 中继核心
│   ├── relay_adaptor.go         #   适配器接口定义（Adaptor/TaskAdaptor）
│   ├── chat_completions_via_responses.go  # 聊天补全 handler
│   ├── claude_handler.go        #   Claude 格式 handler
│   ├── gemini_handler.go        #   Gemini 格式 handler
│   ├── embedding_handler.go     #   嵌入向量 handler
│   ├── image_handler.go         #   图片生成 handler
│   ├── audio_handler.go         #   音频处理 handler
│   ├── rerank_handler.go        #   重排序 handler
│   ├── responses_handler.go     #   OpenAI Responses API handler
│   ├── websocket.go             #   WebSocket 代理
│   ├── relay_task.go            #   异步任务中继
│   ├── channel/                 #   上游提供商适配器目录
│   │   ├── adapter.go           #     Adaptor/TaskAdaptor 接口定义
│   │   ├── api_request.go       #     通用 API 请求逻辑
│   │   ├── openai/              #     OpenAI 适配器
│   │   ├── claude/              #     Claude/Anthropic 适配器
│   │   ├── gemini/              #     Google Gemini 适配器
│   │   ├── aws/                 #     AWS Bedrock 适配器
│   │   ├── deepseek/            #     DeepSeek 适配器
│   │   ├── siliconflow/         #     SiliconFlow 适配器
│   │   ├── codex/               #     Codex 适配器
│   │   └── ...                  #     （共 36 个提供商）
│   ├── common/                  #   中继层公共类型
│   │   ├── relay_info.go        #     RelayInfo 上下文信息
│   │   ├── billing.go           #     计费计算
│   │   └── stream_status.go     #     流式传输状态
│   └── helper/                  #   辅助工具
│       ├── price.go             #     价格计算
│       ├── stream_scanner.go    #     流式扫描器
│       └── billing_expr_request.go  # 计费表达式请求处理
│
├── middleware/                   # 中间件
│   ├── auth.go                  #   JWT/WebAuthn 认证
│   ├── distributor.go           #   渠道分配（负载均衡）
│   ├── rate-limit.go            #   速率限制
│   ├── cors.go                  #   CORS 跨域
│   ├── i18n.go                  #   国际化注入
│   ├── logger.go                #   请求日志
│   ├── request-id.go            #   请求 ID 生成
│   └── turnstile-check.go       #   Cloudflare Turnstile 人机验证
│
├── common/                      # 共享工具
│   ├── json.go                  #   JSON 封装（项目规范：必须通过此文件调用 JSON 操作）
│   ├── init.go                  #   环境变量和全局配置初始化
│   ├── redis.go                 #   Redis 客户端初始化
│   ├── database.go              #   数据库配置标志（UsingPostgreSQL/UsingMySQL/UsingSQLite）
│   ├── crypto.go                #   加密工具
│   ├── hash.go                  #   哈希工具
│   ├── ip.go                    #   IP 地址工具
│   ├── rate-limit.go            #   限流工具
│   ├── disk_cache.go            #   磁盘缓存
│   ├── sys_log.go               #   系统日志
│   ├── system_monitor.go        #   系统资源监控
│   ├── env.go                   #   环境变量读取
│   └── ...                      #   （共 45+ 文件）
│
├── dto/                         # 数据传输对象
│   ├── openai_request.go        #   OpenAI 兼容请求结构体
│   ├── openai_response.go       #   OpenAI 兼容响应结构体
│   ├── claude.go                #   Claude 请求/响应
│   ├── gemini.go                #   Gemini 请求/响应
│   ├── embedding.go             #   嵌入向量请求
│   ├── image.go                 #   图片生成请求
│   └── ...                      #   （共 30+ 文件）
│
├── constant/                    # 常量定义
│   ├── api_type.go              #   API 类型枚举（OpenAI/Claude/Gemini 等）
│   ├── channel.go               #   渠道类型枚举
│   ├── context_key.go           #   Gin Context key 常量
│   ├── cache_key.go             #   缓存 key 常量
│   └── ...
│
├── setting/                     # 运行时配置管理
│   ├── ratio_setting/           #   模型定价比例配置
│   ├── model_setting/           #   模型映射与元数据
│   ├── billing_setting/         #   计费策略配置
│   ├── operation_setting/       #   运营参数配置
│   ├── performance_setting/     #   性能参数配置
│   ├── system_setting/          #   系统全局配置
│   ├── config/                  #   配置文件/结构定义
│   └── ...
│
├── pkg/                         # 内部包
│   ├── billingexpr/             #   计费表达式引擎
│   │   ├── expr.md              #     设计文档（必读！）
│   │   ├── compile.go           #     表达式编译器
│   │   ├── run.go               #     表达式运行时
│   │   ├── settle.go            #     结算逻辑
│   │   └── types.go             #     类型定义
│   ├── cachex/                  #   混合缓存（内存 + Redis）
│   ├── ionet/                   #   IONet 客户端类型
│   └── perf_metrics/            #   性能指标采集与上报
│
├── i18n/                        # 后端国际化
│   ├── i18n.go                  #   初始化和语言加载
│   ├── keys.go                  #   翻译 key 定义
│   └── locales/                 #   翻译文件
│       ├── en.yaml
│       ├── zh-CN.yaml
│       └── zh-TW.yaml
│
├── oauth/                       # OAuth 提供商
│   ├── github.go                #   GitHub OAuth
│   ├── discord.go               #   Discord OAuth
│   ├── oidc.go                  #   OIDC 通用 OAuth
│   ├── linuxdo.go               #   LinuxDo OAuth
│   ├── generic.go               #   自定义 OAuth 通用实现
│   └── registry.go              #   提供商注册表
│
├── types/                       # 类型定义
│   ├── relay_format.go          #   中继格式类型
│   ├── error.go                 #   错误类型
│   ├── file_source.go           #   文件来源类型
│   └── ...
│
├── logger/                      # 日志初始化
├── bin/                         # 编译输出目录
│
├── web/                         # 前端
│   ├── package.json             #   Bun workspace 配置
│   ├── default/                 #   ★ 新前端（推荐开发）
│   │   ├── package.json         #     依赖和脚本
│   │   ├── rsbuild.config.ts    #     Rsbuild 构建配置
│   │   └── src/
│   │       ├── main.tsx         #       入口文件
│   │       ├── routes/          #       路由定义
│   │       │   ├── __root.tsx   #         根路由
│   │       │   ├── index.tsx    #         首页
│   │       │   └── ...          #
│   │       ├── features/        #       功能模块
│   │       │   ├── channels/    #         渠道管理
│   │       │   ├── users/       #         用户管理
│   │       │   ├── dashboard/   #         仪表盘
│   │       │   ├── playground/  #         AI 试验场
│   │       │   ├── pricing/     #         定价
│   │       │   └── ...          #
│   │       ├── components/      #       UI 组件
│   │       │   ├── ui/          #         基础 UI 组件
│   │       │   ├── layout/      #         布局组件
│   │       │   └── data-table/  #         数据表格组件
│   │       ├── stores/          #       状态管理
│   │       ├── hooks/           #       自定义 Hooks
│   │       ├── lib/             #       工具函数
│   │       ├── i18n/            #       前端国际化（6 语言）
│   │       │   ├── config.ts    #         i18next 配置
│   │       │   └── locales/     #         翻译文件 (en/zh/fr/ru/ja/vi)
│   │       └── styles/          #       全局样式
│   └── classic/                 #   经典前端（维护模式）
│       └── src/                 #     React 19 + Rsbuild + Semi Design
│
├── electron/                    # Electron 桌面客户端
│   ├── main.js                  #   Electron 主进程
│   ├── preload.js               #   预加载脚本
│   └── package.json             #   Electron 依赖
│
├── docs/                        # 文档
│   ├── channel/                 #   渠道接入文档
│   ├── installation/            #   安装部署文档
│   ├── openapi/                 #   OpenAPI 规范
│   └── llm-docs/                #   LLM 生成的项目文档（本目录）
│
└── .github/                     # GitHub 配置
    ├── workflows/               #   CI/CD 工作流
    │   ├── docker-build.yml     #     Docker 构建
    │   ├── release.yml          #     发布流程
    │   ├── pr-check.yml         #     PR 质量检查
    │   └── electron-build.yml   #     Electron 构建
    ├── ISSUE_TEMPLATE/          #   Issue 模板
    └── PULL_REQUEST_TEMPLATE.md #   PR 模板
```

---

## 4. 启动方式

### 方式一：本地开发（推荐）

```bash
# 1. 启动后端依赖（PostgreSQL + Redis）
make dev-api

# 2. 启动前端开发服务器（Default + Classic 双主题）
make dev-web
# Default 前端: http://localhost:5173
# Classic 前端: http://localhost:5174

# 或者一步到位：
make dev
```

首次启动会进入 Setup Wizard 引导完成初始配置。

### 方式二：直接运行后端

```bash
# 确保有 .env 文件或环境变量
go run main.go
# 默认监听 :3000
```

### 方式三：Docker Compose 生产部署

```bash
docker-compose up -d
# 访问 http://localhost:3000
```

### 方式四：构建后运行

```bash
# 构建前端
make build-all-frontends

# 编译后端
go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

# 运行
./new-api
```

---

## 5. 构建 / 测试 / Lint 命令

### 后端（Go）

| 命令 | 用途 |
|---|---|
| `go run main.go` | 本地运行后端 |
| `go build -ldflags "-s -w -X '...Version=...'" -o new-api` | 编译二进制 |
| `go test ./...` | 运行所有测试 |
| `go test ./relay/...` | 运行 relay 模块测试 |
| `go vet ./...` | 静态分析 |

### 前端（Default — `web/default/`）

| 命令 | 用途 |
|---|---|
| `bun install` | 安装依赖（在 `web/` 目录执行 workspace install） |
| `bun run dev` | 启动开发服务器 |
| `bun run build` | 生产构建 |
| `bun run build:check` | TypeScript 类型检查 + 构建 |
| `bun run typecheck` | 仅 TypeScript 类型检查 |
| `bun run lint` | ESLint 检查 |
| `bun run format` | Prettier 格式化 |
| `bun run format:check` | Prettier 格式检查 |
| `bun run i18n:sync` | 同步国际化翻译文件 |
| `bun run knip` | 检测未使用的代码/依赖 |

### Makefile

| 命令 | 用途 |
|---|---|
| `make dev` | 启动完整开发环境（后端 + 前端） |
| `make dev-api` | 仅启动后端 Docker 服务 |
| `make dev-web` | 仅启动前端开发服务器 |
| `make build-all-frontends` | 构建两个前端主题 |
| `make reset-setup` | 重置 Setup Wizard 状态 |

---

## 6. 主要模块列表

### 后端模块

| 模块 | 文件数 | 职责 | 重要程度 |
|---|---|---|---|
| `router/` | 6 | HTTP 路由注册，请求分发 | ★★★ |
| `controller/` | 75+ | 请求处理器，参数校验，业务入口 | ★★★ |
| `service/` | 60+ | 核心业务逻辑：计费、配额、渠道选择、请求转换 | ★★★ |
| `model/` | 40+ | 数据模型、GORM 操作、缓存管理 | ★★★ |
| `relay/` | 核心 | AI 中继核心：格式转换、流式处理、提供商适配 | ★★★ |
| `relay/channel/` | 36 个子目录 | 各 AI 提供商的适配器实现 | ★★★ |
| `middleware/` | 25 | 认证、限流、CORS、i18n、日志 | ★★☆ |
| `common/` | 45+ | 基础工具：JSON、Redis、加密、缓存、日志 | ★★☆ |
| `dto/` | 30+ | 请求/响应数据传输对象 | ★★☆ |
| `constant/` | 15 | 全局常量和枚举 | ★★☆ |
| `setting/` | 多个子目录 | 运行时配置管理（计费比例、模型映射等） | ★★☆ |
| `pkg/billingexpr/` | 7 | 计费表达式编译器和运行时 | ★★☆ |
| `pkg/cachex/` | 3 | 混合缓存抽象 | ★☆☆ |
| `i18n/` | 3 + 3 locales | 后端国际化 | ★☆☆ |
| `oauth/` | 8 | OAuth 登录实现 | ★☆☆ |
| `types/` | 9 | 类型定义 | ★☆☆ |

### 前端模块（Default）

| 模块 | 职责 |
|---|---|
| `features/channels/` | 渠道管理（AI 提供商配置） |
| `features/users/` | 用户管理 |
| `features/dashboard/` | 数据仪表盘 |
| `features/playground/` | AI 试验场（在线调试 prompt） |
| `features/keys/` | API Key 管理 |
| `features/pricing/` | 定价展示 |
| `features/subscriptions/` | 订阅管理 |
| `features/wallet/` | 钱包/充值 |
| `features/models/` | 模型管理 |
| `features/system-settings/` | 系统设置 |
| `features/usage-logs/` | 使用日志 |
| `features/rankings/` | 排行榜 |
| `features/redemption-codes/` | 兑换码管理 |
| `features/performance-metrics/` | 性能指标 |
| `features/setup/` | 初始设置向导 |
| `features/auth/` | 登录/注册 |
| `components/ui/` | 基础 UI 组件（基于 Base UI） |
| `stores/` | 状态管理（auth, notification, system-config） |
| `hooks/` | 自定义 Hooks |
| `lib/` | 工具函数（API 封装、格式化等） |
| `i18n/` | 前端国际化（6 语言） |

---

## 7. 关键配置文件说明

### 根目录配置

| 文件 | 说明 |
|---|---|
| `.env.example` | **最重要** — 所有环境变量的模板和说明，包括端口、数据库、Redis、功能开关等 |
| `go.mod` | Go 模块定义和依赖声明 |
| `makefile` | 开发构建命令集 |
| `VERSION` | 版本号（CI 构建时注入，本地为空） |
| `AGENTS.md` | 项目编码规范（JSON 封装规则、数据库兼容要求、前端包管理偏好等） |
| `Dockerfile` | 生产镜像构建（前端编译 -> 后端编译 -> 运行时镜像） |
| `docker-compose.yml` | 生产编排（应用 + PostgreSQL + Redis） |
| `docker-compose.dev.yml` | 开发编排 |

### 前端配置

| 文件 | 说明 |
|---|---|
| `web/package.json` | Bun workspace 配置，定义 default/classic 两个子包 |
| `web/default/package.json` | 新前端依赖和脚本 |
| `web/default/rsbuild.config.ts` | Rsbuild 构建配置（推测） |
| `web/default/src/i18n/config.ts` | i18next 配置（语言检测、fallback 策略） |
| `web/default/src/styles/theme.css` | 主题样式变量 |

### CI/CD 配置

| 文件 | 说明 |
|---|---|
| `.github/workflows/docker-build.yml` | Docker 镜像构建流水线 |
| `.github/workflows/release.yml` | 发布流程（推测：版本号注入 + Docker 推送） |
| `.github/workflows/pr-check.yml` | PR 质量检查（anti-slop 反 AI 水文检测） |
| `.github/workflows/electron-build.yml` | Electron 桌面客户端构建 |
| `.github/PULL_REQUEST_TEMPLATE.md` | PR 模板 |

---

## 8. 新人阅读代码路径建议

### 第一天：理解全局

1. **`README.md`** — 了解项目是什么、做什么
2. **`.env.example`** — 了解所有可配置项和系统能力
3. **`main.go`** — 理解启动流程：初始化顺序、资源加载、后台任务
4. **`router/main.go`** — 理解请求如何被分发到不同路由组

### 第二天：理解请求处理链路

5. **`router/api-router.go`** — 浏览所有业务 API 端点
6. **`router/relay-router.go`** — 理解 AI 中继端点（`/v1/chat/completions` 等）
7. **`middleware/auth.go`** — 理解认证流程
8. **`middleware/distributor.go`** — 理解渠道路由和负载均衡
9. **`controller/relay.go`** — 理解中继请求的入口处理

### 第三天：理解核心中继逻辑

10. **`relay/relay_adaptor.go`** — 理解适配器接口定义（这是整个中继层的核心抽象）
11. **`relay/channel/adapter.go`** — 理解 Adaptor 和 TaskAdaptor 接口方法
12. **`relay/channel/openai/`** — 以 OpenAI 适配器为例，理解一个适配器的完整实现
13. **`relay/common/relay_info.go`** — 理解 RelayInfo 上下文（携带请求的所有元信息）

### 第四天：理解计费和配额

14. **`service/billing.go`** — 理解计费核心逻辑
15. **`service/pre_consume_quota.go`** — 理解预扣配额机制
16. **`model/pricing.go`** — 理解定价模型
17. **`pkg/billingexpr/expr.md`** — 理解计费表达式系统（如果涉及定价功能）

### 第五天：理解数据层和前端

18. **`model/main.go`** — 理解数据库初始化、跨库兼容（SQLite/MySQL/PostgreSQL）
19. **`model/channel.go`** — 理解渠道数据模型
20. **`web/default/src/main.tsx`** — 前端入口
21. **`web/default/src/lib/api.ts`** — 前端 API 请求封装
22. **`web/default/src/features/`** — 浏览各功能模块

### 深入阅读（按需）

- **渠道适配器**：选择一个感兴趣的提供商（如 `relay/channel/claude/`），完整阅读其适配器实现
- **计费表达式**：`pkg/billingexpr/expr.md` + `compile.go` + `run.go`
- **OAuth 登录**：`oauth/registry.go` + `oauth/github.go`
- **国际化**：`i18n/i18n.go` + `web/default/src/i18n/config.ts`

---

## 附录：关键设计模式

### 适配器模式（Adapter Pattern）

`relay/` 层使用经典的适配器模式。`channel.Adaptor` 接口定义了统一的方法（`ConvertOpenAIRequest`、`DoRequest`、`DoResponse` 等），每个上游提供商实现自己的适配器。系统根据渠道类型选择对应适配器，将 OpenAI 格式请求转换为提供商原生格式。

### 缓存策略

项目采用多级缓存：
- **内存缓存**（`common/`）：用于热点数据（渠道列表、用户信息、Token）
- **Redis 缓存**（可选）：分布式环境下的缓存共享
- **磁盘缓存**（`common/disk_cache.go`）：文件级别的缓存

### 数据库兼容层

通过 `common/database.go` 中的标志位（`UsingPostgreSQL`/`UsingMySQL`/`UsingSQLite`）和 `model/main.go` 中的 `commonGroupCol`/`commonKeyCol`/`commonTrueVal` 等变量，实现跨数据库的 SQL 兼容。

### 前端双主题架构

前端通过 Bun workspace 管理两套独立的前端代码（default 和 classic），后端通过 `go:embed` 将两套构建产物嵌入二进制，运行时通过环境变量 `THEME` 选择。
