# MCP 服务管理平台

基于 Go 1.26、Gin、GORM 与 `mcp-go` 的 MCP 服务管理平台。

## 新手入口

- 想先快速理解项目怎么启动、代码应该从哪里看：见 [`docs/项目新手入门指南.md`](docs/项目新手入门指南.md)

它面向 **MCP 服务注册、连接管理、工具同步、工具调用、调用历史与审计追踪**，当前代码同时支持：

- 单体 `all` 模式
- `control-plane / executor` 双角色模式
- PostgreSQL 默认部署
- SQLite 显式回退
- Redis 黑名单与运行态快照
- 同步 / 异步工具调用
- Swagger、Docker / Compose、集成测试与 E2E 测试

> 重要：当前仓库默认数据库是 **PostgreSQL**，默认 `redis.enabled=true`。如果你本地没有 PostgreSQL / Redis，请按下文显式覆盖配置。

---

## 1. 核心能力

- 本地账号密码登录、JWT 鉴权、刷新与注销
- MCP 服务管理：创建、更新、删除、连接、断开、状态查询
- 支持 `stdio`、`sse`、`streamable_http` 等传输方式
- 工具同步、工具详情查询、同步调用、异步调用、任务取消与统计
- 请求历史记录、审计日志导出
- `/health`、`/ready`、`/swagger/*any` 基础运维入口
- Docker Compose 一键拉起 PostgreSQL、Redis 与应用服务
- dual-role 拓扑下 control-plane 通过内部 RPC 调用 executor

---

## 2. 系统架构

### 2.1 分层结构

```text
+------------------------------------------------------+
|                   HTTP API / Swagger                 |
| /auth /services /tools /tasks /history /audit-logs  |
+------------------------------+-----------------------+
                               |
                               v
+------------------------------------------------------+
|                    internal/router                   |
|   Gin 路由、CORS、JWT 鉴权、中间件、ready/health     |
+------------------------------+-----------------------+
                               |
                               v
+------------------------------------------------------+
|                internal/handler + service            |
|  认证 / 用户 / MCP 服务 / 工具调用 / 历史 / 审计     |
+------------------------------+-----------------------+
                               |
         +---------------------+---------------------+
         |                                           |
         v                                           v
+-------------------------------+      +-------------------------------+
|      internal/repository      |      |   mcpclient / rpc / runtime   |
| 用户/服务/工具/历史/审计持久化 |      | 连接、状态、工具目录、工具调用 |
+---------------+---------------+      +---------------+---------------+
                |                                      |
                v                                      v
     +------------------------+           +----------------------------+
     | PostgreSQL / SQLite    |           | Redis / MCP Runtime / RPC |
     +------------------------+           +----------------------------+
```

### 2.2 启动链路

```text
config.Load
    |
    v
logger.Init
    |
    v
bootstrap.NewBuilder(cfg).Build()
    |
    +--> database.Init
    +--> database.Migrate            (control-plane / all)
    +--> EnsureAdmin                 (control-plane / all)
    +--> Redis / JWT / Repository / Service
    +--> HealthChecker               (all / executor 且启用时)
    +--> AuditCleanupTask            (control-plane / all)
    +--> router.New
    +--> RPC Server                  (executor 且 rpc.enabled=true)
    |
    v
serveApp -> ListenAndServe -> Signal -> Shutdown -> Cleanup
```

### 2.3 角色拓扑

```text
                    +----------------------+
                    |    control-plane     |
                    | 管理接口 / 审计 / DB  |
                    | /ready 依赖 executor |
                    +----------+-----------+
                               |
                               | internal RPC
                               v
                    +----------------------+
                    |       executor       |
                    | 本地 runtime 执行层  |
                    | 只暴露 health/ready  |
                    +----------------------+

all 模式 = control-plane + executor 同进程
```

### 2.4 请求流转

```text
Client
  |
  v
Gin Router
  |
  +--> Auth Middleware
  |
  v
Handler
  |
  v
Service
  |
  +--> Repository --> PostgreSQL / SQLite
  |
  +--> Runtime Adapter --> Local Manager / RPC Executor
  |
  +--> History / Audit Sink --> DB / Async Queue
  |
  +--> JWT Blacklist / Runtime Snapshot --> Redis
```

---

## 3. 目录结构

