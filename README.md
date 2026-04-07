# MCP 服务管理平台

基于 Go 1.26、Gin、GORM、PostgreSQL（默认部署）、SQLite 回退与 `mcp-go` 的单机版 MCP 服务管理平台。

## 功能

- 本地账号密码登录与 JWT 鉴权
- MCP 服务管理，支持 `stdio`、`streamable_http`、`sse`
- 服务连接、断开、状态查询、健康检查
- 工具同步、工具调用、调用历史
- 审计日志导出、Swagger 入口、Docker 部署

## 快速开始

### 默认部署：PostgreSQL

推荐优先使用 Docker/Compose 启动默认 PostgreSQL 容器：

```bash
docker compose -f deployments/docker/docker-compose.yml up -d
```

### 本地显式回退：SQLite

如果需要本地开发或临时回退到 SQLite，可显式指定环境变量后启动：

```bash
MCP_DATABASE_DRIVER=sqlite MCP_DATABASE_DSN=data/mcp_manager.db go run ./cmd/server
```

也可以沿用默认的 `config.yaml` 配置并通过环境变量覆盖数据库驱动与 DSN。

### 直接运行

```bash
go mod tidy
go run ./cmd/server
```

> 说明：直接运行时默认会连接 PostgreSQL；如果本地还没有 PostgreSQL，请使用上面的“本地显式回退：SQLite”命令。

默认管理员：

- 用户名：`root`
- 密码：`admin123456`

## 主要接口

- `POST /api/v1/auth/login`
- `GET /api/v1/services`
- `POST /api/v1/services/:id/connect`
- `POST /api/v1/services/:id/sync-tools`
- `POST /api/v1/tools/:id/invoke`
- `GET /api/v1/history`

## 测试

项目当前测试分为三层：

- `internal/...` 下的单元测试，覆盖 service、middleware、database 等核心模块
- `tests/integration`，直接针对 Gin 引擎验证完整业务流
- `tests/e2e`，通过真实 HTTP 服务和 `http.Client` 验证登录、服务生命周期与权限边界

常用命令：

```bash
go test ./...
make test-e2e
```

## Docker

```bash
docker compose -f deployments/docker/docker-compose.yml up -d
```

Docker/Compose 默认会拉起 PostgreSQL 容器并让 `mcp-manager` 连接 PostgreSQL。若要显式切回 SQLite，请在启动前覆盖：

```bash
MCP_DATABASE_DRIVER=sqlite MCP_DATABASE_DSN=data/mcp_manager.db
```

注意：当前 Docker/Compose 默认不再挂载 `/app/data` 作为主数据库路径；如果你要在容器场景下持久化 SQLite，请额外挂载对应 volume。

容器镜像仍保留环境变量覆盖能力，优先级为：环境变量 > 容器内配置 > 根默认配置。
