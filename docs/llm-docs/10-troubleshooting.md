# New API — 常见问题与排障指南

> 适用对象：开发和部署过程中遇到问题的开发者
> 按问题类型分类，每条包含现象、原因和解决方案

---

## 1. 启动与编译问题

### 1.1 `go: go.mod requires go >= 1.25.1`

**现象**：编译时报 Go 版本不满足要求。

**原因**：go.mod 声明了 `go 1.25.1`，本地 Go 版本过低。

**解决**：
```bash
go version  # 检查当前版本
# 升级 Go 到 1.25.1 或更高版本
```

### 1.2 `failed to initialize database` / `no such file or directory`

**现象**：启动时报数据库初始化失败。

**原因**：SQLite 数据库文件路径不存在或无写入权限。

**解决**：
```bash
# 检查 SQLITE_PATH 环境变量
echo $SQLITE_PATH

# 确保目录存在且有写入权限
mkdir -p /data
chmod 755 /data

# 或使用默认路径（当前目录下的 one-api.db）
go run main.go
```

### 1.3 `Redis ping test failed`

**现象**：启动时报 Redis 连接失败，程序退出。

**原因**：`REDIS_CONN_STRING` 配置了 Redis 但 Redis 未启动。

**解决**：
```bash
# 方式一：启动 Redis
docker run -d --name redis -p 6379:6379 redis:7

# 方式二：不使用 Redis（注释掉 .env 中的 REDIS_CONN_STRING）
# REDIS_CONN_STRING=redis://...
```

### 1.4 `bun: command not found`

**现象**：构建前端时报 bun 未找到。

**解决**：
```bash
# macOS / Linux
curl -fsSL https://bun.sh/install | bash

# Windows (PowerShell)
powershell -c "irm bun.sh/install.ps1 | iex"

# 验证
bun --version
```

### 1.5 前端构建失败 `DISABLE_ESLINT_PLUGIN`

**现象**：前端构建时 ESLint 报错导致构建失败。

**解决**：
```bash
# 构建时禁用 ESLint（CI 中的做法）
cd web/default
DISABLE_ESLINT_PLUGIN='true' bun run build

# 或先修复 ESLint 错误
bun run lint
```

---

## 2. 运行时问题

### 2.1 `401 Unauthorized` — Token 无效

**现象**：调用 `/v1/*` API 返回 401。

**排查步骤**：
1. 确认 Token 格式正确：`Authorization: Bearer sk-xxxx`
2. 确认 Token 未过期：在管理界面检查 Token 状态
3. 确认 Token 未被禁用：`status` 应为 1
4. 确认 Token 剩余额度 > 0（或设置了 `unlimited_quota`）

```bash
# 测试 Token 是否有效
curl -s http://localhost:3000/v1/models \
  -H "Authorization: Bearer sk-your-token" | jq .
```

### 2.2 `403 Forbidden` — 无权限访问模型

**现象**：Token 有效但请求特定模型返回 403。

**原因**：Token 开启了模型白名单但目标模型不在白名单中。

**解决**：在管理界面编辑 Token，将目标模型添加到白名单，或关闭模型白名单限制。

### 2.3 `502 Bad Gateway` — 上游无可用渠道

**现象**：请求返回 502，日志显示 "no available channel"。

**排查步骤**：
1. 检查是否有启用的渠道：管理界面 → 渠道管理
2. 检查渠道的模型列表是否包含请求的模型
3. 检查渠道的分组是否与 Token/用户的分组匹配
4. 检查渠道是否被自动禁用（`auto_ban`）

```bash
# 查看渠道状态
curl -s http://localhost:3000/api/channel/ \
  -H "Cookie: session=your-session" | jq '.data.items[] | {id, name, status, models}'
```

### 2.4 流式响应中断或空补全

**现象**：`stream=true` 时响应中途断开或返回空内容。

**原因**：上游超时、网络不稳定、或 `STREAMING_TIMEOUT` 设置过小。

**解决**：
```bash
# .env 中增大流式超时
STREAMING_TIMEOUT=300

# 检查上游渠道的响应时间
# 管理界面 → 渠道管理 → 查看 response_time
```

### 2.5 配额不扣减或扣减异常

**现象**：请求成功但用户配额未变化。

**排查**：
1. 检查是否使用了无限额度 Token（`unlimited_quota=true`）
2. 检查是否命中了信任额度（用户额度 > `trust_quota` 时不预扣）
3. 检查是否为免费模型（`model_price=0`）
4. 检查 `BATCH_UPDATE_ENABLED` 是否为 true（批量模式有延迟）

---

## 3. 数据库问题

### 3.1 SQLite 锁定 `database is locked`

**现象**：高并发时报 SQLite 锁定错误。

**原因**：SQLite 不支持高并发写入。

**解决**：
- 开发环境：降低并发，或使用 `BATCH_UPDATE_ENABLED=false`
- 生产环境：切换到 PostgreSQL 或 MySQL

