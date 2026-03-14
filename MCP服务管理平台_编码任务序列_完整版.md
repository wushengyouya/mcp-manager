# MCP服务管理平台 - 技术执行文档（V1.2）

> 后端服务架构设计与开发指南
> 版本定位：V1 单机版
> 技术栈：Go 1.26 + SQLite + Gin + GORM + mcp-go
> 文档版本：V1.2

---

## 1.项目概述

### V1 范围说明

V1 版本目标为交付一个可在单机环境运行的 MCP 服务管理平台，核心能力包括：

1. 本地账号密码登录与 JWT 鉴权
2. MCP 服务配置管理（`stdio` / `streamable_http` / `sse` 兼容模式）
3. 服务连接、断开、状态查询、健康检查
4. 工具同步、工具调用、调用历史记录
5. 审计日志、邮件告警、Swagger 文档、Docker 部署

以下能力不纳入 V1 交付范围，但在架构设计中保留扩展空间：

1. 单点登录（SSO）
2. 多实例部署
3. 分布式 Token 黑名单
4. PostgreSQL / MySQL 数据库适配
5. 更复杂的集群级调度、服务发现与分布式会话管理

> 说明：`streamable_http` 为 V1 正式支持的远程 MCP 传输方式；`sse` 仅作为旧协议兼容模式保留。

### 项目背景

MCP（Model Context Protocol）是用于模型与外部能力系统化交互的标准协议。本平台作为 MCP 客户端管理平台，负责统一接入并管理多种 MCP 服务，包括本地子进程服务、远程 HTTP MCP 服务以及兼容旧协议的 SSE 服务，提供服务配置管理、连接状态监控、在线工具调用、工具同步、调用历史记录、审计日志和告警能力。

平台采用前后端分离架构，后端使用 Go 语言开发，数据库选用 SQLite，适合单机和中小规模部署。MCP 连接层必须基于 `mcp-go`库 实现，V1 支持 `stdio`、`streamable_http` 和 `sse`（兼容模式）三类传输，其中：

- `stdio`：用于本地子进程 MCP 服务；
- `streamable_http`：用于远程 HTTP MCP 服务，作为首选远程传输方式；
- `sse`：仅用于兼容旧服务或迁移期服务，不作为新接入远程服务的首选。



### 需求澄清结果

根据需求澄清会议，确定以下技术参数和约束条件：

| 澄清项 | 确认结果 |
|--------|----------|
| MCP传输方式 | stdio + streamable_http + sse(兼容模式),采用已有的mcp-go类库 |
| 认证方式 | Bearer Token |
| 连接超时配置 | 可配置，默认30秒 |
| 心跳间隔 | 30秒 |
| ERROR判定阈值 | 连续失败3次 |
| 心跳方式 | 调用MCP的ping方法 |
| 告警渠道 | 邮件通知 |
| Access Token有效期 | 2小时 |
| Refresh Token有效期 | 7天 |
| 单点登录 | 不纳入 V1 交付 |
| 密码加密 | bcrypt |
| 默认管理员 | root / admin123456 |
| 日志保留期限 | 90天 |
| 日志导出 | 支持 |
| Docker部署 | 支持 |
| API 文档 | Swagger / OpenAPI |
| 部署规模 | 单机 / 中小规模部署 |

### 技术选型

技术选型遵循“成熟稳定、易于维护”的原则。Go 语言具有优秀的并发性能和丰富的标准库，适合构建高性能后端服务。SQLite 作为嵌入式数据库，无需独立部署数据库服务，降低运维复杂度。`mcp-go` 类库作为 Go 生态中的 MCP 能力基础设施，用于承载 `stdio`、`streamable_http` 以及 `sse` 兼容模式下的客户端封装与连接管理；其中 `streamable_http` 为 V1 首选远程传输方式，`sse` 仅作为兼容模式保留。

| 层级 | 技术 | 版本 | 用途 |
|------|------|------|------|
| 编程语言 | Go | 1.26 | 后端服务开发 |
| 数据库 | SQLite | 3.x | 数据持久化存储 |
| MCP库 | mcp-go | latest | MCP协议实现 |
| Web框架 | Gin | v1.9+ | HTTP路由和中间件 |
| ORM | GORM | v1.25+ | 数据库访问 |
| 配置管理 | Viper | v1.18+ | 配置文件解析 |
| JWT | jwt-go | v5 | 身份认证 |
| 文档 | Swagger | swag | API文档生成 |
| 日志 | zap | v1.26+ | 结构化日志 |
| 邮件 | gomail | v2 | 告警邮件发送 |

---

## 2.架构设计

### 分层架构

系统采用经典的分层架构模式，从上到下依次为：接口层、应用层、领域层、基础设施层。各层职责明确，通过接口解耦，便于单元测试和模块替换。接口层负责处理HTTP请求和响应，应用层编排业务流程，领域层封装核心业务逻辑，基础设施层提供数据持久化和外部服务集成。

分层架构的核心优势在于关注点分离。每一层只依赖其直接下层，通过依赖注入实现松耦合。接口层不包含业务逻辑，仅负责请求验证、参数转换和响应格式化。应用层协调多个领域服务完成复杂业务流程，但不包含业务规则。领域层是系统的核心，包含所有业务规则和领域模型。基础设施层实现技术细节，如数据库访问、消息队列等。

```text
┌─────────────────────────────────────────────────────────────────┐
│                        接口层 (Handler)                          │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐            │
│  │ MCPHandler   │ │ ToolHandler  │ │ AuthHandler  │            │
│  └──────────────┘ └──────────────┘ └──────────────┘            │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                       应用层 (Service)                           │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌───────────┐ │
│  │ MCPService  │ │ UserService │ │ AuditService│ │ToolService│ │
│  └─────────────┘ └─────────────┘ └─────────────┘ └───────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                     MCP客户端管理层                              │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                MCPConnectionManager                        │  │
│  │  ┌──────────────┐ ┌──────────────────────┐ ┌────────────┐ │  │
│  │  │ StdioClient  │ │ StreamableHTTPClient │ │ SSEClient  │ │  │
│  │  └──────────────┘ └──────────────────────┘ └────────────┘ │  │
│  │  ┌──────────────────────────────────────────────────────┐  │  │
│  │  │                    HealthChecker                     │  │  │
│  │  └──────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                     数据访问层 (Repository)                      │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌────────┐ │
│  │ MCPRepository│ │UserRepository│ │ AuditRepo    │ │ToolRepo│ │
│  └──────────────┘ └──────────────┘ └──────────────┘ └────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                        SQLite 数据库                             │
└─────────────────────────────────────────────────────────────────┘
```

### 传输层策略

1. `stdio`：用于本地子进程 MCP 服务。
2. `streamable_http`：用于远程 HTTP MCP 服务，作为首选远程传输方式。
3. `sse`：仅用于兼容旧服务或迁移期服务。
4. 当服务配置为 `transport_type=streamable_http` 时，默认按标准 Streamable-HTTP 协议处理；仅当 `compat_mode=allow_legacy_sse` 时，才允许在明确识别对端为旧式 SSE 服务后回退到 `SSEClient`。

### Streamable-HTTP 客户端职责

`StreamableHTTPClient` 至少负责以下职责：

1. 初始化连接与协议版本协商；
2. 管理 `MCP-Session-Id`；
3. 为请求注入 `Authorization`、`custom_headers`、`Accept`、`MCP-Protocol-Version`；
4. 兼容 JSON 直接响应与 SSE 流响应；
5. 当 `listen_enabled=true` 时，可选建立独立 `GET` SSE 监听流以接收服务端主动消息；
6. 断开连接时优先发送 `DELETE` 清理会话；
7. 当对端不支持显式会话结束时，允许将 `405 Method Not Allowed` 视为“服务不支持显式结束会话”，而非系统错误。

### Session 状态模型

对 `streamable_http` 服务，连接管理器维护如下运行时状态：

- `session_id`
- `protocol_version`
- `listen_enabled`
- `listen_active`
- `listen_last_error`
- `last_seen_at`
- `transport_capabilities`

### 运行时状态持久化边界

以下字段属于**运行时连接状态**，默认仅保存在内存中的连接管理器或状态对象中，不直接持久化到 `mcp_services` 表：

- `session_id`
- `protocol_version`
- `listen_active`
- `listen_last_error`
- `last_seen_at`
- `transport_capabilities`

数据库中的 `mcp_services` 表只保存静态配置和基础状态字段；运行时状态通过状态查询接口实时返回。

| 层级 | 模块 | 职责说明 |
|------|------|----------|
| 接口层 | Handler | HTTP请求处理、参数校验、响应封装 |
| 接口层 | Middleware | 认证授权、请求日志、CORS处理 |
| 应用层 | Service | 业务流程编排、事务管理 |
| 领域层 | Entity | 领域实体、业务规则封装 |
| 领域层 | VO | 值对象、枚举类型 |
| 基础设施层 | Repository | 数据访问、持久化操作 |
| 基础设施层 | MCPClient | MCP连接管理、健康检查 |

### 模块职责

系统按业务领域划分为多个功能模块，每个模块内聚高、耦合低。MCP服务管理模块是核心模块，负责服务配置的增删改查、连接状态管理、健康检查等功能。工具调用模块负责同步MCP工具元数据、执行工具调用、记录调用历史。用户认证模块提供JWT认证、权限控制、审计日志等功能。

| 模块 | 功能点 | 关键类/接口 |
|------|--------|-------------|
| MCP服务管理 | 服务CRUD | MCPService, MCPRepository |
| MCP服务管理 | 连接管理 | ConnectionManager |
| MCP服务管理 | 健康检查 | HealthChecker |
| 工具调用 | 工具同步 | ToolService |
| 工具调用 | 工具调用 | ToolExecutor |
| 工具调用 | 历史记录 | RequestHistoryRepo |
| 用户认证 | 登录/JWT | AuthService, JWTService |
| 用户认证 | 权限控制 | RBACService |
| 用户认证 | 审计日志 | AuditService |
| 告警通知 | 邮件告警 | AlertService, EmailSender |

## 3.数据库设计

### ER模型概述

数据库设计遵循第三范式，确保数据完整性和一致性。核心实体包括：MCP 服务配置、用户、工具、请求历史、审计日志。用户与服务的访问关系由权限控制实现，服务与工具是一对多关系，服务与请求历史是一对多关系。审计日志独立存储，记录所有关键操作。

数据库表中保留逻辑外键字段及关联索引，用于应用层关联查询与数据校验；SQLite 中不启用数据库级外键约束。相关完整性校验由 Service / Repository 层负责实现。

软删除策略统一如下：

- `users`：支持软删除
- `mcp_services`：支持软删除
- `tools`：支持软删除
- `request_history`：不使用软删除，按保留策略清理
- `audit_logs`：不使用软删除，按保留策略清理

### 表结构设计
- users、mcp_services、tools 支持软删除，增加 deleted_at DATETIME NULL 字段；
request_history、audit_logs 不使用软删除，采用保留期清理策略。

#### mcp_services 表

`mcp_services` 表存储 MCP 服务的完整配置信息，包括连接参数、认证信息、传输策略、兼容模式和基础状态标识。V1 支持 `stdio`、`streamable_http` 和 `sse` 三种传输类型，其中 `streamable_http` 为首选远程传输方式，`sse` 仅用于兼容旧服务。连接状态通过 `status` 字段追踪，支持 `DISCONNECTED`、`CONNECTING`、`CONNECTED`、`ERROR` 四种状态。`failure_count` 字段记录连续失败次数，达到阈值后状态转为 `ERROR`。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | VARCHAR(36) | PK, NOT NULL | UUID主键 |
| name | VARCHAR(100) | NOT NULL, UNIQUE | 服务名称 |
| description | TEXT | NULL | 服务描述 |
| transport_type | VARCHAR(20) | NOT NULL | `stdio` / `streamable_http` / `sse` |
| command | VARCHAR(500) | NULL | `stdio` 命令路径 |
| args | JSON | NULL | `stdio` 参数列表 |
| env | JSON | NULL | `stdio` 环境变量 |
| url | VARCHAR(500) | NULL | 远程 MCP 服务地址 |
| bearer_token | TEXT | NULL | Bearer Token，存储前需加密或脱敏处理 |
| custom_headers | JSON | NULL | 自定义请求头 |
| session_mode | VARCHAR(20) | NOT NULL DEFAULT 'auto' | `auto` / `required` / `disabled` |
| compat_mode | VARCHAR(30) | NOT NULL DEFAULT 'off' | `off` / `allow_legacy_sse` |
| listen_enabled | BOOLEAN | NOT NULL DEFAULT FALSE | 是否启用独立监听流 |
| timeout | INTEGER | NOT NULL DEFAULT 30 | 连接/请求超时（秒） |
| status | VARCHAR(20) | NOT NULL DEFAULT 'DISCONNECTED' | `DISCONNECTED` / `CONNECTING` / `CONNECTED` / `ERROR` |
| failure_count | INTEGER | NOT NULL DEFAULT 0 | 连续失败次数 |
| last_error | TEXT | NULL | 最近一次错误信息 |
| tags | JSON | NULL | 标签列表 |
| created_at | DATETIME | NOT NULL | 创建时间 |
| updated_at | DATETIME | NOT NULL | 更新时间 |
| deleted_at | DATETIME | NULL | 软删除时间 |

> 说明：`session_id`、`protocol_version`、`listen_active` 等字段属于运行时状态，不直接存储在本表中，由连接管理器维护并通过状态接口返回。

#### users 表