```text
.
├── cmd/server                 # 程序入口
├── internal
│   ├── bootstrap              # 应用装配、角色切换、依赖拼装
│   ├── config                 # 配置加载、默认值、环境变量绑定
│   ├── database               # 数据库初始化、迁移与关闭
│   ├── domain/entity          # 领域实体
│   ├── handler                # HTTP Handler
│   ├── mcpclient              # MCP 运行时管理与健康检查
│   ├── middleware             # 鉴权与权限控制
│   ├── repository             # 持久层
│   ├── router                 # 路由与 ready/health
│   ├── rpc                    # dual-role 内部 RPC
│   ├── service                # 业务层
│   └── task                   # 后台任务（如审计清理）
├── pkg                        # 通用组件（crypto/logger/email/response/...）
├── api/docs                   # Swagger 产物
├── db                         # 迁移与种子数据
├── deployments/docker         # Dockerfile、Compose、生产配置
├── tests                      # integration / e2e / pgtest / testutil
├── scripts                    # 本地脚本与 mock MCP server
├── config.yaml                # 默认配置
└── Makefile                   # 构建、测试、Swagger 命令
```

---

## 4. 运行模式

### 4.1 模式说明

| 模式 | 说明 | 暴露 API | 典型用途 |
| --- | --- | --- | --- |
| `all` | 单进程同时承担控制面与执行面 | 完整控制面 API + `/health` `/ready` `/swagger` | 本地开发、单机部署 |
| `control-plane` | 只提供控制面，不跑本地 runtime | 完整控制面 API + `/health` `/ready` `/swagger` | 双角色部署中的管理面 |
| `executor` | 只提供执行能力与内部 RPC | 仅 `/health` `/ready` `/swagger` | 双角色部署中的执行面 |

### 4.2 `/health` 与 `/ready` 的区别

```text
/health  -> 轻量存活探针，返回 {"status":"ok"}
/ready   -> 角色感知探针
            - all:           检查 database + runtime
            - control-plane: 检查 database + executor RPC
            - executor:      检查 database + runtime + rpc server
```

### 4.3 角色相关注意事项

- `executor` 模式不会暴露 `/api/v1/...` 控制面接口。
- `control-plane` 与 `executor` 属于成对出现的 dual-role 部署；当前代码要求这两种角色下 `rpc.enabled=true`，否则配置校验会直接失败。
- `control-plane` 模式下，运行态操作会通过内部 RPC 转发给 executor。
- `all` 模式仍然是最简单、最直接的部署方式。

---

## 5. 快速开始

### 5.1 前置条件

默认配置下建议先准备：

- Go `1.26`
- PostgreSQL `16+`
- Redis `7+`

默认端口：

- HTTP：`8080`
- Executor RPC：`18081`
- PostgreSQL：`5432`
- Redis：`6379`

### 5.2 默认路径：PostgreSQL + Redis

1. 启动 PostgreSQL 与 Redis。
2. 确认 `config.yaml` 或环境变量中的连接信息正确。
3. 运行服务：

```bash
go run ./cmd/server
```

或：

```bash
make build
./bin/mcp-manager
```

默认管理员账号：

- 用户名：`root`
- 密码：`admin123456`
- 邮箱：`root@example.com`

### 5.3 本地显式回退：SQLite

如果本地暂时没有 PostgreSQL，建议显式关闭 Redis 并切回 SQLite：

```bash
MCP_DATABASE_DRIVER=sqlite \
MCP_DATABASE_DSN=data/mcp_manager.db \
MCP_REDIS_ENABLED=false \
go run ./cmd/server
```

> SQLite 是当前仓库的**显式回退路径**，不是默认主路径。

### 5.4 常用环境变量覆盖

配置文件会被 `MCP_*` 环境变量覆盖，常见项如下：

```bash
MCP_SERVER_PORT=8080
MCP_DATABASE_DRIVER=postgres
MCP_DATABASE_DSN=postgres://postgres:postgres@127.0.0.1:5432/mcp_manager?sslmode=disable
MCP_REDIS_ENABLED=true
MCP_REDIS_ADDR=127.0.0.1:6379
MCP_APP_ROLE=all
MCP_RPC_ENABLED=false
MCP_RPC_EXECUTOR_TARGET=http://127.0.0.1:18081
MCP_JWT_SECRET=change-me
```

---

## 6. 配置概览

### 6.1 配置来源优先级

```text
环境变量 MCP_*  >  当前目录 config.yaml  >  代码默认值
```

### 6.2 关键配置分组

| 分组 | 关键项 | 当前默认行为 |
| --- | --- | --- |
| `server` | `host` `port` `read_timeout` `write_timeout` | 监听 `0.0.0.0:8080` |
| `database` | `driver` `dsn` `max_open_conns` | 默认 PostgreSQL |
| `redis` | `enabled` `addr` `key_prefix` | 默认启用，供黑名单与快照使用 |
| `rpc` | `enabled` `listen_addr` `executor_target` | 默认关闭 |
| `execution` | 并发、限流、异步任务 | 默认多项治理关闭 |
| `health_check` | 间隔、超时、失败阈值 | 默认启用 |
| `runtime` | `status_source` `snapshot_enabled` `idle_timeout` | 默认 startup reconcile 开启 |
| `audit`/`history`/`alert` | `async_enabled` `queue_size` | 默认异步关闭 |

