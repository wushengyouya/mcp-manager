# MCP Manager 接口文档

> 本文档根据当前项目路由、Swagger 注解、DTO 绑定与服务层校验整理，面向前端接入使用。
> 业务接口基准路径：`/api/v1`。Swagger UI：`/swagger/index.html`。

## 1. 通用约定

### 1.1 基础地址

本地默认后端地址：

```text
http://127.0.0.1:8080
```

业务接口统一前缀：

```text
/api/v1
```

示例：

```text
GET http://127.0.0.1:8080/api/v1/services
```

### 1.2 认证方式

除登录、刷新 Token、健康检查、Swagger 外，其余业务接口需要在请求头中携带 Access Token：

```http
Authorization: Bearer <access_token>
```

未携带或 Token 无效时，接口返回 `401`。

### 1.3 角色与权限

| 权限级别 | 允许角色 | 说明 |
| --- | --- | --- |
| 公开 | 无需登录 | 登录、刷新 Token、健康检查、Swagger |
| 已登录 | `admin` / `operator` / `readonly` | 查询类接口、登出、修改自己的密码等 |
| 修改权限 | `admin` / `operator` | 创建、更新、删除服务，连接/断开服务，工具调用等 |
| 管理员 | `admin` | 用户管理、审计日志、任务统计 |

用户角色枚举：

```text
admin | operator | readonly
```

### 1.4 统一响应结构

普通成功响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "timestamp": 1710000000000
}
```

分页响应的 `data` 结构：

```json
{
  "items": [],
  "page": 1,
  "page_size": 10,
  "total": 0
}
```

错误响应：

```json
{
  "code": 1001,
  "message": "参数错误说明",
  "timestamp": 1710000000000
}
```

常见业务错误码：

| code | 含义 |
| --- | --- |
| `0` | 成功 |
| `1001` | 参数错误 |
| `1002` | 资源不存在 |
| `1003` | 资源冲突 |
| `2001` | 未认证 / Token 无效 |
| `2002` | Token 已过期 |
| `2003` | 权限不足 |
| `3001` | 服务连接失败 |
| `3002` | 工具调用失败 |
| `3003` | 并发或限流命中 |
| `5001` | 系统错误 |

### 1.5 通用分页参数

以下列表接口使用相同分页规则：

| 参数 | 位置 | 类型 | 必填 | 校验规则 | 默认值 |
| --- | --- | --- | --- | --- | --- |
| `page` | query | integer | 否 | 提供时必须 `> 0` | `1` |
| `page_size` | query | integer | 否 | 提供时必须 `> 0` 且 `<= 100` | `10` |

适用接口：

- `GET /services`
- `GET /users`
- `GET /history`
- `GET /audit-logs`

---

## 2. 运维与文档入口

### 2.1 存活检查

```http
GET /health
```

权限：公开。

响应示例：

```json
{
  "status": "ok"
}
```

### 2.2 就绪检查

```http
GET /ready
```

权限：公开。

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `role` | string | 当前进程角色，可能为 `all` / `control-plane` / `executor` |
| `ready` | boolean | 是否就绪 |
| `checks` | object | 分项检查结果 |
| `reason` | string | 就绪说明，可为空 |

### 2.3 Swagger UI

```http
GET /swagger/index.html
```

权限：公开。

---

## 3. 认证接口

### 3.1 登录

```http
POST /api/v1/auth/login
```

权限：公开。

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `username` | string | 是 | 不能为空 | 用户名 |
| `password` | string | 是 | 不能为空 | 密码 |

请求示例：

```json
{
  "username": "admin",
  "password": "password"
}
```

成功响应 `data`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `access_token` | string | 访问令牌 |
| `refresh_token` | string | 刷新令牌 |
| `expires_in` | integer | Access Token 剩余有效期，单位由后端 Token 服务定义 |
| `user` | object | 当前用户信息 |
| `user.id` | string | 用户 ID |
| `user.username` | string | 用户名 |
| `user.email` | string | 邮箱 |
| `user.role` | string | 用户角色 |
| `user.is_first_login` | boolean | 是否首次登录 |

---

### 3.2 刷新 Token

```http
POST /api/v1/auth/refresh
```

权限：公开。

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `refresh_token` | string | 是 | 不能为空 | 刷新令牌 |

请求示例：

```json
{
  "refresh_token": "<refresh_token>"
}
```

成功响应 `data`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `access_token` | string | 新访问令牌 |
| `refresh_token` | string | 新刷新令牌 |
| `expires_in` | integer | Access Token 剩余有效期 |

---

### 3.3 登出

```http
POST /api/v1/auth/logout
```

权限：已登录。

请求头：

```http
Authorization: Bearer <access_token>
```

请求体可为空；如提供请求体，可传：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `refresh_token` | string | 否 | 无特殊校验 | 需要同时废弃的刷新令牌 |

请求示例：

```json
{
  "refresh_token": "<refresh_token>"
}
```

成功响应 `data`：

```json
{
  "ok": true
}
```

---

## 4. MCP 服务接口

### 4.1 查询服务列表

```http
GET /api/v1/services
```

权限：已登录。

Query 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `page` | integer | 否 | `> 0` | 页码，默认 `1` |
| `page_size` | integer | 否 | `> 0` 且 `<= 100` | 每页大小，默认 `10` |
| `transport_type` | string | 否 | 枚举：`stdio` / `streamable_http` / `sse` | 传输类型 |
| `tag` | string | 否 | 无特殊校验 | 标签过滤 |

---

### 4.2 创建服务

```http
POST /api/v1/services
```

权限：修改权限。

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `name` | string | 是 | 不能为空；服务层会再次去空白校验 | 服务名称 |
| `description` | string | 否 | 无特殊校验 | 服务描述 |
| `transport_type` | string | 是 | 枚举：`stdio` / `streamable_http` / `sse` | 传输类型 |
| `command` | string | 条件必填 | 当 `transport_type=stdio` 时必须提供 | stdio 启动命令 |
| `args` | string[] | 否 | 无特殊校验 | stdio 命令参数 |
| `env` | object | 否 | 值为字符串 | 环境变量 |
| `url` | string | 条件必填 | 当 `transport_type=streamable_http` 或 `sse` 时必须提供 | 远程服务 URL |
| `bearer_token` | string | 否 | 无特殊校验 | 远程服务 Bearer Token |
| `custom_headers` | object | 否 | 值为字符串 | 自定义请求头 |
| `session_mode` | string | 否 | 枚举：`auto` / `required` / `disabled`；为空默认 `auto` | 会话模式 |
| `compat_mode` | string | 否 | 枚举：`off` / `allow_legacy_sse`；为空默认 `off` | 兼容模式 |
| `listen_enabled` | boolean | 否 | 无特殊校验 | 是否启用监听 |
| `timeout` | integer | 否 | `<= 0` 时后端默认改为 `30` | 超时时间 |
| `tags` | string[] | 否 | 无特殊校验 | 标签 |

业务校验：

- `name` 去空白后不能为空。
- `transport_type=stdio` 时，`command` 必须提供。
- `transport_type=streamable_http` 或 `transport_type=sse` 时，`url` 必须提供。
- `transport_type` 不在枚举范围内会返回参数错误。
- 服务名称重复会返回冲突错误。

stdio 示例：

```json
{
  "name": "local-mcp",
  "transport_type": "stdio",
  "command": "node",
  "args": ["server.js"],
  "env": {
    "NODE_ENV": "production"
  },
  "session_mode": "auto",
  "compat_mode": "off",
  "timeout": 30,
  "tags": ["local"]
}
```

远程服务示例：

```json
{
  "name": "remote-mcp",
  "transport_type": "streamable_http",
  "url": "https://example.com/mcp",
  "bearer_token": "token",
  "custom_headers": {
    "X-Client": "mcp-manager"
  },
  "session_mode": "auto",
  "compat_mode": "off",
  "timeout": 30,
  "tags": ["remote"]
}
```

---

### 4.3 获取服务详情

```http
GET /api/v1/services/{id}
```

权限：已登录。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

---

### 4.4 更新服务

```http
PUT /api/v1/services/{id}
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