users表存储系统用户信息，支持管理员、操作员、只读三种角色。密码使用bcrypt加密存储，确保安全性。is_first_login字段标识用户是否首次登录，强制首次登录修改密码。last_login_at记录最后登录时间，用于安全审计。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | VARCHAR(36) | PK, NOT NULL | UUID主键 |
| username | VARCHAR(50) | NOT NULL, UNIQUE | 用户名 |
| password | VARCHAR(100) | NOT NULL | bcrypt加密密码 |
| email | VARCHAR(100) | NOT NULL, UNIQUE | 邮箱地址 |
| role | VARCHAR(20) | NOT NULL | 角色:admin/operator/readonly |
| is_active | BOOLEAN | DEFAULT TRUE | 是否启用 |
| is_first_login | BOOLEAN | DEFAULT TRUE | 是否首次登录 |
| last_login_at | DATETIME | NULL | 最后登录时间 |
| created_at | DATETIME | NOT NULL | 创建时间 |
| updated_at | DATETIME | NOT NULL | 更新时间 |
| deleted_at | DATETIME | NULL | 软删除时间 |

#### tools 表

tools表存储从MCP服务同步的工具元数据，包括工具名称、描述、参数Schema等信息。通过mcp_service_id关联所属服务，支持工具的启用/禁用状态管理。input_schema字段存储JSON Schema格式的参数定义，用于前端动态生成参数表单。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | VARCHAR(36) | PK, NOT NULL | UUID主键 |
| mcp_service_id | VARCHAR(36) | FK, NOT NULL | 所属服务ID |
| name | VARCHAR(100) | NOT NULL | 工具名称 |
| description | TEXT | NULL | 工具描述 |
| input_schema | JSON | NOT NULL | 参数JSON Schema |
| is_enabled | BOOLEAN | DEFAULT TRUE | 是否启用 |
| synced_at | DATETIME | NOT NULL | 同步时间 |
| created_at | DATETIME | NOT NULL | 创建时间 |
| deleted_at | DATETIME | NULL | 软删除时间 |

#### request_history 表

`request_history` 表记录每次工具调用的完整信息，包括请求参数、响应结果、执行状态、耗时、脱敏处理结果、截断标记、摘要信息和压缩信息。该表用于问题追踪、审计辅助和性能分析。`request_body` 和 `response_body` 存储经过脱敏与治理后的 JSON 数据，避免敏感信息和超大字段直接入库。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | VARCHAR(36) | PK, NOT NULL | UUID主键 |
| mcp_service_id | VARCHAR(36) | FK, NOT NULL | 服务ID |
| tool_name | VARCHAR(100) | NOT NULL | 工具名称 |
| user_id | VARCHAR(36) | FK, NOT NULL | 调用用户ID |
| request_body | JSON | NOT NULL | 脱敏后的请求参数 |
| response_body | JSON | NULL | 脱敏后的响应结果 |
| request_truncated | BOOLEAN | NOT NULL DEFAULT FALSE | 请求是否被截断 |
| response_truncated | BOOLEAN | NOT NULL DEFAULT FALSE | 响应是否被截断 |
| request_hash | VARCHAR(128) | NULL | 原始请求内容摘要或哈希 |
| response_hash | VARCHAR(128) | NULL | 原始响应内容摘要或哈希 |
| request_size | INTEGER | NULL | 原始请求大小（字节） |
| response_size | INTEGER | NULL | 原始响应大小（字节） |
| compression_type | VARCHAR(20) | NOT NULL DEFAULT 'none' | `none` / `gzip` |
| status | VARCHAR(20) | NOT NULL | `success` / `failed` |
| error_message | TEXT | NULL | 错误信息 |
| duration_ms | INTEGER | NOT NULL | 耗时（毫秒） |
| created_at | DATETIME | NOT NULL | 创建时间 |

> 说明：
>
> 1. Repository 层负责对敏感字段进行脱敏或过滤；
> 2. 默认黑名单字段包括：`authorization`、`password`、`secret`、`token`、`refresh_token`；
> 3. 当 `request_body` 或 `response_body` 超过统一大小上限时，仅保存截断后的内容，并将对应的 `*_truncated` 字段置为 `true`；
> 4. 如启用压缩，则 `compression_type` 记录压缩方式，读写逻辑统一由 Repository 层处理。

#### audit_logs 表

audit_logs表记录用户的关键操作审计日志，包括配置变更、删除操作、连接操作等。支持90天数据保留策略，通过定时任务自动清理过期数据。提供日志导出功能，满足合规要求。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | VARCHAR(36) | PK, NOT NULL | UUID主键 |
| user_id | VARCHAR(36) | FK, NULL | 操作用户ID |
| username | VARCHAR(50) | NOT NULL | 操作用户名 |
| action | VARCHAR(50) | NOT NULL | 操作类型 |
| resource_type | VARCHAR(50) | NOT NULL | 资源类型 |
| resource_id | VARCHAR(36) | NULL | 资源ID |
| detail | JSON | NULL | 操作详情 |
| ip_address | VARCHAR(45) | NULL | IP地址 |
| user_agent | VARCHAR(500) | NULL | 用户代理 |
| created_at | DATETIME | NOT NULL | 创建时间 |

---

## 4.API接口设计

### 接口规范

API 路径统一采用 RESTful 风格，基础前缀为 `/api/v1`。响应体统一使用标准 JSON 结构，同时严格区分 **HTTP 状态码** 与 **业务错误码**：

- HTTP 状态码用于表达请求在协议层和资源层是否成功；
- 业务错误码 `code` 用于表达业务语义与可枚举错误类型；
- 成功响应的 `code` 固定为 `0`；
- 失败响应的 `code` 为非 `0` 业务错误码。

**统一响应格式：**

| 字段 | 类型 | 说明 |
|------|------|------|
| code | int | 业务状态码：`0` 表示成功，非 `0` 表示失败 |
| message | string | 提示信息 |
| data | object | 业务数据，可为空 |
| timestamp | int64 | 时间戳（毫秒） |

**HTTP 状态码约定：**

| 场景 | HTTP状态码 |
|------|------------|
| 查询成功 | `200 OK` |
| 创建成功 | `201 Created` |
| 更新成功 | `200 OK` |
| 删除成功 | `200 OK` |
| 参数错误 | `400 Bad Request` |
| 未认证 | `401 Unauthorized` |
| 无权限 | `403 Forbidden` |
| 资源不存在 | `404 Not Found` |
| 资源冲突 | `409 Conflict` |
| 系统内部错误 | `500 Internal Server Error` |

**业务错误码分段：**

- `0`：成功
- `1000-1999`：客户端错误（参数错误、资源不存在、资源冲突等）
- `2000-2999`：认证与权限错误（未授权、Token 过期、权限不足等）
- `3000-3999`：业务错误（服务连接失败、工具调用失败等）
- `5000-5999`：系统错误（内部错误、数据库错误等）

> 说明：除历史兼容需要外，不再采用“所有业务错误一律返回 HTTP 200”的约定。

**响应示例：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "uuid",
    "name": "service-name"
  },
  "timestamp": 1709568000000
}
```

**部分错误码定义：**

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 1001 | 参数错误 |
| 1002 | 资源不存在 |
| 2001 | 认证失败 |
| 2002 | Token过期 |
| 2003 | 权限不足 |
| 3001 | 服务连接失败 |
| 3002 | 工具调用失败 |
| 5001 | 系统内部错误 |

### 服务管理API

服务管理API提供MCP服务的完整生命周期管理，包括创建、查询、更新、删除操作，以及连接控制和状态查询。所有写操作需要管理员或操作员权限，读操作只需登录即可。

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | /api/v1/services | 创建MCP服务 | admin/operator |
| GET | /api/v1/services | 服务列表(分页) | login |
| GET | /api/v1/services/:id | 服务详情 | login |
| PUT | /api/v1/services/:id | 更新服务配置 | admin/operator |
| DELETE | /api/v1/services/:id | 删除服务 | admin/operator |
| POST | /api/v1/services/:id/connect | 连接服务 | admin/operator |
| POST | /api/v1/services/:id/disconnect | 断开连接 | admin/operator |
| GET | /api/v1/services/:id/status | 查询状态 | login |
| POST | /api/v1/services/:id/sync-tools | 同步工具列表 | admin/operator |

#### 创建 Streamable-HTTP 服务请求示例

```json
{
  "name": "remote-mcp-server",
  "description": "远程 MCP 服务",
  "transport_type": "streamable_http",
  "url": "https://example.com/mcp",
  "bearer_token": "your-token",
  "custom_headers": {
    "X-API-Key": "demo-key"
  },
  "session_mode": "auto",
  "listen_enabled": true,
  "timeout": 30,
  "tags": ["remote", "streamable-http"]
}
```

**创建Stdio服务请求示例：**

```json
{
  "name": "local-mcp-server",
  "description": "本地MCP服务",
  "transport_type": "stdio",
  "command": "/usr/local/bin/mcp-server",
  "args": ["--port", "8080"],
  "env": {
    "LOG_LEVEL": "debug"
  },
  "timeout": 30,
  "tags": ["local"]
}
```

#### 服务状态响应补充字段

对 `streamable_http` 服务，状态响应建议额外包含以下**运行时字段**：

- `session_id_exists`
- `protocol_version`
- `listen_enabled`
- `listen_active`
- `listen_last_error`
- `last_seen_at`
- `transport_capabilities`
- `transport_type`

> 说明：
>
> 1. 上述字段由连接管理器实时提供，不要求直接持久化到 `mcp_services` 表；
> 2. `session_id_exists` 用于对外暴露布尔语义，避免直接返回敏感会话标识；
> 3. 当服务为 `stdio` 或 `sse` 时，可仅返回与当前传输方式相关的字段。

### 工具调用API

工具调用API提供工具列表查询、工具调用执行、调用历史查询等功能。工具调用需要先连接MCP服务，系统会自动检查连接状态。调用结果包含响应数据和耗时统计，便于性能分析。

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | /api/v1/services/:id/tools | 工具列表 | login |
| GET | /api/v1/tools/:tool_id | 工具详情 | login |
| POST | /api/v1/tools/:tool_id/invoke | 执行工具调用 | admin/operator |
| GET | /api/v1/history | 调用历史列表 | login |
| GET | /api/v1/history/:id | 调用详情 | login |

**工具调用请求示例：**

```json
{
  "arguments": {
    "query": "hello world",
    "limit": 10
  }
}
```

**工具调用响应示例：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "result": {
      "content": [
        {
          "type": "text",
          "text": "Hello, world!"
        }
      ]
    },
    "duration_ms": 156
  },
  "timestamp": 1709568000000
}
```

### 用户认证API

用户认证 API 提供登录、登出、Token 刷新、密码修改、用户管理等功能。登录成功返回 Access Token 和 Refresh Token。用户管理功能包括用户创建、修改、禁用等操作，仅管理员可执行。审计日志查询支持按时间、用户、操作类型筛选。

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | /api/v1/auth/login | 用户登录 | public |
| POST | /api/v1/auth/logout | 用户登出 | login |
| POST | /api/v1/auth/refresh | 刷新 Token | public |
| PUT | /api/v1/users/:id/password | 修改密码 | self |
| GET | /api/v1/users | 用户列表 | admin |
| POST | /api/v1/users | 创建用户 | admin |
| PUT | /api/v1/users/:id | 更新用户 | admin |
| GET | /api/v1/audit-logs | 审计日志列表 | admin |
| GET | /api/v1/audit-logs/export | 导出审计日志 | admin |

#### Token 刷新接口约束
`POST /api/v1/auth/refresh` 为公开接口，不依赖 Access Token，不挂载认证中间件。该接口仅校验请求体中提供的 Refresh Token 的签名、有效期、黑名单状态和令牌类型。

**登录请求示例：**

```json
{
  "username": "root",
  "password": "admin123456"
}
```