### 6.3 当前代码默认值要点

```text
database.driver      = postgres
redis.enabled        = true
rpc.enabled          = false
app.role             = all
runtime.status_source= runtime_first
runtime.snapshot_ttl = 30s
execution.async_invoke_enabled = false
```

---

## 7. HTTP API 概览

### 7.1 公共接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 存活检查 |
| `GET` | `/ready` | 角色感知 readiness |
| `GET` | `/swagger/*any` | Swagger UI |
| `POST` | `/api/v1/auth/login` | 登录 |
| `POST` | `/api/v1/auth/refresh` | 刷新 Token |

### 7.2 认证后可读接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/auth/logout` | 注销 |
| `GET` | `/api/v1/services` | 服务列表 |
| `GET` | `/api/v1/services/:id` | 服务详情 |
| `GET` | `/api/v1/services/:id/status` | 服务状态 |
| `GET` | `/api/v1/services/:id/tools` | 服务下工具列表 |
| `GET` | `/api/v1/tools/:id` | 工具详情 |
| `GET` | `/api/v1/tasks/:id` | 异步任务详情 |
| `GET` | `/api/v1/history` | 请求历史列表 |
| `GET` | `/api/v1/history/:id` | 请求历史详情 |
| `PUT` | `/api/v1/users/:id/password` | 修改密码 |

### 7.3 需要修改权限的接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/services` | 创建服务 |
| `PUT` | `/api/v1/services/:id` | 更新服务 |
| `DELETE` | `/api/v1/services/:id` | 删除服务 |
| `POST` | `/api/v1/services/:id/connect` | 连接服务 |
| `POST` | `/api/v1/services/:id/disconnect` | 断开服务 |
| `POST` | `/api/v1/services/:id/sync-tools` | 同步工具 |
| `POST` | `/api/v1/tools/:id/invoke` | 同步调用工具 |
| `POST` | `/api/v1/tools/:id/invoke-async` | 异步调用工具 |
| `POST` | `/api/v1/tasks/:id/cancel` | 取消异步任务 |

### 7.4 仅管理员接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/users` | 用户列表 |
| `POST` | `/api/v1/users` | 创建用户 |
| `PUT` | `/api/v1/users/:id` | 更新用户 |
| `DELETE` | `/api/v1/users/:id` | 删除用户 |
| `GET` | `/api/v1/tasks/stats` | 异步任务统计 |
| `GET` | `/api/v1/audit-logs` | 审计日志 |
| `GET` | `/api/v1/audit-logs/export` | 导出审计日志 |

### 7.5 权限链路

```text
未登录
  └─> 只可访问 login / refresh / health / ready / swagger

已登录
  └─> JWT 通过
       ├─> 只读接口
       ├─> RequireModify() -> 修改类接口
       └─> RequireAdmin()  -> 管理类接口
```

---

## 8. 常见工作流

### 8.1 创建并连接一个 MCP 服务

```text
1. POST /api/v1/services
2. POST /api/v1/services/:id/connect
3. POST /api/v1/services/:id/sync-tools
4. GET  /api/v1/services/:id/tools
5. POST /api/v1/tools/:id/invoke
6. GET  /api/v1/history
```

### 8.2 异步调用工具

前提：`execution.async_invoke_enabled=true`

```text
POST /api/v1/tools/:id/invoke-async
          |
          v
       返回 task_id
          |
          +--> GET  /api/v1/tasks/:id
          |
          +--> POST /api/v1/tasks/:id/cancel
```

### 8.3 运行态能力分布

```text
Tool Sync / Invoke / Status
          |
          +--> all 模式: 本地 runtime 直接执行
          |
          +--> control-plane 模式: 通过 internal/rpc 转发给 executor
          |
          +--> executor 模式: 仅负责真正执行
```

---

## 9. 构建、测试与开发命令

### 9.1 常用命令

```bash
go run ./cmd/server
make build
make test
make test-e2e
make test-race
make test-matrix
make test-pg
make vet
make swagger
```

### 9.2 Makefile 实际行为