### 3.2 MySQL 中文乱码

**现象**：中文用户名或内容显示为乱码。

**原因**：MySQL 字符集不支持中文。

**解决**：
```sql
-- 检查当前字符集
SHOW VARIABLES LIKE 'character_set%';

-- 修改数据库字符集
ALTER DATABASE new-api CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 重新启动应用，启动时会自动检查
```

### 3.3 PostgreSQL 保留字冲突

**现象**：查询 `group` 或 `key` 列时报语法错误。

**原因**：PostgreSQL 中 `group` 和 `key` 是保留字。

**说明**：项目已通过 `commonGroupCol`/`commonKeyCol` 变量自动处理此问题。如果自己写 raw SQL，必须使用这些变量：

```go
// 正确
DB.Raw(fmt.Sprintf("SELECT %s FROM channels", commonGroupCol))

// 错误 — 会导致 PostgreSQL 报错
DB.Raw("SELECT group FROM channels")
```

---

## 4. 前端问题

### 4.1 前端白屏

**现象**：访问 `http://localhost:3000` 显示白屏。

**排查**：
1. 检查是否构建了前端：`web/default/dist/` 目录是否存在
2. 检查浏览器控制台是否有 JS 错误
3. 检查 `THEME` 环境变量是否正确（`default` 或 `classic`）

```bash
# 重新构建前端
cd web/default && bun run build
# 重启后端
go run main.go
```

### 4.2 前端开发服务器 API 请求 404

**现象**：前端 `bun run dev` 启动后，API 请求返回 404。

**原因**：前端开发服务器的代理配置未指向正确的后端地址。

**解决**：确保后端在 `localhost:3000` 运行，前端 Rsbuild 会自动代理 `/api` 和 `/v1` 请求。

### 4.3 i18n 翻译缺失

**现象**：界面显示英文 key 而非翻译文本。

**解决**：
```bash
cd web/default
bun run i18n:sync  # 同步翻译文件
```

---

## 5. 部署问题

### 5.1 Docker 容器启动后无法访问

**现象**：`docker-compose up -d` 后无法访问 3000 端口。

**排查**：
```bash
# 检查容器状态
docker compose ps

# 查看容器日志
docker compose logs new-api

# 检查端口映射
docker compose port new-api 3000
```

### 5.2 多节点部署 Session 不共享

**现象**：多实例部署时，登录一个节点后另一个节点未登录。

**原因**：Session 密钥不一致或未配置 Redis 共享 Session。

**解决**：
```bash
# 所有节点设置相同的 SESSION_SECRET
SESSION_SECRET=same_random_string_for_all_nodes

# 配置 Redis 共享 Session
REDIS_CONN_STRING=redis://shared-redis:6379
```

### 5.3 版本号显示为空

**现象**：管理界面显示版本号为空。

**原因**：`VERSION` 文件为空，本地构建时未注入版本号。

**解决**：
```bash
# 方式一：写入 VERSION 文件
echo "dev" > VERSION

# 方式二：编译时注入
go build -ldflags "-X 'github.com/QuantumNous/new-api/common.Version=v1.0.0'" -o new-api
```

---

## 6. 开发调试 FAQ

### Q: 如何重置 Setup Wizard？

```bash
# 使用 makefile
make reset-setup

# 或手动操作
# SQLite:
sqlite3 one-api.db "DELETE FROM setups; DELETE FROM users WHERE role = 100;"
# PostgreSQL:
psql -U root -d new-api -c "DELETE FROM setups; DELETE FROM users WHERE role = 100;"
```

### Q: 如何查看当前配置项？

```bash
# 通过 API（需要管理员登录）
curl -s http://localhost:3000/api/option/ \
  -H "Cookie: session=your-session" | jq '.data'
```

### Q: 如何测试某个渠道是否可用？

```bash
# 通过 API（需要管理员登录）
curl -s http://localhost:3000/api/channel/test/1 \
  -H "Cookie: session=your-session" | jq .
```

### Q: 如何切换前端主题？

```bash
# .env 中设置
THEME=classic  # 或 default

# 重启后端
go run main.go
```

### Q: 代码修改后如何快速验证？

```bash
# 后端：编译检查 + 测试
go build ./... && go test ./... && go vet ./...

# 前端：类型检查 + lint
cd web/default && bun run typecheck && bun run lint
```

### Q: 如何在本地模拟多数据库测试？

```bash
# 测试 SQLite（默认）
go test ./model/...

# 测试 PostgreSQL（需要运行中的 PG 实例）
TEST_POSTGRES_DSN=postgresql://root:123456@localhost:5432/test_db go test ./model/...

# 测试 MySQL（需要运行中的 MySQL 实例）
TEST_MYSQL_DSN=root:123456@tcp(localhost:3306)/test_db?parseTime=true go test ./model/...
```