**登录响应示例：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 7200,
    "user": {
      "id": "uuid",
      "username": "root",
      "role": "admin"
    }
  },
  "timestamp": 1709568000000
}
```

---

## 5. 测试策略

### 单元测试

编码任务必须同步编写对应单元测试；涉及鉴权、状态机、序列化、边界校验时优先使用表驱动测试。

测试框架使用Go标准库的testing包，配合testify库提供断言和Mock功能。覆盖率目标设定为80%以上，关键模块如认证、权限控制要求达到90%以上。测试命名遵循`Test{FunctionName}_{Scenario}`格式，便于快速定位问题。

| 模块 | 测试重点 | 覆盖率要求 |
|------|----------|------------|
| Repository | CRUD操作、事务处理、错误处理 | 80% |
| Service | 业务逻辑、边界条件、异常处理 | 85% |
| MCPClient | 连接管理、状态转换、健康检查 | 85% |
| JWTService | Token生成、验证、刷新、过期处理 | 90% |
| AuthService | 登录验证、密码校验、权限检查 | 90% |
| Handler | 参数校验、响应格式、错误码 | 75% |

**单元测试示例：**

```go
func TestMCPService_Create(t *testing.T) {
    tests := []struct {
        name    string
        input   *CreateMCPServiceDTO
        wantErr error
    }{
        {
            name: "create sse service success",
            input: &CreateMCPServiceDTO{
                Name:          "test-service",
                TransportType: "sse",
                URL:           "http://localhost:8080",
            },
            wantErr: nil,
        },
        {
            name: "create service with empty name",
            input: &CreateMCPServiceDTO{
                TransportType: "sse",
                URL:           "http://localhost:8080",
            },
            wantErr: ErrInvalidInput,
        },
        {
            name: "create service with invalid transport type",
            input: &CreateMCPServiceDTO{
                Name:          "test-service",
                TransportType: "invalid",
            },
            wantErr: ErrInvalidTransportType,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### 集成测试

集成测试验证模块间的协作是否正确，重点关注API端到端流程、数据库交互、MCP连接等场景。使用Docker启动临时的测试环境，包括数据库和模拟MCP服务器，确保测试环境的隔离性和可重复性。测试数据通过fixture文件管理，测试完成后自动清理。

集成测试场景包括：用户登录流程、MCP服务创建和连接、工具调用完整流程、权限控制验证、健康检查和状态转换、审计日志记录等。每个场景测试覆盖正常流程和异常流程，验证系统的端到端行为是否符合预期。

| 场景 | 测试步骤 | 验证点 |
|------|----------|--------|
| 用户登录流程 | 1.调用登录API 2.获取Token 3.访问受保护资源 | Token有效性、权限正确 |
| 服务创建流程 | 1.创建服务 2.查询确认 3.审计日志检查 | 数据持久化、审计记录 |
| 连接管理流程 | 1.连接服务 2.状态检查 3.断开连接 | 状态正确转换 |
| 工具调用流程 | 1.同步工具 2.调用工具 3.查看历史 | 调用成功、历史记录 |
| 权限控制验证 | 1.只读用户尝试删除 2.验证返回403 | 权限拦截正确 |
| 健康检查流程 | 1.模拟心跳失败 2.验证状态转ERROR | 状态转换正确 |

**集成测试示例：**

```go
func TestIntegration_MCPServiceLifecycle(t *testing.T) {
    // Setup test environment
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)
    
    router := setupRouter(db)
    
    t.Run("create and connect service", func(t *testing.T) {
        // Create service
        req := httptest.NewRequest("POST", "/api/v1/services", 
            bytes.NewReader(createServiceJSON))
        resp := httptest.NewRecorder()
        router.ServeHTTP(resp, req)
        
        assert.Equal(t, http.StatusCreated, resp.Code)
        
        // Connect service
        req = httptest.NewRequest("POST", "/api/v1/services/{id}/connect", nil)
        resp = httptest.NewRecorder()
        router.ServeHTTP(resp, req)
        
        assert.Equal(t, http.StatusOK, resp.Code)
        
        // Verify status
        req = httptest.NewRequest("GET", "/api/v1/services/{id}/status", nil)
        resp = httptest.NewRecorder()
        router.ServeHTTP(resp, req)
        
        var result map[string]interface{}
        json.Unmarshal(resp.Body.Bytes(), &result)
        assert.Equal(t, "CONNECTED", result["data"].(map[string]interface{})["status"])
    })
}
```

---

# 开发任务执行序列

### 适用场景

本文档适用于以下场景：
- AI智能体自动生成项目代码
- 开发团队参考执行开发任务
- 项目管理者跟踪开发进度
- 代码审查者理解实现逻辑

### 任务格式规范

每个任务严格遵循以下结构：

```
## [任务ID] 任务名称

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | [初始化|编码|测试|配置|集成|文档] |
| 优先级 | [P0-阻塞|P1-关键|P2-重要|P3-普通] |
| 预估工时 | X小时 |
| 依赖任务 | [前置任务ID列表] |
| 所属迭代 | [迭代编号] |

### 任务目标
[任务的核心目标和业务价值]

### 输入规范
[任务开始前必须具备的条件]

### 输出规范
[任务完成后必须交付的产物]

### 实现要点
[关键的实现逻辑和技术要点]

### 验收标准
[明确的可验证的完成标准]

### 验证命令
[验证任务完成的具体命令]
```

### AI执行原则

| 原则 | 说明 |
|------|------|
| 顺序执行 | 优先遵循任务依赖顺序执行；如为修复编译、测试或接口契约问题，允许在同一迭代内做小范围交叉调整 |
| 完整交付 | 每个任务必须完整交付其定义的主要产出文件，并保证代码可编译、可测试 |
| 测试驱动 | 编码任务必须同步编写对应单元测试；涉及鉴权、状态机、序列化、边界校验时优先使用表驱动测试 |
| 版本迭代 | 每个迭代完成后执行总验收测试 |
| 注释规范 | 导出函数、方法、结构体、字段必须添加中文注释，关键逻辑补充必要注释 |
| 错误处理 | 所有错误必须显式处理，不得忽略 |
| 模糊需求 | 若需求影响接口契约、数据模型或安全策略，需先澄清后编码 |
| 任务拆分 | 若任务预计修改超过 8 个文件，或同时影响 3 个以上模块边界，应先拆分子任务；常规的 Handler + DTO + Service + Test 联动修改不视为必须拆分 |
| 代码提交 | 每个 TASK 完成并通过测试后再进行 git 提交 |
| 范围控制 | 不得在未声明的前提下提前实现 SSO、多实例、分布式黑名单、数据库替换等超出 V1 范围的能力 |

---

## 迭代概览(包含 buffer/会议/联调/返工预留 13h)

| 迭代 | 名称 | 任务数 | 预估工时 | 核心交付物 |
|------|------|--------|----------|------------|
| 迭代1 | 项目基础设施 | 8 | 16h | 项目骨架、数据库、日志、配置 |
| 迭代2 | 用户认证模块 | 10 | 26h | JWT认证、权限控制、用户管理、最小审计接口 |
| 迭代3 | MCP服务管理模块 | 10 | 48h | 服务CRUD、连接管理、健康检查、Streamable-HTTP |
| 迭代4 | 工具调用模块 | 6 | 20h | 工具同步、工具调用、历史记录 |
| 迭代5 | 辅助功能模块 | 7 | 26h | 审计日志、告警通知、Swagger文档 |
| **总计** | - | **41** | **136h** | - |

---

## 迭代1: 项目基础设施

### 迭代目标

搭建项目的基础技术架构，包括项目结构初始化、配置管理、日志系统、数据库连接、数据模型定义、API响应规范等基础设施。本迭代是后续所有开发工作的基础，必须优先完成。

---

## TASK-001 初始化Go项目结构

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 初始化 |
| 优先级 | P0-阻塞 |
| 预估工时 | 2小时 |
| 依赖任务 | 无 |
| 所属迭代 | 迭代1 |

### 任务目标

创建符合Go社区最佳实践的标准项目结构，建立模块化的目录布局，配置基础的构建脚本和版本控制文件，为后续开发提供规范化的项目骨架。

### 输入规范

- **环境要求**: Go 1.26 已安装，Git 已配置
- **工作空间**: 空目录，具备读写权限
- **网络环境**: 可访问 Go Module Proxy

### 输出规范

- **目录结构**:
  - `cmd/server/` - 应用入口目录
  - `internal/` - 内部实现代码目录
    - `domain/entity/` - 领域实体
    - `domain/vo/` - 值对象
    - `repository/` - 数据访问层
    - `service/` - 业务服务层
    - `handler/` - HTTP处理器
    - `middleware/` - 中间件
    - `config/` - 配置管理
    - `database/` - 数据库
    - `mcpclient/` - MCP客户端封装
  - `pkg/` - 公共工具包目录
    - `response/` - 统一响应
    - `crypto/` - 加密工具
    - `logger/` - 日志工具
    - `validator/` - 验证器
  - `db/` - 数据库相关
    - `migrations/` - 迁移脚本
    - `seeds/` - 初始数据
  - `api/docs/` - API文档
  - `tests/` - 测试目录
    - `unit/` - 单元测试
    - `integration/` - 集成测试
    - `mocks/` - Mock对象
  - `deployments/docker/` - Docker配置
  - `scripts/` - 脚本工具
  - `data/` - 数据存储

- **配置文件**:
  - `go.mod` - Go模块定义文件，声明模块路径和依赖
  - `go.sum` - 依赖校验文件
  - `Makefile` - 构建脚本，包含常用命令
  - `.gitignore` - Git忽略规则
  - `.dockerignore` - Docker忽略规则

- **入口文件**:
  - `cmd/server/main.go` - 应用主入口，实现基础启动逻辑

### 实现要点

1. **模块初始化**: 使用 `go mod init` 命令初始化Go模块，模块路径遵循 `github.com/{组织名}/{项目名}` 格式

2. **目录规划**: 严格按照Go标准项目布局创建目录，`internal` 目录存放不可被外部引用的内部代码，`pkg` 目录存放可被外部引用的公共代码

3. **Makefile设计**: 定义常用的构建目标，包括 `build`、`run`、`test`、`clean`、`lint` 等，使用变量管理项目名称和路径

4. **入口文件结构**: 主函数应包含配置加载、日志初始化、数据库连接、路由配置、服务启动等核心流程的占位逻辑

5. **Git配置**: 合理配置 `.gitignore`，排除编译产物、IDE配置、环境配置文件、数据库文件、日志文件等

### 验收标准

- [ ] `go mod verify` 执行成功，模块验证通过
- [ ] `go build ./...` 编译无错误
- [ ] 目录结构完整，符合标准布局
- [ ] Makefile可用，`make build` 生成可执行文件
- [ ] 应用入口可运行，输出启动日志

### 验证命令

```bash
go mod verify
go build ./...
make build
./bin/mcp-manager
```

---

## TASK-002 配置Viper配置管理

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P0-阻塞 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-001 |
| 所属迭代 | 迭代1 |

### 任务目标

集成Viper配置管理框架，实现多层级配置加载机制，支持YAML配置文件、环境变量覆盖、默认值设置，确保应用配置灵活且安全。

### 输入规范

- **必需文件**: `go.mod`、`cmd/server/main.go`
- **必需依赖**: `github.com/spf13/viper`
- **前置条件**: 项目结构已初始化

### 输出规范

- **配置结构体文件**: `internal/config/config.go`
  - 定义完整的配置结构体层次
  - 包含Server、Database、JWT、HealthCheck、Audit、Alert、Log等配置项
  - 实现配置加载、默认值设置、配置验证等方法

- **配置文件**: `config.yaml`
  - 默认配置文件
  - 包含所有配置项的默认值
  - 包含必要的注释说明

- **测试文件**: `internal/config/config_test.go`
  - 配置加载测试
  - 配置验证测试
  - 环境变量覆盖测试

### 实现要点

1. **配置结构体设计**: 使用结构体嵌套组织配置层次，每个配置分组定义为独立的子结构体，使用 `mapstructure` 标签支持Viper解析

2. **配置加载流程**: 
   - 设置默认值 → 读取配置文件 → 解析环境变量 → 合并配置 → 验证配置
   - 配置文件不存在时使用默认值运行，不中断启动

3. **环境变量支持**: 设置环境变量前缀为 `MCP`，支持通过环境变量覆盖配置文件中的值，例如 `MCP_SERVER_PORT` 覆盖 `server.port`

4. **配置验证**: 实现配置验证函数，检查端口号范围、日志级别有效性、必填项非空等规则

5. **敏感信息处理**: JWT密钥、数据库密码等敏感配置应支持从环境变量读取，默认配置文件中不应包含生产环境密钥

### 验收标准

- [ ] 配置文件正确解析，结构体字段正确映射
- [ ] 环境变量覆盖功能正常工作
- [ ] 默认值在配置文件缺失时生效
- [ ] 配置验证能拦截无效配置
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/config/...
MCP_SERVER_PORT=9999 go run cmd/server/main.go
```

---

## TASK-003 集成zap日志框架

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P0-阻塞 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-001, TASK-002 |
| 所属迭代 | 迭代1 |

### 任务目标

集成uber-go/zap高性能日志框架，实现结构化日志输出，支持多种日志级别、日志格式选择、日志文件轮转等功能。

### 输入规范

- **必需文件**: `go.mod`、`internal/config/config.go`
- **必需依赖**: `go.uber.org/zap`、`gopkg.in/natefinch/lumberjack.v2`
- **前置条件**: 配置管理已实现

### 输出规范

- **日志模块文件**: `pkg/logger/logger.go`
  - 全局Logger实例管理
  - 日志初始化函数
  - 日志级别解析
  - 便捷日志方法封装

- **测试文件**: `pkg/logger/logger_test.go`
  - 日志初始化测试
  - 日志级别测试
  - 日志格式测试

### 实现要点

1. **全局Logger管理**: 使用 `sync.Once` 确保Logger只初始化一次，提供全局访问方法 `L()` 和 `S()` 分别获取原生Logger和SugaredLogger

2. **日志级别映射**: 将配置中的字符串日志级别映射到zap的 `zapcore.Level`，支持 debug、info、warn、error 四个级别

3. **编码器配置**: 
   - JSON格式：使用 `NewJSONEncoder`，输出结构化JSON日志
   - Console格式：使用 `NewConsoleEncoder`，输出人类可读的控制台日志，带颜色

4. **输出目标配置**:
   - stdout：输出到标准输出
   - 文件路径：使用 `lumberjack` 实现日志文件轮转，配置最大文件大小、备份数量、保留天数

5. **便捷方法封装**: 封装 `Debug`、`Info`、`Warn`、`Error`、`Fatal` 方法，以及对应的格式化版本 `Debugf`、`Infof` 等

### 验收标准

- [ ] 日志正确输出到指定目标
- [ ] JSON格式日志结构正确
- [ ] Console格式日志可读性好
- [ ] 日志级别过滤功能正常
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./pkg/logger/...
go run cmd/server/main.go 2>&1 | head -10
```

---

## TASK-004 集成GORM和SQLite

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P0-阻塞 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-001, TASK-002, TASK-003 |
| 所属迭代 | 迭代1 |

### 任务目标

集成GORM ORM框架和SQLite数据库，实现数据库连接管理、连接池配置、日志集成，为数据访问层提供基础设施。

### 输入规范

- **必需文件**: `go.mod`、`internal/config/config.go`、`pkg/logger/logger.go`
- **必需依赖**: `gorm.io/gorm`、`gorm.io/driver/sqlite`
- **前置条件**: 配置管理和日志系统已实现

### 输出规范

- **数据库模块文件**: `internal/database/database.go`
  - 全局DB实例管理
  - 数据库初始化函数
  - 连接关闭函数
  - 事务辅助函数
  - 健康检查函数

- **测试文件**: `internal/database/database_test.go`
  - 连接初始化测试
  - 连接关闭测试
  - 事务测试
  - 健康检查测试

### 实现要点

1. **数据库连接**：根据配置的驱动类型选择对应的数据库驱动，SQLite 使用文件路径或 `:memory:` 作为 DSN

2. **GORM 配置**：
   - 配置日志适配器，将 GORM 日志桥接到 zap
   - 配置时间函数，统一使用 UTC 时间
   - 配置命名策略，使用蛇形命名转换

3. **SQLite 运行参数**：
   - 启用 WAL 模式
   - 设置 `busy_timeout=10000`（10 秒）
   - 在单机场景下优先保障稳定性而非极限并发吞吐

4. **连接池配置**：
   - SQLite 场景下限制连接数量，避免无意义并发写竞争
   - 推荐 `SetMaxOpenConns(1~4)`，由配置控制
   - 审计日志、调用历史等高频写操作应避免长事务

5. **事务管理**：提供事务辅助函数 `Transaction`，接收一个事务函数作为参数，自动处理提交和回滚

6. **全局实例**：维护全局 DB 实例，提供 `GetDB()` 方法获取，避免频繁传递

7. **外键策略**：表中保留外键字段和索引，但不启用数据库级 FK 约束

### 验收标准

- [ ] 数据库连接成功建立
- [ ] WAL 与 busy_timeout 配置生效
- [ ] 连接健康检查正常
- [ ] 事务提交和回滚功能正常
- [ ] SQL 日志正确输出
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/database/...
go run cmd/server/main.go
```

---

## TASK-005 编写数据库迁移脚本

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P0-阻塞 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-004 |
| 所属迭代 | 迭代1 |

### 任务目标

设计和实现数据库迁移机制，创建所有业务表结构，包括用户表、MCP服务表、工具表、请求历史表、审计日志表，建立必要的索引和约束。

### 输入规范

- **必需文件**: `internal/database/database.go`
- **必需依赖**: `github.com/go-gormigrate/gormigrate/v2`、`github.com/google/uuid`
- **前置条件**: 数据库连接已实现

### 输出规范

- **基础实体文件**: `internal/domain/entity/base.go`
  - 基础实体字段定义（ID、创建时间、更新时间、删除时间）
  - 创建前钩子（自动生成UUID、设置时间戳）
  - 更新前钩子（自动更新时间戳）

- **用户实体文件**: `internal/domain/entity/user.go`
  - 用户实体完整字段定义
  - 角色枚举定义（admin、operator、readonly）
  - 权限判断方法

- **MCP服务实体文件**: `internal/domain/entity/mcp_service.go`
  - MCP服务实体完整字段定义
  - 连接状态枚举定义
  - 传输类型枚举定义
  - JSON字段类型定义（JSONArray、JSONMap）

- **工具实体文件**: `internal/domain/entity/tool.go`
  - 工具实体完整字段定义
  - 与MCP服务的关联关系

- **请求历史实体文件**: `internal/domain/entity/request_history.go`
  - 请求历史实体完整字段定义
  - 请求状态枚举定义
  - 与服务、用户的关联关系

- **审计日志实体文件**: `internal/domain/entity/audit_log.go`
  - 审计日志实体完整字段定义
  - 操作类型字段定义

- **迁移文件**: `internal/database/migrate.go`
  - 迁移函数实现
  - 迁移ID定义

- **测试文件**: `internal/database/migrate_test.go`
  - 迁移执行测试
  - 表结构验证测试

### 实现要点

1. **基础实体设计**: 
   - ID使用UUID格式，存储为VARCHAR(36)
   - 时间字段使用DATETIME类型
   - 支持软删除，使用 `gorm.DeletedAt` 类型

2. **JSON字段处理**: 
   - 数组和Map类型字段使用自定义类型实现 `driver.Valuer` 和 `sql.Scanner` 接口
   - 存储时序列化为JSON字符串，读取时反序列化

3. **枚举类型处理**: 
   - 使用type定义枚举类型，底层为string
   - 定义常量表示枚举值
   - 提供 `IsValid()` 方法验证枚举值有效性

4. **迁移策略**: 
   - 使用gormigrate管理迁移版本
   - 每次迁移使用唯一的迁移ID（格式：日期序列号）
   - 迁移函数中调用 `AutoMigrate` 创建表

5. **索引设计**: 
   - 用户名、邮箱设置唯一索引
   - 服务名设置唯一索引
   - 外键字段设置普通索引
   - 审计日志的操作类型、资源类型设置索引

### 验收标准

- [ ] 所有 5 张核心业务表创建成功
- [ ] 关联字段和索引正确建立
- [ ] 不启用数据库级外键约束
- [ ] 支持软删除的表已包含 `deleted_at` 字段
- [ ] 迁移可重复执行不报错
- [ ] 实体 CRUD 操作测试通过
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/database/...
go run cmd/server/main.go
sqlite3 data/mcp_manager.db ".tables"
```

---

## TASK-006 实现统一响应格式

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-001 |
| 所属迭代 | 迭代1 |

### 任务目标

设计和实现统一的API响应格式，包括成功响应、错误响应、分页响应等，建立完整的错误码体系，确保前后端接口契约一致。

### 输入规范

- **必需文件**: `go.mod`
- **必需依赖**: `github.com/gin-gonic/gin`
- **前置条件**: 项目结构已初始化

### 输出规范

- **响应模块文件**: `pkg/response/response.go`
  - 统一响应结构体定义
  - 分页数据结构体定义
  - 错误码常量定义
  - 错误消息映射
  - 成功响应函数
  - 失败响应函数
  - HTTP错误响应函数

- **业务错误文件**: `pkg/response/errors.go`
  - 业务错误类型定义
  - 错误创建函数
  - 错误类型判断函数

- **测试文件**: `pkg/response/response_test.go`
  - 成功响应测试
  - 失败响应测试
  - 分页响应测试
  - 业务错误测试

### 实现要点


1. **响应结构体设计**:
   - `code`：业务状态码，`0` 表示成功，非 `0` 表示失败
   - `message`：提示信息，人类可读
   - `data`：业务数据，可选
   - `timestamp`：响应时间戳（毫秒级）

2. **错误码规划**:
   - `0`：成功
   - `1000-1999`：客户端错误（参数错误、资源不存在、资源冲突等）
   - `2000-2999`：认证与权限错误（未认证、Token 过期、权限不足等）
   - `3000-3999`：业务错误（服务连接失败、工具调用失败等）
   - `5000-5999`：系统错误（内部错误、数据库错误等）

3. **HTTP 状态码与业务码分离**:
   - HTTP 状态码用于表达请求在协议层和资源层是否成功
   - `code` 用于表达业务语义
   - 创建成功返回 `201 Created`
   - 查询、更新、删除成功返回 `200 OK`
   - 参数错误返回 `400 Bad Request`
   - 未认证返回 `401 Unauthorized`
   - 无权限返回 `403 Forbidden`
   - 资源不存在返回 `404 Not Found`
   - 资源冲突返回 `409 Conflict`
   - 系统错误返回 `500 Internal Server Error`

4. **响应辅助函数**:
   - 提供 `Success()`、`Created()`、`Fail()`、`HTTPError()`、`Page()` 等辅助函数
   - `Fail()` 负责业务错误码封装
   - `HTTPError()` 负责携带明确的 HTTP 状态码输出

5. **兼容约束**:
   - 不再采用“所有业务错误统一返回 HTTP 200”的旧约定
   - Handler 层必须同时保证 HTTP 状态码与业务错误码都符合接口总规范

6. **分页响应**:
   - 分页返回结构中包含 `items`、`page`、`page_size`、`total`
   - 保持与统一响应结构一致，分页数据放入 `data` 字段中

### 验收标准

- [ ] 响应JSON格式符合规范
- [ ] 错误码定义完整覆盖所有场景
- [ ] 分页响应数据结构正确
- [ ] 业务错误可正确识别和处理
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./pkg/response/...
```

---

## TASK-007 实现bcrypt密码加密

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 1小时 |
| 依赖任务 | TASK-001 |
| 所属迭代 | 迭代1 |

### 任务目标

实现基于bcrypt的密码加密和验证功能，提供安全的密码存储和校验机制。

### 输入规范

- **必需文件**: `go.mod`
- **必需依赖**: `golang.org/x/crypto/bcrypt`
- **前置条件**: 项目结构已初始化

### 输出规范

- **密码模块文件**: `pkg/crypto/password.go`
  - 默认cost常量定义
  - 错误常量定义
  - 密码哈希函数
  - 密码验证函数
  - 密码强度验证函数

- **测试文件**: `pkg/crypto/password_test.go`
  - 密码哈希测试（各种密码长度）
  - 密码验证测试
  - 哈希唯一性测试
  - 性能基准测试

### 实现要点

1. **cost因子选择**: 使用默认cost=10，在安全性和性能之间取得平衡，cost越大计算越慢越安全

2. **密码长度验证**: bcrypt最大支持72字节密码，需要验证密码长度在6-72字节范围内

3. **哈希函数设计**: `HashPassword()` 接收明文密码，返回bcrypt哈希字符串，格式为 `$2a$10$...`

4. **验证函数设计**: `CheckPassword()` 接收明文密码和哈希值，返回是否匹配的错误

5. **便捷函数**: 提供 `IsValidPassword()` 返回布尔值，便于业务层调用

### 验收标准

- [ ] 密码可正确加密为bcrypt哈希
- [ ] 正确密码验证通过
- [ ] 错误密码验证失败
- [ ] 相同密码产生不同哈希值
- [ ] 密码长度验证有效
- [ ] 单元测试覆盖率 ≥90%

### 验证命令

```bash
go test -v -cover ./pkg/crypto/...
go test -bench=. ./pkg/crypto/...
```

---

## TASK-008 编写Dockerfile

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 配置 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-001 ~ TASK-007 |
| 所属迭代 | 迭代1 |

### 任务目标

创建Docker镜像构建配置，实现应用的容器化部署支持，包括多阶段构建、镜像优化、健康检查配置。

### 输入规范

- **必需文件**: `go.mod`、`cmd/server/main.go`、`config.yaml`
- **前置条件**: 项目代码可编译

### 输出规范

- **Dockerfile**: `deployments/docker/Dockerfile`
  - 多阶段构建配置
  - 运行时镜像配置
  - 健康检查配置

- **docker-compose文件**: `deployments/docker/docker-compose.yml`
  - 服务定义
  - 卷挂载
  - 环境变量
  - 网络配置

- **生产配置示例**: `deployments/docker/config.prod.yaml`

- **构建脚本**: `scripts/docker-build.sh`

- **忽略文件**: `.dockerignore`

### 实现要点

1. **多阶段构建**:
   - 构建阶段：使用 `golang:1.26-alpine` 镜像，编译二进制文件
   - 运行阶段：使用 `alpine:3.19` 镜像，仅包含必要的运行时依赖

2. **镜像优化**:
   - 使用 `-ldflags="-w -s"` 减小二进制文件体积
   - 使用alpine基础镜像减小镜像体积
   - 使用 `.dockerignore` 排除不必要的文件

3. **安全配置**:
   - 创建非root用户运行应用
   - 设置适当的文件权限
   - 不在镜像中包含敏感配置

4. **健康检查**: 配置HEALTHCHECK指令，定期检查应用健康状态

5. **docker-compose设计**: 定义服务、卷、网络，支持一键启动完整环境

### 验收标准

- [ ] 镜像构建成功
- [ ] 镜像体积合理（<100MB）
- [ ] 容器可正常启动
- [ ] 健康检查正常工作
- [ ] docker-compose可一键启动

### 验证命令

```bash
docker build -t mcp-manager:latest -f deployments/docker/Dockerfile .
docker run -d --name mcp-manager-test -p 8080:8080 mcp-manager:latest
docker ps | grep mcp-manager
docker logs mcp-manager-test
docker stop mcp-manager-test && docker rm mcp-manager-test
```

---

### 迭代1完成检查点

执行以下检查确认迭代1完成：

```bash
# 运行所有测试
go test -v -cover ./...

# 编译检查
go build ./...

# 代码质量检查
go vet ./...

# 启动服务验证
go run cmd/server/main.go &

# Docker构建验证
make docker
```

**迭代验收清单**:
- [ ] 项目可编译无错误
- [ ] 所有单元测试通过
- [ ] 总体测试覆盖率 ≥80%
- [ ] 服务可正常启动
- [ ] 数据库迁移成功
- [ ] Docker镜像可构建

---

## 迭代2: 用户认证模块

### 迭代目标

实现完整的用户认证和授权系统，包括用户管理、JWT认证、权限控制、审计日志记录等功能，为整个平台提供安全基础。

---

## TASK-009 实现UserRepository

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-005 |
| 所属迭代 | 迭代2 |

### 任务目标

实现用户数据访问层，提供用户实体的CRUD操作接口，支持按用户名、邮箱查询，为认证模块提供数据访问支持。

### 输入规范

- **必需文件**: `internal/domain/entity/user.go`、`internal/database/database.go`
- **前置条件**: 用户实体已定义，数据库连接已实现

### 输出规范

- **Repository接口文件**: `internal/repository/user_repository.go`
  - UserRepository接口定义
  - 所有数据访问方法签名

- **Repository实现文件**: `internal/repository/user_repository_impl.go`
  - userRepository结构体定义
  - 所有接口方法的实现
  - 错误常量定义

- **测试文件**: `internal/repository/user_repository_test.go`
  - CRUD操作测试
  - 查询方法测试
  - 边界条件测试

### 实现要点

1. **接口设计**: 定义清晰的Repository接口，包含以下方法：
   - Create、Update、Delete
   - GetByID、GetByUsername、GetByEmail
   - ExistsByUsername、ExistsByEmail
   - List（分页查询）
   - UpdateLastLogin、UpdatePassword、SetFirstLoginFalse

2. **错误处理**: 定义明确的错误类型，如 `ErrUserNotFound`、`ErrUserAlreadyExists`，便于上层区分处理

3. **软删除**: Delete方法使用软删除，记录deleted_at时间戳而非物理删除

4. **分页实现**: List方法支持分页参数，返回记录列表和总数

5. **上下文传递**: 所有方法接收context参数，支持超时控制和请求追踪

### 验收标准

- [ ] 所有CRUD操作正常
- [ ] 按用户名/邮箱查询正确
- [ ] 分页查询功能正常
- [ ] 软删除功能正常
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/repository/...
```

---

## TASK-010 实现密码和JWT工具集成

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-002, TASK-007 |
| 所属迭代 | 迭代2 |

### 任务目标

整合密码加密和JWT工具，创建JWT服务，提供Token生成、解析、刷新功能，实现Token黑名单机制支持登出,登出时至少需要处理 refresh token（比如把 refresh token 也加入黑名单）。

### 输入规范

- **必需文件**: `internal/config/config.go`、`pkg/crypto/password.go`
- **必需依赖**: `github.com/golang-jwt/jwt/v5`
- **前置条件**: 配置管理、密码模块已实现

### 输出规范

- **JWT服务文件**: `pkg/crypto/jwt.go`
  - Claims结构体定义（包含用户ID、用户名、角色）
  - TokenPair结构体定义
  - JWTService结构体和方法
  - Token生成、解析、刷新方法

- **黑名单文件**: `pkg/crypto/blacklist.go`
  - TokenBlacklist结构体
  - 添加、检查、清理方法
  - 全局黑名单实例

- **测试文件**: `pkg/crypto/jwt_test.go`
  - Token生成测试
  - Token解析测试
  - 过期验证测试
  - 刷新Token测试
  - 黑名单测试

### 实现要点

1. JWT Claims 设计包含 `UserID`、`Username`、`Role`、`TokenType`。
2. 生成 Access Token 与 Refresh Token。
3. 解析时校验签名、过期时间、TokenType。
4. V1 使用进程内内存 Map 存储黑名单，至少对 Refresh Token 做黑名单处理。
5. 该黑名单方案仅适用于单实例部署；未来多实例可替换为 Redis 或数据库实现。
6. JWT 密钥必须通过环境变量提供，不得在生产配置文件中写死。

4. **黑名单机制**：
   - V1 版本使用进程内内存 Map 存储已失效 Token
   - 至少对 Refresh Token 做黑名单处理
   - 服务重启后黑名单不保留，该方案仅适用于单实例部署
   - 后续如扩展多实例，替换为 Redis 或数据库实现

5. **配置集成**：从配置读取 JWT 密钥和有效期，密钥必须通过环境变量提供，不得在生产配置文件中写死

### 验收标准

- [ ] Token对正确生成
- [ ] Token正确解析返回Claims
- [ ] 过期Token验证失败
- [ ] 刷新Token生成新Token对
- [ ] 黑名单Token无法使用
- [ ] 单元测试覆盖率 ≥90%

### 验证命令

```bash
go test -v -cover ./pkg/crypto/...
```

---

## TASK-011 实现认证中间件

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-010 |
| 所属迭代 | 迭代2 |

### 任务目标

实现HTTP认证中间件，验证请求中的JWT Token，将用户信息注入请求上下文，为后续权限检查提供基础。

### 输入规范

- **必需文件**: `pkg/crypto/jwt.go`、`pkg/response/response.go`
- **前置条件**: JWT服务已实现

### 输出规范

- **中间件文件**: `internal/middleware/auth.go`
  - Auth中间件函数
  - 上下文键定义
  - 获取当前用户辅助函数

- **测试文件**: `internal/middleware/auth_test.go`
  - 有效Token测试
  - 无效Token测试
  - 过期Token测试
  - 无Token测试

### 实现要点

1. **Token提取**: 从请求头 `Authorization: Bearer {token}` 提取Token

2. **Token验证**: 调用JWT服务验证Token，检查黑名单

3. **上下文注入**: 将用户ID、用户名、角色注入Gin上下文，使用类型安全的上下文键

4. **响应处理**: Token无效时返回401响应，Token过期时返回特殊错误码

5. **辅助函数**: 提供从上下文获取当前用户信息的辅助函数

### 验收标准

- [ ] 有效Token请求正常放行
- [ ] 无Token请求返回401
- [ ] 无效Token请求返回401
- [ ] 过期Token返回特定错误码
- [ ] 用户信息正确注入上下文
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/middleware/...
```

---

## TASK-012 实现权限中间件

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 1小时 |
| 依赖任务 | TASK-011 |
| 所属迭代 | 迭代2 |

### 任务目标

实现基于角色的权限控制中间件，验证用户是否具有执行操作的权限，支持管理员、操作员、只读三种角色。

### 输入规范

- **必需文件**: `internal/middleware/auth.go`、`internal/domain/entity/user.go`
- **前置条件**: 认证中间件已实现

### 输出规范

- **中间件文件**: `internal/middleware/permission.go`
  - RequireRole中间件函数
  - RequireAdmin中间件函数
  - RequireModify中间件函数
  - 权限检查辅助函数

- **测试文件**: `internal/middleware/permission_test.go`
  - 管理员权限测试
  - 操作员权限测试
  - 只读用户权限拒绝测试

### 实现要点

1. **角色检查**: 从上下文获取用户角色，与要求的角色比较

2. **权限层级**: admin > operator > readonly，高角色拥有低角色的所有权限

3. **中间件设计**:
   - `RequireRole(roles...)`: 要求指定角色之一
   - `RequireAdmin()`: 要求管理员角色
   - `RequireModify()`: 要求修改权限（admin或operator）

4. **响应处理**: 权限不足时返回403响应

5. **链式调用**: 权限中间件应在认证中间件之后调用

### 验收标准

- [ ] 管理员可访问所有接口
- [ ] 操作员可访问修改接口
- [ ] 只读用户仅可访问查询接口
- [ ] 权限不足返回403
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/middleware/...
```

---

## TASK-012A 实现最小审计记录接口

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-005 |
| 所属迭代 | 迭代2 |

### 任务目标

实现最小可用的审计记录能力，为认证、用户管理、服务配置变更和连接控制提供统一的审计写入接口，避免主流程依赖迭代5的增强版审计模块。

### 输出规范

- **接口文件**: `internal/service/audit_sink.go`
  - `AuditSink` 接口定义
  - `NoopAuditSink` 实现
  - `DBAuditSink` 实现

- **Repository文件**: `internal/repository/audit_log_repository.go`
  - 最小 `Create` 方法
  - 基础查询能力

- **测试文件**:
  - `internal/service/audit_sink_test.go`
  - `internal/repository/audit_log_repository_test.go`

### 实现要点

1. 定义统一接口：

   ```go
   type AuditSink interface {
       Record(ctx context.Context, entry AuditEntry) error
   }
   ```
2. AuthService、UserService、MCPService 仅依赖 AuditSink，不直接依赖增强版 AuditService。
3. 默认实现为同步写库，优先保证可用性与一致性。
4. 导出、异步写入、清理任务等增强能力仍保留到后续任务实现。

### 验收标准

- [ ] 认证流程可正常写入审计日志

- [ ]  用户管理可正常写入审计日志

- [ ]  MCP 服务配置变更与连接控制可正常写入审计日志

- [ ]  接口可被后续增强版审计服务复用

- [ ]  单元测试覆盖率 ≥85%

### 验证命令
```bash
	go test -v -cover ./internal/service/...
	go test -v -cover ./internal/repository/...
```

---

## TASK-013 实现AuthService

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-009, TASK-010, TASK-012A |
| 所属迭代 | 迭代2 |

### 任务目标

实现认证业务服务层，封装登录、登出、Token刷新、密码修改等核心认证逻辑。

### 输入规范

- **必需文件**: `internal/repository/user_repository.go`、`pkg/crypto/jwt.go`、`pkg/crypto/password.go`
- **前置条件**: UserRepository、JWT服务、密码模块已实现

### 输出规范

- **服务文件**: `internal/service/auth_service.go`
  - AuthService接口定义
  - authService结构体和实现
  - 登录方法
  - 登出方法
  - Token刷新方法
  - 密码修改方法

- **测试文件**: `internal/service/auth_service_test.go`
  - 登录成功测试
  - 登录失败测试
  - Token刷新测试
  - 密码修改测试

### 实现要点

1. **登录流程**:
   - 验证用户名和密码
   - 检查用户状态（是否激活）
   - 生成 Token 对
   - 更新最后登录时间
   - 通过 `AuditSink` 记录审计日志

2. **登出流程**:
   - V1 单机版可采用进程内黑名单或短期黑名单实现
   - 通过 `AuditSink` 记录审计日志

3. **Token刷新**:
   - 验证 Refresh Token
   - 生成新的 Token 对

4. **密码修改**:
   - 验证旧密码
   - 加密存储新密码
   - 更新首次登录标记
   - 通过 `AuditSink` 记录审计日志

5. **错误处理**:
   - 定义明确的业务错误，如“用户名或密码错误”“用户已禁用”等

### 验收标准

- [ ] 登录成功返回Token对
- [ ] 登录失败返回明确错误
- [ ] 登出后Token不可用
- [ ] Token刷新生成新Token对
- [ ] 密码修改后需重新登录
- [ ] 单元测试覆盖率 ≥90%

### 验证命令

```bash
go test -v -cover ./internal/service/...
```

---

## TASK-014 实现UserService

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 3小时 |
| 依赖任务 | TASK-009, TASK-013 |
| 所属迭代 | 迭代2 |

### 任务目标

实现用户管理业务服务层，提供用户的创建、查询、更新、删除等管理功能。

### 输入规范

- **必需文件**: `internal/repository/user_repository.go`、`internal/service/auth_service.go`
- **前置条件**: UserRepository、AuthService已实现

### 输出规范

- **服务文件**: `internal/service/user_service.go`
  - UserService接口定义
  - userService结构体和实现
  - 用户CRUD方法
  - 用户列表查询方法

- **测试文件**: `internal/service/user_service_test.go`
  - 用户创建测试
  - 用户更新测试
  - 用户删除测试
  - 用户列表查询测试

### 实现要点

1. **用户创建**: 
   - 验证用户名、邮箱唯一性
   - 加密密码
   - 设置默认角色
   - 记录审计日志

2. **用户更新**: 
   - 验证用户存在
   - 检查邮箱唯一性（如修改邮箱）
   - 记录审计日志

3. **用户删除**: 
   - 软删除用户
   - 记录审计日志
   - 不能删除自己

4. **列表查询**: 支持分页、按角色过滤、按状态过滤

5. **数据转换**: 定义DTO结构体，避免暴露实体敏感字段

### 验收标准

- [ ] 用户创建成功
- [ ] 重复用户名/邮箱创建失败
- [ ] 用户更新成功
- [ ] 用户删除成功（软删除）
- [ ] 列表分页查询正确
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/service/...
```

---

## TASK-015 实现认证API Handler

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 3小时 |
| 依赖任务 | TASK-011, TASK-012, TASK-013 |
| 所属迭代 | 迭代2 |

### 任务目标

实现认证相关的HTTP处理器，提供登录、登出、Token刷新等API端点。

### 输入规范

- **必需文件**: `internal/service/auth_service.go`、`internal/middleware/auth.go`、`pkg/response/response.go`
- **前置条件**: AuthService、认证中间件已实现

### 输出规范

- **处理器文件**: `internal/handler/auth_handler.go`
  - AuthHandler结构体
  - 登录处理器
  - 登出处理器
  - Token刷新处理器
  - 路由注册函数

- **DTO文件**: `internal/handler/dto/auth_dto.go`
  - 登录请求DTO
  - 登录响应DTO
  - Token刷新请求DTO

- **测试文件**: `internal/handler/auth_handler_test.go`
  - 登录API测试
  - 登出API测试
  - Token刷新API测试

### 实现要点

1. **登录处理器**：
   - 接收用户名和密码
   - 调用 AuthService 登录
   - 返回 Token 对和用户信息

2. **登出处理器**：
   - 从请求头提取 Access Token
   - 从请求体或请求头提取 Refresh Token（如提供）
   - 调用 AuthService 登出
   - 将至少 Refresh Token 加入黑名单
   - 返回成功响应

3. **Token 刷新处理器**：
   - 接收 Refresh Token
   - 仅校验 Refresh Token，不校验 Access Token
   - 调用 AuthService 刷新
   - 返回新 Token 对

4. **参数验证**：使用 Go validator 标签进行请求参数验证

5. **路由设计**：
   - `POST /api/v1/auth/login` - 登录（公开）
   - `POST /api/v1/auth/logout` - 登出（需认证）
   - `POST /api/v1/auth/refresh` - 刷新 Token（公开）

### 验收标准

- [ ] 登录API正常工作
- [ ] 登出API正常工作
- [ ] Token刷新API正常工作
- [ ] 参数验证正确
- [ ] 错误响应格式正确
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/handler/...
```

---

## TASK-016 实现用户管理API Handler

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 3小时 |
| 依赖任务 | TASK-014, TASK-015 |
| 所属迭代 | 迭代2 |

### 任务目标

实现用户管理相关的HTTP处理器，提供用户的创建、查询、更新、删除等API端点。

### 输入规范

- **必需文件**: `internal/service/user_service.go`、`internal/handler/auth_handler.go`
- **前置条件**: UserService已实现

### 输出规范

- **处理器文件**: `internal/handler/user_handler.go`
  - UserHandler结构体
  - 用户CRUD处理器
  - 用户列表处理器
  - 路由注册函数

- **DTO文件**: `internal/handler/dto/user_dto.go`
  - 创建用户请求DTO
  - 更新用户请求DTO
  - 用户响应DTO
  - 用户列表查询DTO

- **测试文件**: `internal/handler/user_handler_test.go`
  - 用户创建API测试
  - 用户更新API测试
  - 用户删除API测试
  - 用户列表API测试

### 实现要点

1. **创建用户处理器**: 接收用户信息，调用UserService创建，返回创建的用户

2. **更新用户处理器**: 接收用户ID和更新信息，调用UserService更新

3. **删除用户处理器**: 接收用户ID，调用UserService删除

4. **用户列表处理器**: 接收分页参数，调用UserService查询，返回分页数据

5. **权限控制**: 
   - 创建用户需admin角色
   - 更新用户需admin角色或自己
   - 删除用户需admin角色
   - 用户列表需admin角色

### 验收标准

- [ ] 用户创建API正常工作
- [ ] 用户更新API正常工作
- [ ] 用户删除API正常工作
- [ ] 用户列表API正常工作
- [ ] 权限控制正确
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/handler/...
```

---

## TASK-017 初始化管理员脚本

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-013, TASK-014 |
| 所属迭代 | 迭代2 |

### 任务目标

实现系统初始化脚本，在首次运行时创建默认管理员账户，确保系统可登录使用。

### 输入规范

- **必需文件**: `internal/service/user_service.go`、`internal/repository/user_repository.go`
- **前置条件**: UserService、数据库迁移已实现

### 输出规范

- **初始化脚本**: `scripts/init_admin.go`
  - 初始化函数
  - 管理员配置

- **集成代码**: 更新 `cmd/server/main.go`
  - 启动时调用初始化
  - 初始化日志输出

- **测试文件**: `scripts/init_admin_test.go`
  - 首次初始化测试
  - 重复初始化测试

### 实现要点

1. **初始化逻辑**: 
   - 检查是否已存在管理员用户
   - 如不存在则创建默认管理员
   - 默认账号：root，默认密码：admin123456

2. **幂等性**: 脚本可重复执行，不会重复创建管理员

3. **安全提示**: 初始化时输出提示信息，建议首次登录后修改密码

4. **配置支持**: 支持通过环境变量覆盖默认账号密码

5. **错误处理**: 初始化失败时记录错误日志，不中断启动

### 验收标准

- [ ] 首次启动创建默认管理员
- [ ] 重复启动不重复创建
- [ ] 默认管理员可正常登录
- [ ] 初始化日志正确输出

### 验证命令

```bash
go run cmd/server/main.go &
# 使用 root/admin123456 登录验证
```

---

## 迭代2完成检查点

执行以下检查确认迭代2完成：

```bash
# 运行所有测试
go test -v -cover ./...

# 启动服务
go run cmd/server/main.go &

# 测试登录API
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"root","password":"admin123456"}'

# 测试需要认证的API
TOKEN="获取到的access_token"
curl -X GET http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN"
```

**迭代验收清单**:
- [ ] 所有单元测试通过
- [ ] JWT认证功能正常
- [ ] 权限控制功能正常
- [ ] 用户管理API正常
- [ ] 默认管理员可登录

---

## 迭代3: MCP服务管理模块

### 迭代目标

实现MCP服务的完整生命周期管理，包括服务配置的增删改查、连接管理、健康检查、工具元数据同步等核心功能。

---

## TASK-018 实现MCPServiceRepository

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-005 |
| 所属迭代 | 迭代3 |

### 任务目标

实现MCP服务数据访问层，提供服务配置的持久化操作。

### 输入规范

- **必需文件**: `internal/domain/entity/mcp_service.go`、`internal/database/database.go`
- **前置条件**: MCP服务实体已定义

### 输出规范

- **Repository接口文件**: `internal/repository/mcp_service_repository.go`
  - MCPServiceRepository接口定义

- **Repository实现文件**: `internal/repository/mcp_service_repository_impl.go`
  - mcpServiceRepository结构体
  - 所有接口方法实现

- **测试文件**: `internal/repository/mcp_service_repository_test.go`
  - CRUD操作测试
  - 状态更新测试
  - 查询过滤测试

### 实现要点

1. `transport_type` 支持 `stdio`、`streamable_http`、`sse`。
2. 正确处理 `args`、`env`、`tags`、`custom_headers` 等 JSON 字段。
3. 支持按 `transport_type` 和标签过滤服务列表。
4. 状态更新方法需兼容 `streamable_http` 的会话态管理。

### 验收标准

- [ ] 所有CRUD操作正常
- [ ] JSON字段正确存储和读取
- [ ] 状态更新方法正常
- [ ] 标签过滤功能正常
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/repository/...
```

---

## TASK-019 实现Stdio客户端封装

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 6小时 |
| 依赖任务 | TASK-018 |
| 所属迭代 | 迭代3 |

### 任务目标

封装mcp-go库的Stdio传输方式，实现与本地MCP服务进程的通信。优先复用 mcp-go 的 client.NewStdioMCPClient / client.NewSSEMCPClient 等能力，再在平台里补齐连接生命周期、重试、认证 header 注入、状态管理等“平台差异”。（mcp-go README/GoDoc 显示其支持 stdio、SSE、streamable-HTTP，并提供相应实现/选项。）

### 输入规范

- **必需文件**: `internal/domain/entity/mcp_service.go`
- **必需依赖**: `github.com/mark3labs/mcp-go`
- **前置条件**: MCP服务实体已定义

### 输出规范

- **客户端接口文件**: `internal/mcpclient/client.go`
  - MCPClient接口定义
  - 通用方法和错误定义

- **Stdio客户端文件**: `internal/mcpclient/stdio_client.go`
  - StdioClient结构体
  - 连接、断开、调用方法
  - 工具列表获取方法

- **测试文件**: `internal/mcpclient/stdio_client_test.go`
  - 连接测试（使用模拟MCP服务）
  - 工具调用测试
  - 错误处理测试

### 实现要点

1. **连接流程**: 
   - 根据配置的命令和参数启动子进程
   - 创建标准输入输出管道
   - 初始化MCP客户端连接

2. **配置支持**: 
   - 支持配置环境变量
   - 支持配置工作目录
   - 支持配置超时时间

3. **生命周期管理**: 
   - 连接成功后记录进程信息
   - 断开时正确终止子进程
   - 处理子进程异常退出

4. **错误处理**: 捕获并封装底层错误，提供清晰的错误信息

5. **资源清理**: 确保连接断开后资源正确释放

### 验收标准

- [ ] 可成功启动并连接本地MCP服务
- [ ] 可获取工具列表
- [ ] 可调用工具并获取结果
- [ ] 可正确断开连接
- [ ] 异常情况正确处理
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/mcpclient/...
```

---

## TASK-020 实现SSE客户端封装

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 6小时 |
| 依赖任务 | TASK-018 |
| 所属迭代 | 迭代3 |

### 任务目标

封装mcp-go库的SSE传输方式，实现与远程MCP服务的HTTP通信。优先复用 mcp-go 的 client.NewStdioMCPClient / client.NewSSEMCPClient 等能力，再在平台里补齐连接生命周期、重试、认证 header 注入、状态管理等“平台差异”。（mcp-go README/GoDoc 显示其支持 stdio、SSE、streamable-HTTP，并提供相应实现/选项。）

### 输入规范

- **必需文件**: `internal/domain/entity/mcp_service.go`
- **必需依赖**: `github.com/mark3labs/mcp-go`
- **前置条件**: MCP服务实体已定义

### 输出规范

- **SSE客户端文件**: `internal/mcpclient/sse_client.go`
  - SSEClient结构体
  - 连接、断开、调用方法
  - 工具列表获取方法
  - 认证头处理

- **测试文件**: `internal/mcpclient/sse_client_test.go`
  - 连接测试（使用模拟HTTP服务）
  - 认证测试
  - 工具调用测试

### 实现要点

1. **连接流程**: 
   - 根据配置的URL建立SSE连接
   - 配置HTTP请求头
   - 初始化MCP客户端

2. **认证支持**: 
   - 支持Bearer Token认证
   - 将Token添加到Authorization请求头
   - 处理认证失败情况

3. **超时配置**: 支持配置连接超时和请求超时

4. **错误处理**: 
   - 处理网络错误
   - 处理HTTP错误状态码
   - 处理SSE连接中断

5. **重连机制**: 可选的自动重连支持

### 验收标准

- [ ] 可成功连接远程MCP服务
- [ ] Bearer Token认证正常
- [ ] 可获取工具列表
- [ ] 可调用工具并获取结果
- [ ] 网络错误正确处理
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/mcpclient/...
```

---

### TASK-020A 实现 StreamableHTTPClient 封装

#### 元数据

| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 8小时 |
| 依赖任务 | TASK-018 |
| 所属迭代 | 迭代3 |

#### 任务目标

封装 `mcp-go` 的 Streamable-HTTP 能力，实现与远程 MCP 服务的连接、初始化、会话管理、工具调用、断开与兼容处理。

#### 输出规范

- `internal/mcpclient/streamable_http_client.go`
- `internal/mcpclient/streamable_http_client_test.go`
- 必要时增加 `internal/mcpclient/http_headers.go`
- 必要时增加 `internal/mcpclient/session.go`

#### 实现要点

1. 根据服务配置创建 HTTP transport，并注入 `Authorization`、`custom_headers`、`Accept`、`MCP-Protocol-Version`。
2. 发送 `InitializeRequest`，保存服务返回的 `MCP-Session-Id` 与协商后的协议版本。
3. 工具调用统一走 `POST`；兼容 `application/json` 与 `text/event-stream` 两种返回。
4. 若 `listen_enabled=true`，额外发起 `GET` SSE 流监听服务器主动消息。
5. `Disconnect()` 时优先发 `DELETE` 结束会话；若服务返回 `405`，按“不支持显式结束会话”处理。
6. 若 `transport_type=streamable_http` 但对端表现为旧 SSE 服务，仅在显式兼容模式下回退到 `SSEClient`。

#### 验收标准

- [ ] 可连接标准 Streamable-HTTP MCP 服务
- [ ] 可保存并复用 `MCP-Session-Id`
- [ ] 可成功执行 `tools/list`、`tools/call`、`ping`
- [ ] 可处理 JSON 响应和 SSE 流响应
- [ ] `Disconnect()` 能发送 `DELETE` 并正确处理 `405`
- [ ] 单元测试覆盖率 ≥85%

---

## TASK-021 实现ConnectionManager

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-019, TASK-020, TASK-020A |
| 所属迭代 | 迭代3 |

### 输入规范

- **必需文件**: `internal/mcpclient/stdio_client.go`、`internal/mcpclient/sse_client.go`、`internal/mcpclient/streamable_http_client.go`
- **前置条件**: Stdio、SSE、Streamable-HTTP 客户端已实现

### 实现要点

1. 客户端创建逻辑从二选一扩展为三选一：`StdioClient`、`StreamableHTTPClient`、`SSEClient`。
2. 管理器根据 `transport_type` 选择具体客户端实现，并统一暴露连接、断开、状态查询、工具调用入口。
3. 管理器维护逻辑连接与可选监听流的组合状态。
4. 新增运行时状态字段：`session_id`、`protocol_version`、`listen_active`、`listen_last_error`、`transport_capabilities`。
5. 对 `streamable_http` 服务，断开时同时关闭本地监听流并尝试发送 `DELETE` 清理会话。
6. 管理器对外返回的状态对象不得直接暴露敏感会话值；如需暴露，统一转换为 `session_id_exists` 等安全字段。

### 验收标准

- [ ] 可成功管理多个连接
- [ ] 并发访问安全
- [ ] 连接状态正确跟踪
- [ ] 断开连接正确清理
- [ ] 单元测试覆盖率 ≥85%

### 验收命令

```bash
go test -v -cover ./internal/mcpclient/...
```

---

## TASK-022 实现健康检查器

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-021 |
| 所属迭代 | 迭代3 |

### 任务目标

实现MCP服务健康检查机制，定期执行心跳检测，自动更新服务状态。

### 输入规范

- **必需文件**: `internal/mcpclient/manager.go`、`internal/repository/mcp_service_repository.go`
- **前置条件**: ConnectionManager已实现

### 输出规范

- **健康检查文件**: `internal/mcpclient/health_checker.go`
  - HealthChecker结构体
  - 启动、停止方法
  - 心跳检查方法
  - 状态更新回调

- **测试文件**: `internal/mcpclient/health_checker_test.go`
  - 心跳成功测试
  - 心跳失败测试
  - 连续失败状态转换测试

### 实现要点

1. **健康检查机制**：
   - 优先使用 MCP 的 `ping` 能力进行健康检查
   - 若目标服务未实现或未声明 `ping`，则回退为轻量探测方式
   - 轻量探测可选方式包括：初始化握手状态检查、工具列表轻量查询、连接状态探测

2. **检查周期**：
   - 每 30 秒执行一次检查
   - 记录每次检查结果与错误信息

3. **状态转换**：
   - 连续 3 次检查失败，状态转为 `ERROR`
   - 任意一次检查成功，状态转为 `CONNECTED`
   - 成功后重置失败计数

4. **定时任务**：使用 `time.Ticker` 实现定时检查，支持优雅停止

5. **并发处理**：每个服务的健康检查独立执行，互不阻塞

6. **错误分类**：
   - 区分“连接断开”“能力不支持”“调用失败”“超时”四类错误
   - 错误信息需写入数据库，便于状态追踪和告警

### 验收标准

- [ ] 心跳检查定期执行
- [ ] 心跳成功状态正确
- [ ] 连续失败状态转为ERROR
- [ ] 错误信息正确记录
- [ ] 可正确停止健康检查
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/mcpclient/...
```

---



## TASK-023 实现MCPService业务服务

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-018, TASK-021, TASK-022, TASK-012A |
| 所属迭代 | 迭代3 |

### 任务目标

实现MCP服务管理业务服务层，封装服务配置管理和连接控制的核心业务逻辑。

### 输入规范

- **必需文件**: `internal/repository/mcp_service_repository.go`、`internal/mcpclient/manager.go`
- **前置条件**: Repository和ConnectionManager已实现

### 输出规范

- **服务文件**: `internal/service/mcp_service.go`
  - MCPService接口定义
  - mcpService结构体
  - 服务CRUD方法
  - 连接/断开方法
  - 状态查询方法

- **测试文件**: `internal/service/mcp_service_test.go`
  - 服务创建测试
  - 服务连接测试
  - 服务删除测试

### 实现要点

1. 创建服务时校验 `transport_type` 与必填字段。
2. 对 `streamable_http`：
   - 校验 `url`
   - 校验可选 `custom_headers`
   - 校验 `session_mode`
   - 校验 `compat_mode`
   - 校验 `listen_enabled`
3. 连接成功后若为远程服务，优先进行初始化协商并回填运行时状态。
4. 配置创建、更新、删除、连接、断开操作均需通过 `AuditSink` 记录审计日志。
5. 审计日志中需记录传输方式与关键连接参数，但敏感头和令牌必须脱敏后再记录

### 验收标准

- [ ] 服务CRUD操作正常
- [ ] 连接/断开功能正常
- [ ] 状态正确更新
- [ ] 审计日志正确记录
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/service/...
```

---

## TASK-024 实现MCP服务API Handler

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-023 |
| 所属迭代 | 迭代3 |

### 任务目标

实现MCP服务管理HTTP处理器，提供服务管理相关API端点。

### 输入规范

- **必需文件**: `internal/service/mcp_service.go`、`internal/middleware/auth.go`
- **前置条件**: MCPService已实现

### 输出规范

- **处理器文件**: `internal/handler/mcp_handler.go`
  - MCPHandler结构体
  - 服务CRUD处理器
  - 连接/断开处理器
  - 状态查询处理器
  - 路由注册函数

- **DTO文件**: `internal/handler/dto/mcp_dto.go`
  - 创建服务请求DTO
  - 更新服务请求DTO
  - 服务响应DTO
  - 服务列表查询DTO

- **测试文件**: `internal/handler/mcp_handler_test.go`
  - 服务创建API测试
  - 服务连接API测试
  - 服务列表API测试

### 实现要点

1. **API设计**:
   - POST /api/v1/services - 创建服务
   - GET /api/v1/services - 服务列表
   - GET /api/v1/services/:id - 服务详情
   - PUT /api/v1/services/:id - 更新服务
   - DELETE /api/v1/services/:id - 删除服务
   - POST /api/v1/services/:id/connect - 连接服务
   - POST /api/v1/services/:id/disconnect - 断开服务
   - GET /api/v1/services/:id/status - 查询状态

2. **权限控制**: 
   - 创建、更新、删除需修改权限
   - 连接、断开需修改权限
   - 查询需登录权限

3. **删除保护**: 删除前检查服务是否已断开连接

4. **参数验证**:
   - `transport_type=stdio` 时必须提供 `command`。
   - `transport_type=streamable_http` 时必须提供 `url`。
   - `transport_type=sse` 时必须提供 `url`。
   - `custom_headers` 为对象，键值必须是字符串。
   - `session_mode` 仅允许 `auto` / `required` / `disabled`。
   - `compat_mode` 仅允许 `off` / `allow_legacy_sse`。
   - `listen_enabled` 必须为布尔值。


### 验收标准

- [ ] 所有API端点正常工作
- [ ] 权限控制正确
- [ ] 参数验证有效
- [ ] 错误响应正确
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/handler/...
```

---

## TASK-025 实现工具同步功能

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 3小时 |
| 依赖任务 | TASK-021 |
| 所属迭代 | 迭代3 |

### 任务目标

实现从MCP服务同步工具元数据的功能，将工具列表持久化到数据库。

### 输入规范

- **必需文件**: `internal/mcpclient/manager.go`、`internal/domain/entity/tool.go`
- **前置条件**: ConnectionManager已实现，Tool实体已定义

### 输出规范

- **Repository文件**: `internal/repository/tool_repository.go`
  - ToolRepository接口和实现

- **服务文件**: `internal/service/tool_service.go`
  - 工具同步方法
  - 工具列表查询方法

- **测试文件**: `internal/service/tool_service_test.go`
  - 工具同步测试
  - 工具列表查询测试

### 实现要点

1. 同步工具前确保远程会话已初始化（如适用）。
2. 对 Streamable-HTTP 服务记录协商协议版本与会话态，便于排查同步问题。
3. 同步失败时记录传输方式、HTTP 状态码（如有）、错误分类。

### 验收标准

- [ ] 工具列表正确同步
- [ ] 增量更新正确
- [ ] 工具状态保留
- [ ] 同步时间正确记录
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/service/...
```

---

## TASK-026 配置Gin路由和启动HTTP服务

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-015, TASK-016, TASK-024 |
| 所属迭代 | 迭代3 |

### 任务目标

配置Gin路由器，注册所有API路由，实现HTTP服务启动和优雅关闭。

### 输入规范

- **必需文件**: 所有Handler文件、中间件文件
- **必需依赖**: `github.com/gin-gonic/gin`
- **前置条件**: 所有Handler已实现

### 输出规范

- **路由文件**: `internal/router/router.go`
  - 路由器创建函数
  - 路由注册函数
  - 中间件配置

- **更新main.go**: 
  - HTTP服务启动
  - 优雅关闭处理

- **测试文件**: `internal/router/router_test.go`
  - 路由注册测试
  - 中间件链测试

### 实现要点

1. **路由分组**: 
   - /api/v1 - API版本前缀
   - 公开路由组（登录等）
   - 认证路由组（需登录）
   - 管理路由组（需管理员权限）

2. **中间件链**: 
   - CORS中间件
   - 请求日志中间件
   - 认证中间件
   - 权限中间件

3. **优雅关闭**: 
   - 监听系统信号
   - 停止接收新请求
   - 等待现有请求完成
   - 关闭数据库连接

4. **健康检查端点**: 提供/health端点用于负载均衡和健康检查

### 验收标准

- [ ] 所有路由正确注册
- [ ] 中间件正确应用
- [ ] HTTP服务可启动
- [ ] 优雅关闭正常
- [ ] 健康检查端点可用

### 验证命令

```bash
go run cmd/server/main.go &
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/auth/login -X POST -d '{"username":"root","password":"admin123456"}'
```

---

## 迭代3完成检查点

执行以下检查确认迭代3完成：

```bash
# 运行所有测试
go test -v -cover ./...

# 启动服务
go run cmd/server/main.go &

# 创建 Streamable-HTTP MCP 服务
curl -X POST http://localhost:8080/api/v1/services \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"test-streamable-service",
    "transport_type":"streamable_http",
    "url":"http://localhost:8081/mcp",
    "session_mode":"auto",
    "compat_mode":"off",
    "listen_enabled":true
  }'

# 连接服务
curl -X POST http://localhost:8080/api/v1/services/{id}/connect \
  -H "Authorization: Bearer $TOKEN"

# 查询服务状态
curl http://localhost:8080/api/v1/services/{id}/status \
  -H "Authorization: Bearer $TOKEN"
```

---

## 迭代4: 工具调用模块

### 迭代目标

实现MCP工具的在线调用功能，包括工具元数据管理、工具调用执行、请求历史记录等。

---

## TASK-027 实现ToolRepository

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-005 |
| 所属迭代 | 迭代4 |

### 任务目标

实现工具数据访问层，提供工具元数据的持久化操作。

### 输入规范

- **必需文件**: `internal/domain/entity/tool.go`、`internal/database/database.go`
- **前置条件**: Tool实体已定义

### 输出规范

- **Repository文件**: `internal/repository/tool_repository.go`
  - ToolRepository接口定义
  - toolRepository结构体和实现

- **测试文件**: `internal/repository/tool_repository_test.go`
  - 工具CRUD测试
  - 按服务查询测试

### 实现要点

1. **接口方法设计**:
   - Create、Update、Delete
   - GetByID、GetByServiceAndName
   - ListByService
   - BatchUpsert（批量同步用）

2. **批量操作**: 实现批量插入/更新，用于工具同步

3. **关联查询**: 支持关联查询服务信息

### 验收标准

- [ ] 工具CRUD操作正常
- [ ] 按服务查询正常
- [ ] 批量操作正常
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/repository/...
```

---

## TASK-028 实现RequestHistoryRepository

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-005 |
| 所属迭代 | 迭代4 |

### 任务目标

实现请求历史数据访问层，提供调用记录的持久化和查询功能。

### 输入规范

- **必需文件**: `internal/domain/entity/request_history.go`
- **前置条件**: RequestHistory实体已定义

### 输出规范

- **Repository文件**: `internal/repository/request_history_repository.go`
  - RequestHistoryRepository接口和实现

- **测试文件**: `internal/repository/request_history_repository_test.go`
  - 历史记录CRUD测试
  - 分页查询测试

### 实现要点

1. 记录请求和响应前，对敏感字段做脱敏或过滤。
2. 默认黑名单字段包括：`authorization`、`password`、`secret`、`token`、`refresh_token`。
3. 对 `request_body` 和 `response_body` 设置统一大小上限；超出时只保留截断后的内容，并记录截断标记。
4. 如需保留完整内容校验能力，可额外存储摘要或哈希值。
5. 大字段允许压缩后存储；压缩与解压逻辑必须在 Repository 层统一实现。

### 验收标准

- [ ] 历史记录存储正常
- [ ] 分页查询正常
- [ ] 过滤功能正常
- [ ] 敏感字段已脱敏或过滤
- [ ] 大字段截断或摘要逻辑生效
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/repository/...
```

---

## TASK-029 实现ToolInvokeService

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-021, TASK-027, TASK-028 |
| 所属迭代 | 迭代4 |

### 任务目标

实现工具调用业务服务，封装工具调用的核心逻辑，包括参数验证、调用执行、结果处理、历史记录。

### 输入规范

- **必需文件**: `internal/mcpclient/manager.go`、工具和历史Repository
- **前置条件**: ConnectionManager、Repository已实现

### 输出规范

- **服务文件**: `internal/service/tool_invoke_service.go`
  - ToolInvokeService接口和实现
  - 工具调用方法
  - 参数验证方法
  - 结果处理方法

- **测试文件**: `internal/service/tool_invoke_service_test.go`
  - 工具调用成功测试
  - 参数验证失败测试
  - 服务未连接测试

### 实现要点

1. 对 Streamable-HTTP 服务，在调用前校验会话是否有效。
2. 若收到 `404 + MCP-Session-Id` 失效语义，按“需重新初始化会话”处理。
3. 调用结果与错误记录中补充 `transport_type`、HTTP 状态码（如适用）、是否流式返回。

### 验收标准

- [ ] 工具调用正常执行
- [ ] 参数验证有效
- [ ] 未连接服务返回错误
- [ ] 调用历史正确记录
- [ ] 耗时统计正确
- [ ] 单元测试覆盖率 ≥90%

### 验证命令

```bash
go test -v -cover ./internal/service/...
```

---

## TASK-030 实现工具调用API Handler

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-025, TASK-029 |
| 所属迭代 | 迭代4 |

### 任务目标

实现工具调用相关HTTP处理器，提供工具列表、工具详情、工具调用等API端点。

### 输入规范

- **必需文件**: `internal/service/tool_service.go`、`internal/service/tool_invoke_service.go`
- **前置条件**: 工具服务已实现

### 输出规范

- **处理器文件**: `internal/handler/tool_handler.go`
  - ToolHandler结构体
  - 工具列表处理器
  - 工具详情处理器
  - 工具调用处理器
  - 同步工具处理器

- **DTO文件**: `internal/handler/dto/tool_dto.go`
  - 工具调用请求DTO
  - 工具调用响应DTO
  - 工具列表响应DTO

- **测试文件**: `internal/handler/tool_handler_test.go`
  - 工具列表API测试
  - 工具调用API测试

### 实现要点

1. **API设计**:
   - GET /api/v1/services/:id/tools - 工具列表
   - GET /api/v1/tools/:id - 工具详情
   - POST /api/v1/tools/:id/invoke - 调用工具
   - POST /api/v1/services/:id/sync-tools - 同步工具

2. **权限控制**: 
   - 查询需登录权限
   - 调用需修改权限

3. **响应设计**: 
   - 工具调用返回结果和耗时
   - 工具列表包含服务信息

### 验收标准

- [ ] 工具列表API正常
- [ ] 工具详情API正常
- [ ] 工具调用API正常
- [ ] 工具同步API正常
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/handler/...
```

---

## TASK-031 实现请求历史API Handler

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-028 |
| 所属迭代 | 迭代4 |

### 任务目标

实现请求历史查询HTTP处理器，提供历史记录查询API。

### 输入规范

- **必需文件**: `internal/repository/request_history_repository.go`
- **前置条件**: RequestHistoryRepository已实现

### 输出规范

- **处理器文件**: `internal/handler/history_handler.go`
  - HistoryHandler结构体
  - 历史列表处理器
  - 历史详情处理器

- **DTO文件**: `internal/handler/dto/history_dto.go`
  - 历史列表查询DTO
  - 历史详情响应DTO

- **测试文件**: `internal/handler/history_handler_test.go`
  - 历史列表API测试
  - 历史详情API测试

### 实现要点

1. **API设计**:
   - GET /api/v1/history - 历史列表（分页）
   - GET /api/v1/history/:id - 历史详情

2. **过滤支持**: 支持按服务ID、工具名、状态、时间范围过滤

3. **权限控制**: 普通用户只能查看自己的调用历史，管理员可查看全部

### 验收标准

- [ ] 历史列表API正常
- [ ] 历史详情API正常
- [ ] 过滤功能正常
- [ ] 权限控制正确
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/handler/...
```

---

## TASK-032 更新路由注册

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P1-关键 |
| 预估工时 | 1小时 |
| 依赖任务 | TASK-030, TASK-031 |
| 所属迭代 | 迭代4 |

### 任务目标

将工具和历史相关Handler注册到路由器。

### 输入规范

- **必需文件**: `internal/handler/tool_handler.go`、`internal/handler/history_handler.go`、`internal/router/router.go`
- **前置条件**: Handler已实现

### 输出规范

- **更新文件**: `internal/router/router.go`
  - 注册工具相关路由
  - 注册历史相关路由

### 实现要点

1. **路由设计**:
   - 工具路由需登录权限
   - 工具调用需修改权限
   - 历史查询需登录权限

2. **路由分组**: 工具和服务关联，使用服务ID作为前缀

### 验收标准

- [ ] 工具路由正确注册
- [ ] 历史路由正确注册
- [ ] 权限中间件正确应用

### 验证命令

```bash
go test -v ./internal/router/...
```

---

## 迭代4完成检查点

执行以下检查确认迭代4完成：

```bash
# 运行所有测试
go test -v -cover ./...

# 测试工具API
curl http://localhost:8080/api/v1/services/{service_id}/tools \
  -H "Authorization: Bearer $TOKEN"

# 测试工具调用
curl -X POST http://localhost:8080/api/v1/tools/{tool_id}/invoke \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"arguments":{...}}'

# 测试历史查询
curl http://localhost:8080/api/v1/history \
  -H "Authorization: Bearer $TOKEN"
```

---

## 迭代5: 辅助功能模块

### 迭代目标

实现审计日志、告警通知、API文档等辅助功能，完善系统的可观测性和运维支持。

---

## TASK-033 实现AuditLogRepository

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P2-重要 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-005 |
| 所属迭代 | 迭代5 |

### 任务目标

实现审计日志数据访问层，提供审计记录的持久化和查询功能。

### 输出规范

- **Repository文件**: `internal/repository/audit_log_repository.go`
  - AuditLogRepository接口和实现

- **测试文件**: `internal/repository/audit_log_repository_test.go`
  - 审计日志存储测试
  - 分页查询测试

### 实现要点

1. **接口方法**: Create、List、DeleteOlderThan

2. **索引设计**: 为用户ID、操作类型、资源类型、创建时间建立索引

3. **批量操作**: 支持批量插入

### 验收标准

- [ ] 审计日志正确存储
- [ ] 分页查询正常
- [ ] 过期数据清理正常

### 验证命令

```bash
go test -v -cover ./internal/repository/...
```

---

## TASK-034 实现AuditService

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P2-重要 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-033 |
| 所属迭代 | 迭代5 |

### 任务目标

实现增强版审计日志业务服务，在最小审计记录能力基础上补充异步写入、查询、导出和清理协调能力。

### 输出规范

- **服务文件**: `internal/service/audit_service.go`
  - AuditService接口和实现
  - 记录审计方法
  - 查询审计方法
  - 导出审计方法

- **测试文件**: `internal/service/audit_service_test.go`
  - 审计记录测试
  - 审计查询测试

### 实现要点

1. **能力定位**：`TASK-012A` 提供最小可用审计写入接口，本任务在其基础上提供增强能力而非替代主流程依赖。
2. **审计记录**：统一封装操作用户、操作类型、资源类型、资源ID、详情、IP地址、用户代理。
3. **异步记录**：可使用 channel 实现异步审计，降低非关键路径延迟。
4. **查询支持**：支持按时间范围、用户、操作类型过滤。
5. **导出功能**：支持导出为 CSV 或 JSON 格式。
6. **兼容要求**：对外保留与 `AuditSink` 一致的记录接口，确保认证和服务管理模块无需感知增强实现细节。

### 验收标准

- [ ] 审计记录正确
- [ ] 查询功能正常
- [ ] 导出功能正常
- [ ] 单元测试覆盖率 ≥85%

### 验证命令

```bash
go test -v -cover ./internal/service/...
```

---

## TASK-035 实现审计日志清理任务

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P2-重要 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-034 |
| 所属迭代 | 迭代5 |

### 任务目标

实现审计日志定时清理任务，自动删除超过保留期限的日志。

### 输出规范

- **清理任务文件**: `internal/task/audit_cleanup.go`
  - 定时任务结构体
  - 启动、停止方法

- **测试文件**: `internal/task/audit_cleanup_test.go`
  - 清理任务测试

### 实现要点

1. **定时执行**: 每天凌晨执行一次清理

2. **保留期限**: 默认保留90天，可通过配置调整

3. **批量删除**: 分批删除避免单次操作数据量过大

### 验收标准

- [ ] 定时任务正确执行
- [ ] 过期数据正确删除
- [ ] 可配置保留期限

### 验证命令

```bash
go test -v -cover ./internal/task/...
```

---

## TASK-036 实现邮件告警Service

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 编码 |
| 优先级 | P2-重要 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-002 |
| 所属迭代 | 迭代5 |

### 任务目标

实现邮件告警服务，在MCP服务连接异常时发送告警邮件。

### 输出规范

- **服务文件**: `internal/service/alert_service.go`
  - AlertService接口和实现
  - 发送告警方法
  - 邮件模板

- **邮件发送文件**: `pkg/email/sender.go`
  - SMTP发送实现

- **测试文件**: `internal/service/alert_service_test.go`
  - 告警发送测试

### 实现要点

1. **告警触发**: 连续健康检查失败达到阈值时触发告警

2. **告警内容**: 包含服务名称、错误信息、时间戳、传输方式、URL 或命令摘要、最近错误分类、会话状态（如适用）

3. **告警静默**: 同一服务告警间隔不小于配置时间

4. **邮件配置**: 支持SMTP配置，包括服务器、端口、认证信息

### 验收标准

- [ ] 告警邮件正确发送
- [ ] 告警内容正确
- [ ] 告警静默正常
- [ ] 单元测试覆盖率 ≥80%

### 验证命令

```bash
go test -v -cover ./internal/service/...
```

---

## TASK-037 集成Swagger文档

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 集成 |
| 优先级 | P2-重要 |
| 预估工时 | 4小时 |
| 依赖任务 | TASK-026, TASK-032 |
| 所属迭代 | 迭代5 |

### 任务目标

集成Swagger/OpenAPI文档，自动生成API文档。

### 输出规范

- **Swagger注解**: 为所有Handler添加Swagger注解
- **文档生成**: `api/docs/` 目录下的文档文件
- **访问端点**: `/swagger/*` 文档访问端点

### 实现要点

1. **安装swag**: 安装swag命令行工具

2. **添加注解**: 
   - API总体描述注解
   - 每个Handler的注解
   - 请求和响应模型注解

3. **生成文档**: 使用 `swag init` 命令生成文档

4. **集成Gin**: 使用 `gin-swagger` 中间件提供文档访问

### 验收标准

- [ ] Swagger文档正确生成
- [ ] 文档可正常访问
- [ ] API描述完整准确

### 验证命令

```bash
swag init -g cmd/server/main.go -o api/docs
go run cmd/server/main.go &
curl http://localhost:8080/swagger/index.html
```

---

## TASK-038 编写集成测试

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 测试 |
| 优先级 | P1-关键 |
| 预估工时 | 6小时 |
| 依赖任务 | TASK-001 ~ TASK-037 |
| 所属迭代 | 迭代5 |

### 任务目标

编写端到端集成测试，验证系统主要流程的正确性。

### 输出规范

- **集成测试文件**: `tests/integration/`
  - 用户认证流程测试
  - MCP服务管理流程测试
  - 工具调用流程测试
  - 权限控制测试

- **测试辅助文件**: `tests/integration/setup.go`
  - 测试环境初始化
  - 测试数据准备
  - 测试清理

### 实现要点

1. **测试环境**: 使用Docker启动测试依赖服务

2. **测试数据**: 使用Fixture准备测试数据

3. **测试场景**:
   - 用户注册登录流程
   - MCP服务创建连接断开流程
   - 工具同步调用流程
   - 权限控制验证
   - 标准 Streamable-HTTP 服务连接测试
   - JSON 响应模式测试
   - SSE 流响应模式测试
   - 独立 GET 监听测试
   - DELETE 会话结束测试
   - 兼容模式测试（旧 SSE 服务）
   - Bearer Token 与自定义 Header 鉴权测试

4. **清理机制**: 每个测试后清理数据，保证测试隔离

### 验收标准

- [ ] 所有集成测试通过
- [ ] 测试场景覆盖主要流程
- [ ] 测试数据正确清理

### 验证命令

```bash
go test -v ./tests/integration/...
```

---

## TASK-039 编写README和部署文档

### 元数据
| 属性 | 值 |
|------|-----|
| 任务类型 | 文档 |
| 优先级 | P2-重要 |
| 预估工时 | 2小时 |
| 依赖任务 | TASK-038 |
| 所属迭代 | 迭代5 |

### 任务目标

编写项目README和部署文档，方便用户理解和使用。

### 输出规范

- **README文件**: `README.md`
  - 项目介绍
  - 功能特性
  - 快速开始
  - 配置说明

- **部署文档**: `docs/deployment.md`
  - Docker部署
  - 环境变量说明
  - 生产环境配置

- **API文档**: `docs/api.md`
  - API使用说明
  - 认证说明
  - 错误码说明

### 实现要点

1. **README结构**: 项目介绍 → 功能特性 → 技术栈 → 快速开始 → 配置说明 → 开发指南

2. **部署文档**: 包含Docker命令、环境变量表、常见问题

3. **使用示例**: 提供curl命令示例

### 验收标准

- [ ] README内容完整
- [ ] 部署步骤可操作
- [ ] 配置说明清晰

### 验证命令

```bash
# 按文档步骤执行验证
```

---

## 迭代5完成检查点

执行以下检查确认迭代5完成：

```bash
# 运行所有测试
go test -v -cover ./...

# 运行集成测试
go test -v ./tests/integration/...

# 检查Swagger文档
curl http://localhost:8080/swagger/index.html

# 构建Docker镜像
make docker

# 运行容器
docker run -d --name mcp-manager -p 8080:8080 mcp-manager:latest
```

---

## 项目完成验收

### 最终验收清单

- [ ] 所有单元测试通过
- [ ] 所有集成测试通过
- [ ] 总体测试覆盖率 ≥80%
- [ ] Docker镜像可构建
- [ ] 容器可正常启动运行
- [ ] Swagger文档可访问
- [ ] README文档完整

### 验收命令

```bash
# 完整测试
go test -v -cover ./...

# 构建
make build

# Docker
make docker
docker run -d --name mcp-manager -p 8080:8080 mcp-manager:latest

# API测试
curl http://localhost:8080/health
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"root","password":"admin123456"}'
```

---

## 附录A: 任务依赖关系图

```
迭代1 (项目基础设施)
├── TASK-001 (项目结构)
│   ├── TASK-002 (配置管理)
│   │   ├── TASK-003 (日志)
│   │   └── TASK-010 (JWT)
│   ├── TASK-004 (数据库)
│   │   └── TASK-005 (迁移)
│   │       ├── TASK-009 (UserRepository)
│   │       ├── TASK-018 (MCPServiceRepository)
│   │       ├── TASK-027 (ToolRepository)
│   │       ├── TASK-028 (RequestHistoryRepository)
│   │       └── TASK-012A (最小审计接口 / AuditSink)
│   ├── TASK-006 (统一响应格式)
│   ├── TASK-007 (密码加密)
│   └── TASK-008 (Dockerfile)

迭代2 (用户认证)
├── TASK-009 (UserRepository)
├── TASK-010 (JWT)
│   └── TASK-011 (认证中间件)
│       └── TASK-012 (权限中间件)
├── TASK-012A (最小审计接口 / AuditSink)
├── TASK-013 (AuthService)
├── TASK-014 (UserService)
├── TASK-015 (AuthHandler)
├── TASK-016 (UserHandler)
└── TASK-017 (初始化管理员)

迭代3 (MCP服务管理)
├── TASK-018 (MCPServiceRepository)
├── TASK-019 (Stdio客户端)
├── TASK-020 (SSE客户端)
├── TASK-020A (StreamableHTTPClient)
├── TASK-021 (ConnectionManager)
├── TASK-022 (健康检查)
├── TASK-023 (MCPService)
├── TASK-024 (MCPHandler)
├── TASK-025 (工具同步)
└── TASK-026 (路由配置)

迭代4 (工具调用)
├── TASK-027 (ToolRepository)
├── TASK-028 (RequestHistoryRepository)
├── TASK-029 (ToolInvokeService)
├── TASK-030 (ToolHandler)
├── TASK-031 (HistoryHandler)
└── TASK-032 (路由更新)

迭代5 (辅助功能增强)
├── TASK-033 (AuditLogRepository增强)
├── TASK-034 (AuditService增强)
├── TASK-035 (审计日志清理任务)
├── TASK-036 (告警服务)
├── TASK-037 (Swagger)
├── TASK-038 (集成测试)
└── TASK-039 (文档)
```

---

## 附录B: 快速命令参考

```bash
# 开发环境
go mod download          # 下载依赖
go mod tidy              # 整理依赖
go run cmd/server/main.go # 运行服务

# 测试
go test ./...            # 运行所有测试
go test -v -cover ./...  # 详细输出+覆盖率
go test -race ./...      # 竞态检测

# 代码质量
go fmt ./...             # 格式化
go vet ./...             # 静态检查
golangci-lint run        # 综合检查

# 构建
go build -o bin/mcp-manager cmd/server/main.go
make build               # Makefile构建

# Docker
make docker              # 构建镜像
docker-compose up -d     # 启动服务

# 文档
swag init -g cmd/server/main.go -o api/docs
```

---

**文档版本**：V1.2
**任务总数**：41 个（不含后续演进项）
**预估总工时**：136 小时
**最后更新**：2026 年 3 月
**维护团队**：MCP 服务管理平台开发组

## 版本记录

| 版本 | 日期 | 说明 |
|------|------|------|
| V1.0 | 2024-01 | 初始版本，完成总体架构、任务拆分与开发指南 |
| V1.1 | 2026-03 | 明确 V1 单机范围；移除 SSO 交付要求；统一 refresh 接口定义；统一数据库外键与软删除口径；补充 SQLite、健康检查、历史记录治理策略 |
| V1.2 | 2026-03 | 修复重复标题；统一 `session_mode` 枚举；补全 `TASK-021` 对 `StreamableHTTPClient` 的依赖与输入；同步架构图、任务统计与文末版本信息；修复 `TASK-012A` Markdown 格式 |