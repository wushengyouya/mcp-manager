# mcp-manager 架构升级建议 v2.1

## 1. 摘要

当前 `mcp-manager` **适合做渐进式演进，不适合直接一步到位升级为完整的 control-plane / executor 分布式架构**。

更适合当前仓库和团队规模的路线是：

1. **先把单体改造成可分离单体**
2. **再替换关键基础设施**（SQLite -> PostgreSQL，进程内状态 -> Redis）
3. **最后再拆成双角色部署**（control-plane / executor）

本文档保留“单代码库、双部署角色、控制面与执行面分离”的方向，但把实施节奏调整为更贴合当前项目现状的版本。

配套文档：

- `docs/architecture-upgrade-roadmap.md`：实施路线图与按周计划
- `docs/architecture-upgrade-phase0-phase1-checklist.md`：第一阶段代码改造清单
- `docs/architecture-upgrade-phase0-phase1-issues.md`：可直接开发的 issue/task 列表

### 1.1 当前判断

- 当前主要瓶颈是 **SQLite、进程内运行态、进程内 JWT 黑名单、历史/审计同步写放大**，不是 Gin 本身。
- 当前代码已经具备一定的分层基础（`handler/service/repository`），但**服务层仍直接依赖本地 `mcpclient.Manager`**，尚未形成“本地执行 / 远程执行”边界。
- 因此，**最优第一步不是立刻拆双角色，而是先抽象边界、去状态化、做观测**。

### 1.2 容量目标的使用方式

以下容量只能作为**中期目标区间**，不能视为当前实测结论：

- 管理/API 流量：`500-2000 QPS`
- 工具调用并发：`100-300`

这些数字应在 **Phase 0 压测与观测基线**完成后，再转换为正式验收目标。

---

## 2. 当前系统现状

结合当前仓库实现，系统本质上仍是“单实例状态型单体”：

- HTTP 服务、路由、业务、数据库初始化都在单进程完成
- 默认数据库是 SQLite
- MCP 长连接全部由进程内 `Manager` 持有
- JWT 黑名单保存在进程内内存
- 健康检查在单进程内定时执行
- 历史与审计写入仍以同步 DB 写为主

### 2.1 当前代码中的关键事实

#### 单体初始化与依赖拼装

`cmd/server/main.go` 里直接完成以下动作：

- 初始化数据库
- 初始化 JWT 黑名单
- 初始化 `mcpclient.Manager`
- 初始化健康检查器
- 初始化 HTTP Router

这说明当前系统仍是**强耦合单进程启动模型**。

#### 数据库仍只支持 SQLite

当前配置与数据库实现只接受 SQLite：

- `internal/config/config.go`
- `internal/database/database.go`

并且默认配置是：

- `database.driver = sqlite`
- `max_open_conns = 1`
- `max_idle_conns = 1`

这意味着写入放大、分页查询、健康检查回写都会较早遇到瓶颈。

#### MCP 运行态完全在进程内

`internal/mcpclient/manager.go` 使用进程内 map 持有：

- 已连接服务
- 运行态快照
- 工具调用目标连接

这意味着：

- 无法横向扩容
- 多副本之间无法共享 owner
- 状态查询无法天然一致

#### JWT 黑名单是进程内内存

`pkg/crypto/blacklist.go` 是进程内实现。

这意味着只要出现多副本部署：

- 登出立即失效无法跨副本一致
- refresh 后旧 token 作废无法全局一致

#### 健康检查与状态写回耦合较深

健康检查目前由 `internal/mcpclient/health_checker.go` 定时轮询，并直接推动状态更新。

这意味着：

- 健康检查职责与运行态存储耦合
- 将来迁移到 executor 时，需要一起拆分

#### 历史 / 审计仍以同步写入为主

- `internal/service/tool_invoke_service.go`
- `internal/service/audit_sink.go`

虽然已经有 `sink` 语义，但默认实现仍是同步写 PostgreSQL/SQLite 风格的数据库仓储。

### 2.2 当前阶段的保守判断

在没有正式压测数据之前，可以做如下保守判断：