请求体：同 [4.2 创建服务](#42-创建服务)。

> 注意：更新接口当前使用与创建接口相同的 `UpsertServiceRequest`，因此 `name` 和 `transport_type` 仍为必填。

---

### 4.5 删除服务

```http
DELETE /api/v1/services/{id}
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

成功响应 `data`：

```json
{
  "ok": true
}
```

---

### 4.6 连接服务

```http
POST /api/v1/services/{id}/connect
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

业务规则：

- 连接失败时可能返回 `502` / `3001`。
- 当服务配置为 `session_mode=required` 但服务端未返回会话时，连接失败。
- 当服务配置为 `session_mode=disabled` 但服务端返回会话时，连接失败。

---

### 4.7 断开服务

```http
POST /api/v1/services/{id}/disconnect
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

成功响应 `data`：

```json
{
  "ok": true
}
```

---

### 4.8 查询服务状态

```http
GET /api/v1/services/{id}/status
```

权限：已登录。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

服务状态枚举：

```text
DISCONNECTED | CONNECTING | CONNECTED | ERROR
```

可能返回的 `data` 字段包括：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `status` | string | 当前综合状态 |
| `persisted_status` | string | 数据库持久化状态 |
| `runtime_status` | string | 运行时状态，存在运行时状态时返回 |
| `last_error` | string | 最近错误 |
| `failure_count` | integer | 失败次数 |
| `transport_type` | string | 传输类型 |
| `session_id_exists` | boolean | 是否存在会话 ID |
| `protocol_version` | string | 协议版本 |
| `listen_enabled` | boolean | 是否启用监听 |
| `listen_active` | boolean | 监听是否活跃 |
| `listen_last_error` | string | 监听最近错误 |
| `last_seen_at` | string | 最近观测时间 |
| `last_used_at` | string | 最近使用时间 |
| `in_flight` | integer | 进行中的请求数 |
| `transport_capabilities` | object | 传输能力 |

---

## 5. 工具与任务接口

### 5.1 查询服务下工具列表

```http
GET /api/v1/services/{id}/tools
```

权限：已登录。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

---

### 5.2 同步工具列表

```http
POST /api/v1/services/{id}/sync-tools
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 服务 ID |

业务规则：

- 服务处于 `ERROR` 状态时，同步工具可能返回冲突错误，需要先恢复连接。

---

### 5.3 查询工具详情

```http
GET /api/v1/tools/{id}
```

权限：已登录。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 工具 ID |

---

### 5.4 同步调用工具

```http
POST /api/v1/tools/{id}/invoke
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 工具 ID |

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `arguments` | object | 是 | 不能为空 | 工具调用参数，结构由具体 MCP 工具定义 |

请求示例：

```json
{
  "arguments": {
    "query": "hello"
  }
}
```

业务规则：

- 调用失败可能返回 `502` / `3002`。
- 具体 `arguments` 字段由工具自身 schema 决定，后端只要求其为必填对象。

---

### 5.5 异步调用工具

```http
POST /api/v1/tools/{id}/invoke-async
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 工具 ID |

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `arguments` | object | 是 | 不能为空 | 工具调用参数，结构由具体 MCP 工具定义 |
| `timeout_ms` | integer | 否 | `<= 0` 时使用后端默认异步任务超时 | 任务超时时间，毫秒 |

请求示例：

```json
{
  "arguments": {
    "query": "hello"
  },
  "timeout_ms": 30000
}
```

业务规则：

- 需要后端启用异步调用能力；未启用时返回冲突类错误。
- 队列满时返回 `429` / `3003`。
- 成功创建任务返回 HTTP `202`。

成功响应 `data` 可能包含：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 任务 ID |
| `request_id` | string | 请求 ID |
| `tool_id` | string | 工具 ID |
| `service_id` | string | 服务 ID |
| `status` | string | 任务状态 |
| `timeout_ms` | integer | 任务超时时间 |
| `queue_length` | integer | 队列长度 |
| `queue_capacity` | integer | 队列容量 |

任务状态枚举：

```text
pending | running | succeeded | failed | cancelled | timed_out
```

---

### 5.6 查询异步任务详情

```http
GET /api/v1/tasks/{id}
```

权限：已登录。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 任务 ID |

权限规则：

- `admin` 可查询所有任务。
- 非 `admin` 只能查询自己创建的任务。

---

### 5.7 取消异步任务

```http
POST /api/v1/tasks/{id}/cancel
```

权限：修改权限。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 任务 ID |

业务规则：

- 任务不存在返回 `404`。
- 非 `admin` 只能取消自己创建的任务。
- 任务已处于终态时，返回当前任务状态，不再重复取消。
- 成功受理返回 HTTP `202`。

---

### 5.8 查询异步任务统计

```http
GET /api/v1/tasks/stats
```

权限：管理员。

响应 `data` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `pending` | integer | 等待中任务数 |
| `running` | integer | 执行中任务数 |
| `succeeded` | integer | 成功任务数 |
| `failed` | integer | 失败任务数 |
| `cancelled` | integer | 已取消任务数 |
| `timed_out` | integer | 超时任务数 |
| `queue_length` | integer | 队列长度 |
| `queue_capacity` | integer | 队列容量 |
| `executor_in_flight` | integer | 执行器当前并发 |
| `executor_limit` | integer | 执行器并发上限 |
| `service_rate_limit` | integer | 服务级限流值 |
| `user_rate_limit` | integer | 用户级限流值 |
| `rate_limit_window_ms` | integer | 限流窗口，毫秒 |

---

## 6. 调用历史接口

### 6.1 查询调用历史列表

```http
GET /api/v1/history
```

权限：已登录。

Query 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `page` | integer | 否 | `> 0` | 页码，默认 `1` |
| `page_size` | integer | 否 | `> 0` 且 `<= 100` | 每页大小，默认 `10` |
| `service_id` | string | 否 | 无特殊校验 | 服务 ID |
| `tool_name` | string | 否 | 无特殊校验 | 工具名 |
| `status` | string | 否 | 代码未强制枚举；建议使用 `success` / `failed` | 调用状态 |
| `start_at` | string | 否 | 如提供，必须符合 RFC3339 格式 | 开始时间 |
| `end_at` | string | 否 | 如提供，必须符合 RFC3339 格式 | 结束时间 |

时间示例：

```text
2026-04-29T10:00:00Z
```

权限规则：

- `admin` 可查看全部历史。
- 非 `admin` 只能查看自己的历史。

调用状态枚举：

```text
success | failed
```

---

### 6.2 查询调用历史详情

```http
GET /api/v1/history/{id}
```

权限：已登录。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 历史记录 ID |

权限规则：

- `admin` 可查看全部历史详情。
- 非 `admin` 只能查看自己的历史详情，否则返回 `403`。

---

## 7. 用户接口

### 7.1 查询用户列表

```http
GET /api/v1/users
```

权限：管理员。

Query 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `page` | integer | 否 | `> 0` | 页码，默认 `1` |
| `page_size` | integer | 否 | `> 0` 且 `<= 100` | 每页大小，默认 `10` |
| `role` | string | 否 | 枚举：`admin` / `operator` / `readonly` | 用户角色 |
| `active` | boolean | 否 | 布尔值 | 是否启用 |

---

### 7.2 创建用户

```http
POST /api/v1/users
```

权限：管理员。

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `username` | string | 是 | 不能为空 | 用户名 |
| `password` | string | 是 | 不能为空 | 密码 |
| `email` | string | 是 | 不能为空，必须是合法邮箱格式 | 邮箱 |
| `role` | string | 是 | 枚举：`admin` / `operator` / `readonly` | 用户角色 |

请求示例：

```json
{
  "username": "operator1",
  "password": "password",
  "email": "operator1@example.com",
  "role": "operator"
}
```

业务规则：

- 用户名或邮箱重复时可能返回冲突错误。

---

### 7.3 更新用户

```http
PUT /api/v1/users/{id}
```

权限：管理员。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 用户 ID |

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `email` | string | 否 | 如提供，必须是合法邮箱格式 | 邮箱 |
| `role` | string | 否 | 如提供，必须为 `admin` / `operator` / `readonly` | 用户角色 |
| `is_active` | boolean | 否 | 布尔值 | 是否启用 |

业务校验：

- `email`、`role`、`is_active` 至少提供一个，否则返回 `400`。

请求示例：

```json
{
  "email": "new@example.com",
  "role": "readonly",
  "is_active": true
}
```

---

### 7.4 删除用户

```http
DELETE /api/v1/users/{id}
```

权限：管理员。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 用户 ID |

成功响应 `data`：

```json
{
  "ok": true
}
```

---

### 7.5 修改密码

```http
PUT /api/v1/users/{id}/password
```

权限：已登录。

Path 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | string | 是 | 不能为空 | 用户 ID |

请求体：

| 字段 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `old_password` | string | 是 | 不能为空 | 原密码 |
| `new_password` | string | 是 | 不能为空 | 新密码 |

权限规则：

- 用户只能修改自己的密码。
- `admin` 可以修改其他用户密码。

成功响应 `data`：

```json
{
  "ok": true
}
```

---

## 8. 审计日志接口

### 8.1 查询审计日志

```http
GET /api/v1/audit-logs
```

权限：管理员。

Query 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `page` | integer | 否 | `> 0` | 页码，默认 `1` |
| `page_size` | integer | 否 | `> 0` 且 `<= 100` | 每页大小，默认 `10` |
| `user_id` | string | 否 | 无特殊校验 | 用户 ID |
| `action` | string | 否 | 无特殊校验 | 操作类型 |
| `resource_type` | string | 否 | 无特殊校验 | 资源类型 |

---

### 8.2 导出审计日志 CSV

```http
GET /api/v1/audit-logs/export
```

权限：管理员。

Query 参数：

| 参数 | 类型 | 必填 | 校验规则 | 说明 |
| --- | --- | --- | --- | --- |
| `action` | string | 否 | 无特殊校验 | 操作类型 |

响应：`text/csv; charset=utf-8`。

响应头包含：

```http
Content-Disposition: attachment; filename=audit_logs.csv
```

---

## 9. 前端接入建议校验清单

前端表单建议按以下规则提前校验，减少无效请求：

1. 分页参数：`page > 0`，`page_size` 范围为 `1~100`。
2. 用户角色：只能选择 `admin`、`operator`、`readonly`。
3. 邮箱字段：创建用户必填且必须合法；更新用户如填写也必须合法。
4. 服务传输类型：只能选择 `stdio`、`streamable_http`、`sse`。
5. 服务创建/更新：
   - `name` 必填。
   - `transport_type=stdio` 时，`command` 必填。
   - `transport_type=streamable_http` 或 `sse` 时，`url` 必填。
   - `session_mode` 只能为 `auto`、`required`、`disabled`。
   - `compat_mode` 只能为 `off`、`allow_legacy_sse`。
6. 工具调用：`arguments` 必填，且必须是对象。
7. 历史查询时间：`start_at`、`end_at` 使用 RFC3339 格式，例如 `2026-04-29T10:00:00Z`。
8. 用户更新：`email`、`role`、`is_active` 至少提供一个。
9. 修改密码：`old_password`、`new_password` 均必填。

## 10. 覆盖说明

本文档覆盖当前运行时注册的 28 个 `/api/v1` 业务接口，并补充了 3 个非业务入口：

- `GET /health`
- `GET /ready`
- `GET /swagger/index.html`

注意：当服务以 `executor` 角色运行时，后端只注册 `/health`、`/ready`、`/swagger/*any`，不会注册 `/api/v1` 控制面业务接口。前端管理后台应连接 `all` 或 `control-plane` 角色实例。

---

## 11. 响应数据模型

本节补充各接口 `data` 的实际结构。除 CSV 导出外，业务接口均使用 [1.4 统一响应结构](#14-统一响应结构)。下方“响应 data”均指统一响应中的 `data` 字段。

### 11.1 BaseModel

多数实体包含通用字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 资源 ID |
| `created_at` | string | 创建时间，RFC3339 格式 |
| `updated_at` | string | 更新时间，RFC3339 格式 |
| `deleted_at` | object/null | 软删除字段，通常前端可忽略 |

### 11.2 User

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 用户 ID |
| `username` | string | 用户名 |
| `email` | string | 邮箱 |
| `role` | string | 角色：`admin` / `operator` / `readonly` |
| `is_active` | boolean | 是否启用 |
| `is_first_login` | boolean | 是否首次登录 |
| `last_login_at` | string/null | 最近登录时间 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

> `password`、`token_version` 不会通过 JSON 响应返回。

### 11.3 MCPService

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 服务 ID |
| `name` | string | 服务名称 |
| `description` | string | 服务描述 |
| `transport_type` | string | `stdio` / `streamable_http` / `sse` |
| `command` | string | stdio 命令 |
| `args` | string[] | stdio 参数 |
| `env` | object | 环境变量，值为字符串 |
| `url` | string | 远程服务 URL |
| `bearer_token` | string | Bearer Token；为空时可能省略 |
| `custom_headers` | object | 自定义请求头，值为字符串 |
| `session_mode` | string | `auto` / `required` / `disabled` |
| `compat_mode` | string | `off` / `allow_legacy_sse` |
| `listen_enabled` | boolean | 是否启用监听 |
| `timeout` | integer | 超时时间 |
| `status` | string | `DISCONNECTED` / `CONNECTING` / `CONNECTED` / `ERROR` |
| `failure_count` | integer | 失败次数 |
| `last_error` | string | 最近错误 |
| `tags` | string[] | 标签 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

### 11.4 Tool

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 工具 ID |
| `mcp_service_id` | string | 所属 MCP 服务 ID |
| `name` | string | 工具名称 |
| `description` | string | 工具描述 |
| `input_schema` | object | 工具入参 JSON Schema，由 MCP 服务返回 |
| `is_enabled` | boolean | 是否启用 |
| `synced_at` | string | 最近同步时间 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

### 11.5 RequestHistory

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 历史记录 ID |
| `mcp_service_id` | string | MCP 服务 ID |
| `tool_name` | string | 工具名 |
| `user_id` | string | 调用用户 ID |
| `request_body` | object | 请求体，敏感字段会被脱敏 |
| `response_body` | object | 响应体，可能被截断 |
| `request_truncated` | boolean | 请求体是否被截断 |
| `response_truncated` | boolean | 响应体是否被截断 |
| `request_hash` | string | 原始请求摘要 |
| `response_hash` | string | 原始响应摘要 |
| `request_size` | integer | 原始请求大小，字节 |
| `response_size` | integer | 原始响应大小，字节 |
| `compression_type` | string | 历史压缩类型 |
| `status` | string | `success` / `failed` |
| `error_message` | string | 错误信息 |
| `duration_ms` | integer | 调用耗时，毫秒 |
| `created_at` | string | 创建时间 |

### 11.6 AuditLog

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 审计日志 ID |
| `user_id` | string | 用户 ID |
| `username` | string | 用户名 |
| `action` | string | 操作动作 |
| `resource_type` | string | 资源类型 |
| `resource_id` | string | 资源 ID |
| `detail` | object | 审计详情 |
| `ip_address` | string | IP 地址 |
| `user_agent` | string | User-Agent |
| `created_at` | string | 创建时间 |

### 11.7 RuntimeStatus

`POST /services/{id}/connect` 返回该结构，`GET /services/{id}/status` 会返回它的聚合/展开字段。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `service_id` | string | 服务 ID；连接接口返回 |
| `id` | string | 服务 ID；状态接口返回 |
| `name` | string | 服务名称；状态接口返回 |
| `status` | string | 服务状态 |
| `transport_type` | string | 实际传输类型或配置传输类型 |
| `session_id_exists` | boolean | 是否存在会话 ID |
| `protocol_version` | string | MCP 协议版本，可能省略 |
| `listen_enabled` | boolean | 是否启用监听 |
| `listen_active` | boolean | 监听是否活跃 |
| `listen_last_error` | string | 监听最近错误，可能省略 |
| `last_seen_at` | string/null | 最近观测时间 |
| `last_used_at` | string/null | 最近使用时间 |
| `in_flight` | integer | 当前进行中的请求数 |
| `transport_capabilities` | object | 传输能力，可能省略 |
| `last_error` | string | 最近错误，可能省略 |
| `failure_count` | integer | 失败次数 |
| `status_source` | string | 状态来源：`persisted` / `runtime` / `snapshot`；状态接口返回 |
| `snapshot_freshness` | string | 快照新鲜度：`missing` / `stale` / `fresh`；状态接口返回 |
| `snapshot_observed_at` | string/null | 快照观测时间；存在共享运行态快照时返回 |

### 11.8 ToolInvokeResult

同步工具调用返回：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `result` | object | 调用结果包装 |
| `result.transport_type` | string | 实际传输类型 |
| `result.payload` | object | MCP `CallToolResult` 转换后的结果 |
| `result.payload.is_error` | boolean | MCP 工具是否报告错误 |
| `result.payload.structured_content` | object | 结构化内容，存在时返回 |
| `result.payload.content` | array | MCP 内容数组，存在时返回 |
| `duration_ms` | integer | 调用耗时，毫秒 |

### 11.9 AsyncInvokeTask

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 任务 ID |
| `request_id` | string | 请求 ID |
| `tool_id` | string | 工具 ID |
| `service_id` | string | 服务 ID |
| `status` | string | `pending` / `running` / `succeeded` / `failed` / `cancelled` / `timed_out` |
| `cancel_requested` | boolean | 是否已请求取消 |
| `timeout_ms` | integer | 超时时间，毫秒 |
| `result` | object | 成功结果，结构同 `ToolInvokeResult`，存在时返回 |
| `error_message` | string | 错误信息，存在时返回 |
| `duration_ms` | integer | 执行耗时，毫秒 |
| `created_at` | string | 创建时间 |
| `started_at` | string/null | 开始时间 |
| `finished_at` | string/null | 结束时间 |
| `queue_length` | integer | 队列长度 |
| `queue_capacity` | integer | 队列容量 |
| `executor_in_flight` | integer | 执行器当前并发 |
| `executor_limit` | integer | 执行器并发上限 |
| `service_rate_limit` | integer | 服务级限流 |
| `user_rate_limit` | integer | 用户级限流 |
| `rate_limit_window_ms` | integer | 限流窗口，毫秒 |

### 11.10 AsyncTaskStats

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `pending` | integer | 等待中任务数 |
| `running` | integer | 执行中任务数 |
| `succeeded` | integer | 成功任务数 |
| `failed` | integer | 失败任务数 |
| `cancelled` | integer | 已取消任务数 |
| `timed_out` | integer | 超时任务数 |
| `queue_length` | integer | 队列长度 |
| `queue_capacity` | integer | 队列容量 |
| `executor_in_flight` | integer | 执行器当前并发 |
| `executor_limit` | integer | 执行器并发上限 |
| `service_rate_limit` | integer | 服务级限流 |
| `user_rate_limit` | integer | 用户级限流 |
| `rate_limit_window_ms` | integer | 限流窗口，毫秒 |

---

## 12. 接口响应明细

### 12.1 运维与文档入口响应

| 接口 | HTTP 状态码 | 响应结构 |
| --- | --- | --- |
| `GET /health` | `200` | `{ "status": "ok" }` |
| `GET /ready` | `200` 或 `503` | `{ "role": string, "ready": boolean, "checks": object, "reason": string }` |
| `GET /swagger/index.html` | `200` | HTML 页面 |

### 12.2 认证接口响应

#### `POST /api/v1/auth/login`

成功：HTTP `200`。

响应 `data`：

```json
{
  "access_token": "string",
  "refresh_token": "string",
  "expires_in": 3600,
  "user": {
    "id": "string",
    "username": "string",
    "email": "string",
    "role": "admin",
    "is_first_login": false
  }
}
```

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `400` | `1001` | 请求体缺失或字段校验失败 |
| `401` | `2001` | 用户名或密码错误、账号不可用等认证失败 |

#### `POST /api/v1/auth/refresh`

成功：HTTP `200`。

响应 `data`：

```json
{
  "access_token": "string",
  "refresh_token": "string",
  "expires_in": 3600
}
```

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `400` | `1001` | 请求体缺失或 `refresh_token` 为空 |
| `401` | `2001` / `2002` | refresh token 无效、过期或会话不可用 |

#### `POST /api/v1/auth/logout`

成功：HTTP `200`。

响应 `data`：

```json
{
  "ok": true
}
```

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `400` | `1001` | 请求体非空但不是合法 JSON |
| `401` | `2001` / `2002` | 未登录、Token 无效或过期 |

### 12.3 MCP 服务接口响应

#### `GET /api/v1/services`

成功：HTTP `200`。

响应 `data`：分页对象，`items` 为 `MCPService[]`。

```json
{
  "items": [
    {
      "id": "string",
      "name": "remote-mcp",
      "description": "string",
      "transport_type": "streamable_http",
      "command": "",
      "args": [],
      "env": {},
      "url": "https://example.com/mcp",
      "custom_headers": {},
      "session_mode": "auto",
      "compat_mode": "off",
      "listen_enabled": false,
      "timeout": 30,
      "status": "DISCONNECTED",
      "failure_count": 0,
      "last_error": "",
      "tags": [],
      "created_at": "2026-04-29T10:00:00Z",
      "updated_at": "2026-04-29T10:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 10,
  "total": 1
}
```

#### `POST /api/v1/services`

成功：HTTP `201`。

响应 `data`：`MCPService`。

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `400` | `1001` | 参数校验失败，例如缺少 `name` / `transport_type` / 条件必填字段 |
| `401` | `2001` / `2002` | 未登录或 Token 问题 |
| `403` | `2003` | 当前角色无修改权限 |
| `409` | `1003` | 服务名称已存在 |

#### `GET /api/v1/services/{id}`

成功：HTTP `200`。

响应 `data`：`MCPService`。

常见错误：`400` 参数错误、`401` 未认证、`404` 服务不存在。

#### `PUT /api/v1/services/{id}`

成功：HTTP `200`。

响应 `data`：`MCPService`。

常见错误：`400` 参数错误、`401` 未认证、`403` 权限不足、`404` 服务不存在、`409` 名称冲突。

#### `DELETE /api/v1/services/{id}`

成功：HTTP `200`。

响应 `data`：

```json
{
  "ok": true
}
```

常见错误：`400` 参数错误、`401` 未认证、`403` 权限不足、`404` 服务不存在。

#### `POST /api/v1/services/{id}/connect`

成功：HTTP `200`。

响应 `data`：`RuntimeStatus`。

```json
{
  "service_id": "string",
  "status": "CONNECTED",
  "transport_type": "streamable_http",
  "session_id_exists": true,
  "protocol_version": "string",
  "listen_enabled": false,
  "listen_active": false,
  "in_flight": 0,
  "failure_count": 0
}
```

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `401` | `2001` / `2002` | 未认证或 Token 问题 |
| `403` | `2003` | 当前角色无修改权限 |
| `404` | `1002` | 服务不存在 |
| `502` | `3001` | 服务连接失败 |

#### `POST /api/v1/services/{id}/disconnect`

成功：HTTP `200`。

响应 `data`：

```json
{
  "ok": true
}
```

常见错误：`401` 未认证、`403` 权限不足、`404` 服务不存在。

#### `GET /api/v1/services/{id}/status`

成功：HTTP `200`。

响应 `data`：服务状态聚合对象。字段见 [11.7 RuntimeStatus](#117-runtimestatus)，并固定包含：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 服务 ID |
| `name` | string | 服务名称 |
| `status` | string | 当前综合状态 |
| `persisted_status` | string | 数据库持久化状态 |
| `runtime_status` | string/null | 运行时状态；没有运行时状态时为 `null` |
| `status_source` | string | 状态来源：`persisted` / `runtime` / `snapshot` |
| `snapshot_freshness` | string | 快照新鲜度：`missing` / `stale` / `fresh` |
| `snapshot_observed_at` | string/null | 快照观测时间；仅存在共享运行态快照时返回 |

示例：

```json
{
  "id": "string",
  "name": "remote-mcp",
  "status": "CONNECTED",
  "persisted_status": "CONNECTED",
  "runtime_status": "CONNECTED",
  "status_source": "runtime",
  "snapshot_freshness": "missing",
  "last_error": "",
  "failure_count": 0,
  "transport_type": "streamable_http",
  "session_id_exists": true,
  "protocol_version": "2024-11-05",
  "listen_enabled": false,
  "listen_active": false,
  "in_flight": 0
}
```

常见错误：`401` 未认证、`404` 服务不存在。

### 12.4 工具与任务接口响应

#### `GET /api/v1/services/{id}/tools`

成功：HTTP `200`。

响应 `data`：`Tool[]`。

```json
[
  {
    "id": "string",
    "mcp_service_id": "string",
    "name": "search",
    "description": "string",
    "input_schema": {},
    "is_enabled": true,
    "synced_at": "2026-04-29T10:00:00Z",
    "created_at": "2026-04-29T10:00:00Z",
    "updated_at": "2026-04-29T10:00:00Z"
  }
]
```

常见错误：`401` 未认证、`404` 服务不存在。

#### `POST /api/v1/services/{id}/sync-tools`

成功：HTTP `200`。

响应 `data`：同步后的 `Tool[]`。

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `401` | `2001` / `2002` | 未认证或 Token 问题 |
| `403` | `2003` | 当前角色无修改权限 |
| `404` | `1002` | 服务不存在 |
| `409` | `1003` | 服务处于错误状态，需要先恢复连接 |
| `502` | `3001` / `3002` | 远端服务或工具同步失败 |

#### `GET /api/v1/tools/{id}`

成功：HTTP `200`。

响应 `data`：`Tool`。

常见错误：`401` 未认证、`404` 工具不存在。

#### `POST /api/v1/tools/{id}/invoke`

成功：HTTP `200`。

响应 `data`：`ToolInvokeResult`。

```json
{
  "result": {
    "transport_type": "streamable_http",
    "payload": {
      "is_error": false,
      "structured_content": {},
      "content": []
    }
  },
  "duration_ms": 123
}
```

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `400` | `1001` | 请求体缺失、`arguments` 缺失或 JSON 非法 |
| `401` | `2001` / `2002` | 未认证或 Token 问题 |
| `403` | `2003` | 当前角色无修改权限 |
| `404` | `1002` | 工具或服务不存在 |
| `409` | `1003` | 服务处于错误状态 |
| `429` | `3003` | 并发或限流命中 |
| `502` | `3002` | 工具调用失败 |

#### `POST /api/v1/tools/{id}/invoke-async`

成功：HTTP `202`。

响应 `data`：`AsyncInvokeTask`。

```json
{
  "id": "string",
  "request_id": "string",
  "tool_id": "string",
  "service_id": "string",
  "status": "pending",
  "cancel_requested": false,
  "timeout_ms": 30000,
  "duration_ms": 0,
  "created_at": "2026-04-29T10:00:00Z",
  "queue_length": 1,
  "queue_capacity": 100,
  "executor_in_flight": 0,
  "executor_limit": 10,
  "service_rate_limit": 0,
  "user_rate_limit": 0,
  "rate_limit_window_ms": 60000
}
```

常见错误：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| `400` | `1001` | 请求参数错误 |
| `401` | `2001` / `2002` | 未认证或 Token 问题 |
| `403` | `2003` | 当前角色无修改权限 |
| `404` | `1002` | 工具或服务不存在 |
| `409` | `1003` | 异步调用未启用或服务处于错误状态 |
| `429` | `3003` | 异步队列满、并发或限流命中 |

#### `GET /api/v1/tasks/{id}`

成功：HTTP `200`。

响应 `data`：`AsyncInvokeTask`。

当任务成功完成时，`data.result` 存在，结构同 `ToolInvokeResult`；当任务失败时，`data.error_message` 存在。

常见错误：`401` 未认证、`403` 无权查看该任务、`404` 异步任务未启用或任务不存在。

#### `POST /api/v1/tasks/{id}/cancel`

成功：HTTP `202`。

响应 `data`：`AsyncInvokeTask`。

常见错误：`401` 未认证、`403` 无权取消该任务、`404` 异步任务未启用或任务不存在。

#### `GET /api/v1/tasks/stats`

成功：HTTP `200`。

响应 `data`：`AsyncTaskStats`。

```json
{
  "pending": 0,
  "running": 0,
  "succeeded": 0,
  "failed": 0,
  "cancelled": 0,
  "timed_out": 0,
  "queue_length": 0,
  "queue_capacity": 100,
  "executor_in_flight": 0,
  "executor_limit": 10,
  "service_rate_limit": 0,
  "user_rate_limit": 0,
  "rate_limit_window_ms": 60000
}
```

常见错误：`401` 未认证、`403` 非管理员、`404` 异步任务未启用。

### 12.5 调用历史接口响应

#### `GET /api/v1/history`

成功：HTTP `200`。

响应 `data`：分页对象，`items` 为 `RequestHistory[]`。

```json
{
  "items": [
    {
      "id": "string",
      "mcp_service_id": "string",
      "tool_name": "search",
      "user_id": "string",
      "request_body": {},
      "response_body": {},
      "request_truncated": false,
      "response_truncated": false,
      "request_hash": "string",
      "response_hash": "string",
      "request_size": 123,
      "response_size": 456,
      "compression_type": "none",
      "status": "success",
      "error_message": "",
      "duration_ms": 100,
      "created_at": "2026-04-29T10:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 10,
  "total": 1
}
```

常见错误：`400` query 参数错误、`401` 未认证。

#### `GET /api/v1/history/{id}`

成功：HTTP `200`。

响应 `data`：`RequestHistory`。

常见错误：`400` 参数错误、`401` 未认证、`403` 无权查看、`404` 历史不存在。

### 12.6 用户接口响应

#### `GET /api/v1/users`

成功：HTTP `200`。

响应 `data`：分页对象，`items` 为 `User[]`。

```json
{
  "items": [
    {
      "id": "string",
      "username": "admin",
      "email": "admin@example.com",
      "role": "admin",
      "is_active": true,
      "is_first_login": false,
      "last_login_at": "2026-04-29T10:00:00Z",
      "created_at": "2026-04-29T10:00:00Z",
      "updated_at": "2026-04-29T10:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 10,
  "total": 1
}
```

常见错误：`400` query 参数错误、`401` 未认证、`403` 非管理员。

#### `POST /api/v1/users`

成功：HTTP `201`。

响应 `data`：`User`。

常见错误：`400` 参数错误、`401` 未认证、`403` 非管理员、`409` 用户名或邮箱冲突。

#### `PUT /api/v1/users/{id}`

成功：HTTP `200`。

响应 `data`：`User`。

常见错误：`400` 参数错误或没有提供更新字段、`401` 未认证、`403` 非管理员、`404` 用户不存在、`409` 邮箱冲突。

#### `DELETE /api/v1/users/{id}`

成功：HTTP `200`。

响应 `data`：

```json
{
  "ok": true
}
```

常见错误：`400` 参数错误、`401` 未认证、`403` 非管理员、`404` 用户不存在。

#### `PUT /api/v1/users/{id}/password`

成功：HTTP `200`。

响应 `data`：

```json
{
  "ok": true
}
```

常见错误：`400` 参数错误或旧密码不正确、`401` 未认证、`403` 不能修改其他用户密码、`404` 用户不存在。

### 12.7 审计日志接口响应

#### `GET /api/v1/audit-logs`

成功：HTTP `200`。

响应 `data`：分页对象，`items` 为 `AuditLog[]`。

```json
{
  "items": [
    {
      "id": "string",
      "user_id": "string",
      "username": "admin",
      "action": "create_service",
      "resource_type": "mcp_service",
      "resource_id": "string",
      "detail": {},
      "ip_address": "127.0.0.1",
      "user_agent": "Mozilla/5.0",
      "created_at": "2026-04-29T10:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 10,
  "total": 1
}
```

常见错误：`400` query 参数错误、`401` 未认证、`403` 非管理员。

#### `GET /api/v1/audit-logs/export`

成功：HTTP `200`。

响应类型：`text/csv; charset=utf-8`。

响应头：

```http
Content-Disposition: attachment; filename=audit_logs.csv
```

响应体：CSV 文本，不使用统一 JSON 响应结构。

常见错误：`400` query 参数错误、`401` 未认证、`403` 非管理员。

---

## 13. 前端 TypeScript 类型参考

以下类型可作为前端 API Client 的初始定义，后续可按页面需要拆分。

```ts
export interface ApiResponse<T = unknown> {
  code: number;
  message: string;
  data?: T;
  timestamp: number;
}

export interface PageData<T> {
  items: T[];
  page: number;
  page_size: number;
  total: number;
}

export type Role = 'admin' | 'operator' | 'readonly';
export type TransportType = 'stdio' | 'streamable_http' | 'sse';
export type ServiceStatus = 'DISCONNECTED' | 'CONNECTING' | 'CONNECTED' | 'ERROR';
export type RequestStatus = 'success' | 'failed';
export type AsyncTaskStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'timed_out';

export interface User {
  id: string;
  username: string;
  email: string;
  role: Role;
  is_active: boolean;
  is_first_login: boolean;
  last_login_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface MCPService {
  id: string;
  name: string;
  description: string;
  transport_type: TransportType;
  command: string;
  args: string[];
  env: Record<string, string>;
  url: string;
  bearer_token?: string;
  custom_headers: Record<string, string>;
  session_mode: 'auto' | 'required' | 'disabled';
  compat_mode: 'off' | 'allow_legacy_sse';
  listen_enabled: boolean;
  timeout: number;
  status: ServiceStatus;
  failure_count: number;
  last_error: string;
  tags: string[];
  created_at: string;
  updated_at: string;
}

export interface Tool {
  id: string;
  mcp_service_id: string;
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
  is_enabled: boolean;
  synced_at: string;
  created_at: string;
  updated_at: string;
}

export interface RuntimeStatus {
  service_id?: string;
  id?: string;
  name?: string;
  status: ServiceStatus;
  persisted_status?: ServiceStatus;
  runtime_status?: ServiceStatus;
  transport_type: string;
  session_id_exists?: boolean;
  protocol_version?: string;
  listen_enabled?: boolean;
  listen_active?: boolean;
  listen_last_error?: string;
  status_source?: 'persisted' | 'runtime' | 'snapshot';
  snapshot_freshness?: 'missing' | 'stale' | 'fresh';
  snapshot_observed_at?: string | null;
  last_seen_at?: string | null;
  last_used_at?: string | null;
  in_flight?: number;
  transport_capabilities?: Record<string, unknown>;
  last_error?: string;
  failure_count?: number;
}

export interface ToolInvokeResult {
  result: {
    transport_type: string;
    payload: {
      is_error: boolean;
      structured_content?: Record<string, unknown>;
      content?: unknown[];
    } | null;
  };
  duration_ms: number;
}

export interface AsyncInvokeTask {
  id: string;
  request_id: string;
  tool_id: string;
  service_id: string;
  status: AsyncTaskStatus;
  cancel_requested: boolean;
  timeout_ms: number;
  result?: ToolInvokeResult;
  error_message?: string;
  duration_ms: number;
  created_at: string;
  started_at?: string | null;
  finished_at?: string | null;
  queue_length: number;
  queue_capacity: number;
  executor_in_flight: number;
  executor_limit: number;
  service_rate_limit: number;
  user_rate_limit: number;
  rate_limit_window_ms: number;
}

export interface AsyncTaskStats {
  pending: number;
  running: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  timed_out: number;
  queue_length: number;
  queue_capacity: number;
  executor_in_flight: number;
  executor_limit: number;
  service_rate_limit: number;
  user_rate_limit: number;
  rate_limit_window_ms: number;
}

export interface RequestHistory {
  id: string;
  mcp_service_id: string;
  tool_name: string;
  user_id: string;
  request_body: Record<string, unknown>;
  response_body: Record<string, unknown>;
  request_truncated: boolean;
  response_truncated: boolean;
  request_hash: string;
  response_hash: string;
  request_size: number;
  response_size: number;
  compression_type: string;
  status: RequestStatus;
  error_message: string;
  duration_ms: number;
  created_at: string;
}

export interface AuditLog {
  id: string;
  user_id: string;
  username: string;
  action: string;
  resource_type: string;
  resource_id: string;
  detail: Record<string, unknown>;
  ip_address: string;
  user_agent: string;
  created_at: string;
}
```