- `make build`：编译到 `bin/mcp-manager`
- `make test`：执行覆盖率测试、integration/e2e、race
- `make test-e2e`：执行 `tests/integration` 与 `tests/e2e`
- `make test-race`：执行 `go test -race ./...`
- `make test-matrix`：执行数据库矩阵相关包测试
- `make test-pg`：在 `MCP_TEST_POSTGRES_DSN` 存在时执行 PostgreSQL matrix
- `make swagger`：刷新 `api/docs`

### 9.3 测试分层

```text
internal/*_test.go         -> 单元测试
tests/integration          -> 组装后的业务流测试
tests/e2e                  -> 真实 HTTP 场景测试
tests/pgtest               -> PostgreSQL 测试辅助
tests/testutil             -> 测试装配与 HTTP/MCP mock 工具
```

---

## 10. Docker / Compose

### 10.1 默认 all 模式

```bash
docker compose -f deployments/docker/docker-compose.yml --profile all up -d
```

默认会拉起：

- `postgres`
- `redis`
- `mcp-manager-all`

### 10.2 dual-role 模式

```bash
docker compose -f deployments/docker/docker-compose.yml --profile dual-role up -d
```

默认会拉起：

- `postgres`
- `redis`
- `mcp-control-plane`
- `mcp-executor`

### 10.3 Compose 拓扑示意

```text
+-------------------+        +-------------------+
| mcp-control-plane | -----> |   mcp-executor    |
|      :8080        |  RPC   |   :8081 / :18081  |
+----+---------+----+        +----+---------+----+
     |         |                  |         |
     |         +-------+  +-------+         |
     |                 |  |                 |
     v                 v  v                 v
+-------------+    +-------------+    +-------------+
| PostgreSQL  |    |    Redis    |    | MCP Runtime |
|   :5432     |    |   :6379     |    |  执行能力   |
+-------------+    +-------------+    +-------------+

说明：control-plane 与 executor 都会连接 PostgreSQL；若启用 Redis，也都会初始化 Redis 客户端。
只有 executor 持有本地 MCP Runtime，control-plane 通过 RPC 转发运行态操作。
```

### 10.4 容器配置说明

- Docker 镜像默认复制 `deployments/docker/config.prod.yaml` 到容器内 `/app/config.yaml`
- Compose 中默认数据库驱动仍是 `postgres`
- Compose 中默认 Redis 启用
- dual-role 场景下两个进程都会连接 PostgreSQL；若启用 Redis，也都会初始化 Redis 客户端
- dual-role 场景下：
  - control-plane：`MCP_APP_ROLE=control-plane`
  - executor：`MCP_APP_ROLE=executor`
  - 两者都必须开启 `MCP_RPC_ENABLED=true`
  - executor 开启 RPC 监听
  - control-plane 指向 executor RPC 地址

---

## 11. Swagger

启动服务后访问：

```text
http://127.0.0.1:8080/swagger/index.html
```

刷新 Swagger 文档：

```bash
make swagger
```

对应命令：

```bash
swag init -g cmd/server/main.go -o api/docs
```

---

## 12. 重要默认值与限制

### 12.1 默认值

```text
HTTP 端口          : 8080
Executor RPC 端口 : 18081
默认数据库         : PostgreSQL
默认 Redis         : 启用
默认角色           : all
默认管理员         : root / admin123456
```

### 12.2 使用注意事项

1. **直接运行默认依赖 PostgreSQL**  
   如果本地没有 PostgreSQL，请显式切换到 SQLite。

2. **Redis 默认启用**  
   如果你不准备本地 Redis，建议设置 `MCP_REDIS_ENABLED=false`。

3. **executor 不暴露完整业务 API**  
   看到 `/api/v1/...` 404 时，先检查当前 `app.role`。

4. **RPC 默认关闭，但 dual-role 必须开启**  
   `all` 模式可保持关闭；`control-plane` 与 `executor` 模式下若未开启 `rpc.enabled=true`，配置校验会直接失败。

5. **异步调用默认关闭**  
   使用 `/api/v1/tools/:id/invoke-async` 前需要开启 `execution.async_invoke_enabled`。

6. **`/ready` 比 `/health` 更适合作为部署探针**  
   `/ready` 会真正反映角色依赖是否就绪。

---

## 13. 参考文件

如果你要继续深入代码，建议从这些文件开始：

- `cmd/server/main.go`
- `internal/bootstrap/app.go`
- `internal/router/router.go`
- `internal/config/config.go`
- `config.yaml`
- `deployments/docker/docker-compose.yml`
- `deployments/docker/Dockerfile`
- `tests/testutil/app.go`
- `tests/e2e/app_e2e_test.go`

---

## 14. License

如仓库后续补充 License，请以仓库根目录为准。当前 README 未内置额外授权声明。