- 当前瓶颈会优先出现在 SQLite 写入与单进程状态管理
- 不是所有 transport 都必须立即分布式 sticky 化
- 当前最值得优先处理的是**状态外置与执行边界抽象**，而不是“先拆服务”

---

## 3. 设计原则

### 3.1 总原则

- **先抽象边界，再替换基础设施，再拆部署角色**
- **优先保留外部 REST API 兼容性**
- **优先保留现有代码分层，不做无必要的大重写**
- **同步调用优先 RPC，不先引入 MQ request-reply**
- **只对真正 sticky 的 transport 做 owner/lease 管理**

### 3.2 三类“真相”的重新定义

为避免后续实现混乱，建议明确三类状态来源：

#### 持久化真相

由 PostgreSQL 承担：

- 用户
- 服务配置
- 工具元数据
- 历史记录
- 审计日志

#### 活连接一手真相

由 **executor 本地内存 / 本地连接对象** 承担：

- 某个连接是否真实存活
- 会话是否仍有效
- 当前 transport 的原生运行态

#### 跨副本协调状态

由 Redis 承担：

- owner 租约
- epoch / fencing token
- 运行态缓存
- JWT 黑名单
- 限流与并发计数

> 不建议表述为“Redis 是运行态真相”。更准确的表述是：
> **Redis 是跨副本协调状态与运行态缓存。**

---

## 4. 目标架构（修订版）

### 4.1 最终方向

最终仍然建议演进为：

- 单代码库
- 双部署角色
- control-plane / executor 分离
- PostgreSQL + Redis
- 内部同步 RPC（优先 gRPC）

但这个目标应在完成前置抽象后推进。

### 4.2 演进中的过渡形态

在正式双角色部署之前，建议先支持三种运行模式：

- `app.role = all`
- `app.role = control-plane`
- `app.role = executor`

其中：

- `all` 用于本地开发、测试、单机部署、回退
- `control-plane` 用于对外 API 与配置查询
- `executor` 用于 MCP 长连接、工具调用、健康检查

这样可以避免一旦拆分就无法本地调试的问题。

### 4.3 修订后的架构图

```mermaid
flowchart TB
    U[用户 / 前端 / API 调用方]
    LB[LB / Ingress]

    subgraph CP[Control Plane]
        CP1[control-plane-1]
        CP2[control-plane-2]
    end

    subgraph EX[Executors]
        EX1[executor-1]
        EX2[executor-2]
    end

    subgraph INFRA[基础设施]
        PG[(PostgreSQL)]
        R[(Redis)]
        RPC[gRPC / Internal RPC]
    end

    subgraph MCP[MCP Services]
        S1[stdio]
        S2[sse]
        S3[streamable_http(session)]
        S4[streamable_http(stateless)]
    end

    U --> LB
    LB --> CP1
    LB --> CP2

    CP1 --> PG
    CP2 --> PG

    CP1 --> R
    CP2 --> R

    CP1 --> RPC
    CP2 --> RPC

    RPC --> EX1
    RPC --> EX2

    EX1 --> R
    EX2 --> R

    EX1 --> PG
    EX2 --> PG

    EX1 --> S1
    EX1 --> S2
    EX1 --> S3
    EX2 --> S4
```

---

## 5. 在拆角色之前必须先补的抽象层

这是当前文档最需要补充的部分。

### 5.1 ExecutorGateway

定义服务层访问“执行能力”的统一抽象。

目标：把现在对 `*mcpclient.Manager` 的直接依赖，改成可以切换的执行网关。

建议接口能力：

- `ConnectService`
- `DisconnectService`
- `SyncTools`
- `InvokeTool`
- `GetRuntimeStatus`

实现可以分两类：

- `LocalExecutorGateway`
- `RemoteExecutorGateway`

这样当前单体仍然可跑，但后续可平滑切到 RPC。

### 5.2 RuntimeStore

统一运行态的读写来源。

建议实现分层：

- `InMemoryRuntimeStore`
- `RedisRuntimeStore`

control-plane 查询状态时，不应再直接假设运行态来自本地 `Manager`。

