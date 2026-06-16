# New API — 本地开发环境搭建指南

> 适用对象：首次接触项目、需要在本地跑起来的开发者
> 平台：Windows / macOS / Linux

---

## 1. 前置依赖

| 工具 | 最低版本 | 安装方式 | 验证命令 |
|---|---|---|---|
| **Go** | 1.25+ | https://go.dev/dl/ | `go version` |
| **Bun** | 1.0+ | https://bun.sh/ | `bun --version` |
| **Git** | 2.30+ | 系统自带或 https://git-scm.com/ | `git --version` |
| **Docker**（可选） | 24+ | https://docs.docker.com/get-docker/ | `docker --version` |
| **Docker Compose**（可选） | v2 | 通常随 Docker 安装 | `docker compose version` |

---

## 2. 快速启动（最简路径）

```bash
# 1. 克隆仓库
git clone https://github.com/QuantumNous/new-api.git
cd new-api

# 2. 安装前端依赖
cd web
bun install --frozen-lockfile
cd ..

# 3. 构建前端（嵌入到 Go 二进制中）
cd web/default && bun run build && cd ../..
cd web/classic && bun run build && cd ../..

# 4. 启动后端（默认使用 SQLite，无需数据库）
go run main.go

# 5. 访问
# 浏览器打开 http://localhost:3000
# 首次访问会进入 Setup Wizard
# 默认 root 账户：root / 123456（如果自动创建）
```

---

## 3. 开发模式启动（推荐）

开发模式下前后端分离，前端支持热更新。

### 3.1 启动后端依赖（PostgreSQL + Redis）

```bash
# 使用 Docker Compose 启动开发环境
make dev-api

# 这会启动：
# - PostgreSQL (端口 5432)
# - Redis (端口 6379)
# - 后端服务（使用 Docker）
```

如果不想用 Docker，可以直接运行后端：

```bash
# 创建 .env 文件
cp .env.example .env

# 编辑 .env，配置数据库（可选，不配置则使用 SQLite）
# SQL_DSN=postgresql://root:123456@localhost:5432/new-api
# REDIS_CONN_STRING=redis://:123456@localhost:6379

# 启动后端
go run main.go
```

### 3.2 启动前端开发服务器

```bash
# 安装依赖（在 web/ 目录执行 workspace install）
cd web
bun install

# 启动 Default 前端（端口 5173）
cd default && bun run dev

# 或同时启动两个前端
make dev-web
# Default: http://localhost:5173
# Classic: http://localhost:5174
```

### 3.3 一步到位

```bash
# 同时启动后端依赖 + 前端开发服务器
make dev
```

---

## 4. 环境变量配置

### 4.1 最小配置（.env）

```bash
# 复制模板
cp .env.example .env
```

### 4.2 常用环境变量

```bash
# 端口（默认 3000）
PORT=3000

# 数据库（不配置则使用 SQLite）
# SQL_DSN=postgresql://root:password@localhost:5432/new-api
# SQL_DSN=root:password@tcp(localhost:3306)/new-api?parseTime=true

# Redis（可选，不配置则禁用）
# REDIS_CONN_STRING=redis://:password@localhost:6379

# 调试模式
DEBUG=true

# 前端主题（default 或 classic）
THEME=default

# Session 密钥（生产环境必须修改）
SESSION_SECRET=your_random_string_here
```

### 4.3 开发专用环境变量

```bash
# 启用 pprof 性能分析
ENABLE_PPROF=true

# 批量更新（开发时可关闭以立即看到配额变化）
BATCH_UPDATE_ENABLED=false

# 渠道自动测试频率（秒）
CHANNEL_TEST_FREQUENCY=300
```

---

## 5. 数据库选择

### 5.1 SQLite（零配置，推荐开发）

```bash
# 不设置 SQL_DSN 即可，默认使用 one-api.db
go run main.go
```

### 5.2 PostgreSQL

```bash
# 方式一：Docker
docker run -d --name new-api-pg \
  -e POSTGRES_USER=root \
  -e POSTGRES_PASSWORD=123456 \
  -e POSTGRES_DB=new-api \
  -p 5432:5432 \
  postgres:16

# .env 配置
SQL_DSN=postgresql://root:123456@localhost:5432/new-api
```

### 5.3 MySQL

```bash
# 方式一：Docker
docker run -d --name new-api-mysql \
  -e MYSQL_ROOT_PASSWORD=123456 \
  -e MYSQL_DATABASE=new-api \
  -p 3306:3306 \
  mysql:8

# .env 配置
SQL_DSN=root:123456@tcp(localhost:3306)/new-api?parseTime=true
```

---

## 6. 常用开发命令

### 后端

```bash
# 编译检查
go build ./...

# 运行测试
go test ./...

# 运行特定模块测试
go test ./model/...
go test ./pkg/billingexpr/...
go test ./relay/helper/...
go test ./service/...
go test ./dto/...

# 静态分析
go vet ./...

# 清理未使用依赖
go mod tidy

# 启动后端
go run main.go
```

### 前端

```bash
cd web/default

# 开发服务器
bun run dev

# TypeScript 类型检查
bun run typecheck

# ESLint 检查
bun run lint

# 格式化
bun run format

# 格式检查（不修改）
bun run format:check

# 生产构建
bun run build

# 构建检查（类型检查 + 构建）
bun run build:check

# 同步 i18n 翻译
bun run i18n:sync

# 检测未使用的代码/依赖
bun run knip
```

### Makefile

```bash
make dev              # 启动完整开发环境
make dev-api          # 仅启动后端 Docker 服务
make dev-web          # 仅启动前端开发服务器
make build-all-frontends  # 构建两个前端
make reset-setup      # 重置 Setup Wizard 状态
```

---

## 7. 首次设置向导

首次启动后访问 `http://localhost:3000`，会自动进入 Setup Wizard：

1. **设置管理员密码** — 修改默认 root 密码
2. **配置基础设置** — 系统名称、公告等
3. **添加 AI 渠道** — 配置至少一个上游 AI 提供商的 API Key
4. **创建 API Token** — 用于调用 `/v1/*` 中继 API

如果需要重新运行向导：

```bash
make reset-setup
```

---

## 8. 调试技巧

### 8.1 后端调试

```bash
# 启用 pprof（端口 8005）
ENABLE_PPROF=true go run main.go

# 访问 pprof
# http://localhost:8005/debug/pprof/
```

### 8.2 查看日志

```bash
# 日志默认写入 ./logs/ 目录
# 可通过 --log-dir 参数修改
go run main.go --log-dir ./my-logs
```

### 8.3 数据库调试

```bash
# SQLite（使用 sqlite3 命令行）
sqlite3 one-api.db

# PostgreSQL（使用 psql）
psql -h localhost -U root -d new-api
```

### 8.4 API 测试

```bash
# 测试系统状态
curl http://localhost:3000/api/status

# 测试中继 API（需要有效的 Token）
curl http://localhost:3000/v1/models \
  -H "Authorization: Bearer sk-your-token-here"
```