### 5.3 TokenBlacklistStore

统一 JWT 黑名单能力。

建议实现：

- `InMemoryTokenBlacklistStore`
- `RedisTokenBlacklistStore`

这样在单机和多副本之间可以平滑切换。

### 5.4 HistorySink / AuditSink

当前项目已有 `AuditSink` 语义，但还不够完整。

建议形成统一模式：

- `SyncDBHistorySink`
- `AsyncHistorySink`
- `SyncDBAuditSink`
- `AsyncAuditSink`

注意：

- 异步 sink 不应作为第一步强制项
- 应支持开关与降级
- 队列满时可以回退同步写，避免数据完全丢失

### 5.5 LeaseStore

当进入双角色部署时，需要统一 owner 语义：

- `AcquireOwner`
- `RenewOwner`
- `ReleaseOwner`
- `GetOwner`
- `BumpEpoch`

这部分不建议在单体阶段提前做成复杂分布式逻辑，但接口应提前存在。

---

## 6. transport 调度模型

这部分原文方向是对的，建议保留，但要明确它是 **Phase 2 之后** 的能力。

### 6.1 必须 sticky 的场景

以下服务建议 sticky 到某个 executor：

- `stdio`
- `sse`
- `streamable_http` 且 `session_mode=required`
- `streamable_http` 且 `listen_enabled=true`

### 6.2 可无 owner 的场景

以下服务可按请求即时调度：

- `streamable_http` 且无 session 依赖
- `session_mode=disabled`
- `listen_enabled=false`

### 6.3 调度规则

- sticky 服务：优先 owner 路由
- 无 owner 时：尝试抢占 owner
- 非 sticky 服务：从健康 executor 中选择执行，不建立长期 owner

这个策略优于“所有 transport 全量 sticky”。

---

## 7. 分阶段实施（修订版）

## 7.1 Phase 0：观测与边界抽象

这是**必须先做**的阶段。

### 实现内容

#### 观测基础

- 暴露 Prometheus 指标
  - HTTP QPS / 状态码 / 路由耗时
  - MCP 调用耗时
  - SQLite / PostgreSQL 连接池状态
  - 健康检查次数 / 失败率
  - 每 service 调用次数 / 失败率 / 在途数
- 开启 `pprof`
- 建立压测基线
  - 纯查询接口
  - 服务 CRUD
  - 工具调用
  - 多 transport 混合场景

#### 抽象边界

先引入接口，不改变部署形态：

- `ExecutorGateway`
- `RuntimeStore`
- `TokenBlacklistStore`
- `HistorySink`
- `AuditSink`

#### 配置预埋

预留但不强制启用：

- `app.role`
- `redis.*`
- `grpc.*`
- `metrics.*`

### 验收

- 单体运行逻辑不变
- 关键路径已有指标
- 服务层不再直接硬编码依赖“只能本地执行”
- 能拿到当前系统的 p50 / p95 / p99 基线

---

## 7.2 Phase 1：单体内的基础设施升级

此阶段仍保持 **单体部署**，不强制拆 control-plane / executor。

### 实现内容

#### 数据库升级

- SQLite -> PostgreSQL
- 尽量保留当前实体模型与 GORM 分层
- 修正唯一索引冲突识别逻辑，不再依赖 SQLite 错误字符串

#### JWT 黑名单升级

- 由进程内黑名单切换到 Redis
- 保留本地短 TTL 只读缓存作为性能优化
- Redis 不可用时要有明确降级策略

#### 运行态缓存升级

- 状态查询优先查 `RuntimeStore`
- 单体模式下可先用“本地 + Redis 缓存写入”模式过渡
- 但此时仍由本地执行器掌握一手连接真相

#### 导出与写入治理

- 审计导出改为真正流式/分页游标式输出
- 历史 / 审计 sink 默认仍可先同步写
- 异步 sink 改为**可选开关**，而非此阶段强依赖

### 验收

- 单体模式下 PostgreSQL 运行稳定
- Redis 黑名单可以支撑多副本 token 失效一致性
- 状态查询已不依赖“只能看本进程内存”
- 历史/审计查询性能优于 SQLite 阶段

---

## 7.3 Phase 2：单代码库双角色

完成前两阶段后，再进入本阶段。

### 实现内容

- 支持：
  - `app.role = all`
  - `app.role = control-plane`
  - `app.role = executor`
- 增加内部 RPC（优先 gRPC）
- `ExecutorGateway` 切换为：
  - control-plane 走 `RemoteExecutorGateway`
  - executor 走 `LocalExecutorGateway`
- 健康检查仅在 executor 生效
- sticky 服务引入 owner lease + epoch
- 非 sticky 服务采用无 owner 即时调度

### 验收

- `2 control-plane + 2 executor + PostgreSQL + Redis` 可以稳定运行
- 多副本下登录、登出、refresh 一致
- sticky 服务可接管
- 非 sticky 服务可被任意健康 executor 调度
- `all` 模式仍可用于本地开发和回退

---

## 7.4 Phase 3：执行面增强

### 实现内容

- executor 分级限流
  - 每 executor 最大在途调用
  - 每 service 最大在途调用
  - 每用户调用速率限制
- 历史 / 审计 / 告警逐步异步 sink 化
- 可选新增异步调用接口
  - `POST /api/v1/tools/:id/invoke-async`
  - `GET /api/v1/tasks/:id`
- 长耗时调用支持取消、超时透传、任务查询

### 验收

- 在目标并发下系统退化可控
- 队列积压不拖垮同步链路
- 长耗时任务可被查询与取消

---

## 7.5 Phase 4：按热点拆分（触发式）

只有在以下条件满足时再考虑：

- 历史/审计查询明显压垮主库
- 某类 transport 的 executor 成为独立容量热点
- 团队规模、发布频率、运维能力已经支撑独立服务治理

### 优先拆分方向

- 历史 / 审计读链路独立读副本或归档
- executor 按 transport 拆池
  - `executor-stdio`
  - `executor-remote`
- 不默认拆 `auth/user` 为独立微服务

---

## 8. Redis 设计（修订定位）

Redis 的职责定义为：

- JWT 黑名单
- owner lease
- epoch / fencing token
- 运行态缓存
- 并发计数 / 限流计数
- executor 心跳与负载信息

### 8.1 建议 Key

- `mcp:auth:blacklist:{token_sha256}`
- `mcp:service:owner:{service_id}`
- `mcp:service:epoch:{service_id}`
- `mcp:service:runtime:{service_id}`
- `mcp:service:sem:{service_id}`
- `mcp:executor:load:{executor_id}`

### 8.2 租约规则

- sticky 服务通过 `SET NX EX` 抢占 owner
- executor 周期续租
- 抢占成功后递增 `epoch`
- 运行态写入带 `epoch`
- 非当前 epoch 的旧 owner 不允许更新状态或执行敏感操作

---

## 9. 内部 RPC 设计建议

建议在 Phase 2 引入内部 `ExecutionService`。

### 9.1 建议接口

- `ConnectService`
- `DisconnectService`
- `SyncTools`
- `InvokeTool`
- `GetRuntimeStatus`
- `PingExecutor`

### 9.2 关键字段

#### ConnectServiceRequest

- `service_id`
- `expected_epoch`
- `service_snapshot`
- `actor`

#### InvokeToolRequest

- `service_id`
- `tool_id`
- `tool_name`
- `arguments`
- `expected_epoch`
- `request_id`
- `timeout_ms`
- `actor`

### 9.3 关键约束

- sticky 服务上的 `Connect / Disconnect / Invoke / SyncTools` 必须校验 `expected_epoch`
- epoch 不匹配应直接拒绝
- `request_id` 用于日志关联与排障
- 不要求在第一版就做业务级强幂等

---

## 10. 数据库与索引建议

### 10.1 PostgreSQL 替换原则

- 保持现有实体模型尽量稳定
- 保持 `repository` 分层不变
- 尽量减少 service 层业务逻辑重写

### 10.2 必加索引

#### `request_histories`

- `(mcp_service_id, created_at desc)`
- `(user_id, created_at desc)`
- `(status, created_at desc)`
- `(tool_name, created_at desc)`

#### `audit_logs`

- `(created_at desc)`
- `(user_id, created_at desc)`
- `(action, created_at desc)`
- `(resource_type, created_at desc)`

#### `mcp_services`

- 保留有效记录唯一索引

#### `tools`

- 保留 `(mcp_service_id, name)` 活跃唯一索引

### 10.3 代码层额外注意点

当前仓储里部分唯一冲突判断依赖 SQLite 风格错误字符串。迁移 PostgreSQL 时需要统一封装，否则会出现行为不一致。

---

## 11. 配置项建议

建议最终支持以下配置：

- `app.role = all | control-plane | executor`
- `database.driver = sqlite | postgres`
- `database.dsn`
- `redis.addr`
- `redis.password`
- `redis.db`
- `grpc.listen_addr`
- `grpc.dial_timeout`
- `runtime.owner_ttl`
- `runtime.renew_interval`
- `invoke.max_concurrency_per_executor`
- `invoke.max_concurrency_per_service`
- `invoke.default_timeout`
- `metrics.enabled`
- `metrics.listen_addr`

但要注意：

- `sqlite` 不应在迁移完成前被立刻删除
- `all` 模式应长期保留

---

## 12. 对现有代码的影响边界

建议优先保留现有 `handler/service/repository` 分层，只在基础设施和执行边界做演进。

### 12.1 handler

- 对外 REST API 尽量保持兼容
- 新增接口优先走增量，不要破坏现有调用路径

### 12.2 service

优先把以下服务改造成“面向抽象编程”：

- `MCPService`
- `ToolService`
- `ToolInvokeService`

核心目标：

- 不再直接依赖只能本地执行的 `Manager`
- 能在本地执行与远程执行之间切换

### 12.3 mcpclient

- 保留 executor 内的核心实现
- `Manager` 最终只存在于 `executor` 或 `all` 模式内
- control-plane 不直接持有 MCP 长连接

### 12.4 repository

- 保留现有 GORM 仓储模式
- 升级数据库驱动与索引策略
- 补齐 PostgreSQL 兼容错误归一化

### 12.5 config

- 增加 role、redis、grpc、metrics、limit 配置
- 保留单体默认值，避免升级后本地无法启动

---

## 13. 测试与验收

### 13.1 功能回归

- 登录、刷新、登出
- 用户管理
- 服务 CRUD
- connect / disconnect / status
- sync-tools
- invoke
- history list / get
- audit list / export

### 13.2 一致性与分布式行为

在进入双角色后，重点验证：

- token 在多副本下立即失效
- sticky 服务不会出现双 owner 同时执行
- executor 宕机后租约过期并可接管
- epoch 不匹配时旧 owner 不得更新状态或执行调用

### 13.3 性能与故障

- PostgreSQL 主库高负载
- Redis 短暂抖动
- executor 宕机 / 重启
- 远端 MCP 服务超时、断流、session 异常
- `stdio` 子进程退出、卡死、慢响应

### 13.4 transport 专项

- `stdio`
- `sse`
- `streamable_http + session_mode=required`
- `streamable_http + session_mode=disabled`
- `listen_enabled = true / false`

---

## 14. 明确不建议现在就做的事

以下动作不建议作为当前阶段默认动作：

- 直接全面微服务化
- 一开始就引入 MQ 承担同步 request-reply
- 在没有抽象层前就把 service 逻辑全部改成 RPC
- 把 Redis 定义为唯一运行态真相
- 过早拆 `auth/user` 独立服务

---

## 15. 最终结论

**这份升级方案的方向是对的，但应改成“渐进式升级方案”，而不是“一步到位实施方案”。**

对于当前 `mcp-manager`，更合适的执行顺序是：

1. **观测与边界抽象**
2. **单体内基础设施升级**
3. **双角色部署**
4. **执行面增强**
5. **按热点拆分**

用一句话总结：

> 先把系统做成“可分离单体”，再把它拆成“控制面 + 执行面”。

