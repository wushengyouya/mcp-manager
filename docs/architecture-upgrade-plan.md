# mcp-manager 架构升级建议 v3.1

## 1. 摘要

当前 `mcp-manager` **适合渐进式演进，不适合直接一步到位升级为完整的 control-plane / executor 分布式架构**。

结合当前仓库实现，建议采用以下升级路径：

1. **先把当前单体改造成“可分离单体”**
2. **再升级关键基础设施**（SQLite -> PostgreSQL，进程内状态/黑名单 -> Redis）
3. **最后再拆成双部署角色**（control-plane / executor）

本文档保留以下目标方向：

- 单代码库
- 双部署角色
- 控制面与执行面分离
- PostgreSQL + Redis
- 内部 RPC

但会把实施顺序、边界抽象和验收方式调整为更贴合当前项目现状的版本。

---

## 2. 当前系统现状

结合当前代码，系统本质上仍然是一个**带分层的状态型单体**：

- HTTP 服务、依赖装配、数据库初始化都在单进程完成
- 默认数据库为 SQLite
- MCP 长连接全部由进程内 `Manager` 持有
- JWT 黑名单保存在进程内内存
- 健康检查在单进程内定时执行
- 历史与审计默认以同步数据库写入为主

### 2.1 当前代码中的关键事实

#### 单进程装配模型仍然很强

`cmd/server/main.go` 中的 `buildApp` 直接完成以下装配：

- 初始化数据库
- 执行数据库迁移
- 初始化 JWT 黑名单与 JWT 服务
- 初始化 `mcpclient.Manager`
- 初始化健康检查器
- 初始化审计清理任务
- 初始化管理员账号
- 初始化 HTTP Router

这说明当前系统仍然是**强耦合单进程启动模型**，还没有为 `all / control-plane / executor` 三种角色模式做装配边界。

#### 数据库当前只支持 SQLite

当前数据库配置和实现只接受 SQLite：

- `internal/config/config.go`
- `internal/database/database.go`

并且默认配置为：

- `database.driver = sqlite`
- `max_open_conns = 1`
- `max_idle_conns = 1`

这意味着数据库写入放大、分页扫描、导出、健康状态回写会较早成为瓶颈。

#### MCP 运行态完全在进程内

`internal/mcpclient/manager.go` 使用进程内 map 持有：

- 已连接服务
- 会话状态
- 工具调用目标连接
- 最近运行态快照

这意味着：

- 当前无法横向扩容
- 多副本无法共享 owner
- control-plane 无法天然读取一致的运行态

#### JWT 黑名单是进程内实现

`pkg/crypto/blacklist.go` 为进程内黑名单实现，`pkg/crypto/jwt.go` 中 `JWTService` 直接依赖该实现。

这意味着在多副本部署下：

- 登出后的 token 无法全局立即失效
- refresh 后旧 token 无法跨副本一致失效

#### 健康检查与运行态更新耦合较深

`internal/mcpclient/health_checker.go` 定时轮询已连接服务，并直接驱动状态更新。

这意味着：

- 健康检查逻辑当前只能附着于本地 `Manager`
- control-plane / executor 拆分后需要迁移为 executor 本地职责

#### 历史与审计链路还不对称

- `Audit` 已有 sink 抽象：`internal/service/audit_sink.go`
- `History` 目前仍在 `internal/service/tool_invoke_service.go` 中直接写 `RequestHistoryRepository`

这意味着：

- 审计链路可以渐进演进为同步/异步 sink
- 历史链路在引入 `HistorySink` 前，仍需要先从直接 repo 写入中抽离

#### 当前已经存在“持久化状态与运行态可能不一致”的风险

进程重启后：

- `Manager` 内存会丢失
- 数据库中的服务状态仍可能保留为 `CONNECTED` 或 `ERROR`

因此数据库中的 `service.status` 当前更适合作为**持久化视图**，而不是连接真相。

---

## 3. 设计原则

### 3.1 总原则

- **先拆启动编排，再拆运行角色**
- **先抽象执行边界，再替换基础设施**
- **先保留外部 REST API 兼容性**
- **优先保留现有 handler / service / repository 分层**
- **优先小接口抽象，不先引入大而全网关**
- **先固化调用边界，再定义内部 RPC wire contract**
- **只对真正 sticky 的 transport 做 owner/lease 管理**
- **Redis 只承担跨副本协调状态与运行态快照，不承担活连接真相**

### 3.2 三类“真相”定义

#### 持久化真相（PostgreSQL）

由 PostgreSQL 保存：

- 用户
- 服务配置
- 工具元数据
- 请求历史
- 审计日志

#### 活连接真相（Executor 本地内存）

由 executor 本地连接对象承担：

- 某个连接是否真实存活
- 当前 session 是否仍有效
- transport 原生运行态
- 当前监听是否真实激活

#### 跨副本协调状态与快照（Redis）

由 Redis 保存：

- token 黑名单
- owner lease
- epoch / fencing token
- 运行态快照
- 并发计数 / 限流计数
- executor 心跳与负载信息

> Redis 不是活连接真相来源，只是跨副本协调状态与查询快照来源。

---

## 4. 目标架构

### 4.1 最终目标方向

最终建议演进为：

- 单代码库
- 双部署角色
- `control-plane` / `executor` 分离
- PostgreSQL + Redis
- 内部 RPC

### 4.2 过渡形态

在真正双角色部署前，先支持以下三种运行模式：

- `app.role = all`
- `app.role = control-plane`
- `app.role = executor`

其中：

- `all`：本地开发、测试、单机部署、回退模式
- `control-plane`：对外 API、配置读写、权限、历史/审计查询、后台管理任务
- `executor`：MCP 长连接、健康检查、工具读取、工具调用

### 4.3 网络暴露面建议

当前项目是单个 Gin 服务统一对外暴露路由，后续拆角色时需要先明确每个角色暴露什么：

- `control-plane`
  - 对外 REST API
  - `/health`
  - `/ready`
  - 可选 `metrics` / `pprof`
  - 内部 RPC client，不直接持有长连接
- `executor`
  - 不对外暴露业务 REST API
  - 保留 `/health`
  - 保留 `/ready`
  - 可选 `metrics` / `pprof`
  - 暴露内部 RPC server
- `all`
  - 同时具备 control-plane 与 executor 的能力，主要用于开发、测试与回退

> 对当前仓库而言，executor 更适合作为“内部执行节点”，而不是第二个对外 API 服务。

---

## 5. 启动编排重构（Phase 0 前置，必须先做）

当前项目在真正拆分角色前，必须先完成**启动编排重构**。

### 5.1 目标

把当前 `cmd/server/main.go` 中的单体式装配过程拆成可组合模块，为以下能力做准备：

- 根据 `app.role` 决定初始化哪些组件
- 保留 `all` 模式用于开发和回退
- 避免未来把角色判断散落到业务代码中

### 5.2 建议的装配模块

建议逐步拆分为以下构建单元：

- `BuildCore`
  - 配置
  - 日志
  - 数据库
  - 仓储
- `BuildAuth`
  - JWT
  - TokenBlacklistStore
  - AuthService
- `BuildBootstrap`
  - 初始化管理员账号
  - 其他一次性启动引导逻辑
- `BuildExecutorRuntime`
  - `mcpclient.Manager`
  - 健康检查器
- `BuildBackgroundTasks`
  - 审计清理
  - 其他后台任务
- `BuildHTTP`
  - handler
  - router
  - middleware

### 5.3 后台任务与启动引导归属

在支持 `all / control-plane / executor` 三种角色前，必须先定义后台任务归属，避免多实例重复执行或角色越权执行。

| 能力/任务 | 建议归属 | 说明 |
| --- | --- | --- |
| `EnsureAdmin` | `all` / `control-plane` / 独立 init job | 属于启动引导，不应在 executor 全量运行 |
| `AuditCleanupTask` | `all` / `control-plane` / 单实例 job | 属于数据库维护任务，应避免多实例重复清理 |
| `HealthChecker` | `all` / `executor` | 依赖本地 `Manager` 与活连接真相 |
| `Manager` | `all` / `executor` | control-plane 不直接持有长连接 |

### 5.4 验收标准

- 单体模式行为不变
- `all` 模式仍可完整运行
- 支持根据 `app.role` 决定是否初始化 `Manager`、健康检查器和后台任务
- 启动引导与后台任务具备明确归属，不会由 executor 误跑 control-plane 任务
- 角色判断主要停留在装配层，而不是业务层

---

## 6. 执行边界抽象（先小接口，不先大网关）

当前项目不适合一开始引入单一大而全的 `ExecutorGateway`。  
更适合的方式是：**按 service 层消费方拆分多个小接口**。

### 6.1 设计目标

把当前 service 层对 `*mcpclient.Manager` 的直接依赖改造成可替换抽象，并允许：

- 本地实现
- 远程实现
- 单体与双角色模式并存

### 6.2 建议接口拆分

#### ServiceConnector

用于服务连接管理：

- `ConnectService`
- `DisconnectService`

#### RuntimeStatusReader

用于读取运行态：

- `GetRuntimeStatus`

#### ToolCatalogExecutor

用于读取远端工具目录：

- `ListTools`

> 不建议把 `SyncTools` 放进执行器抽象。当前项目中“同步工具”是应用服务行为：执行器只负责从远端读取工具信息，是否落库、如何落库仍由 `ToolService` 和仓储层负责。

#### ToolInvoker

用于工具调用：

- `InvokeTool`

### 6.3 对应当前代码落点

优先改造以下服务：

- `internal/service/mcp_service.go`
- `internal/service/tool_service.go`
- `internal/service/tool_invoke_service.go`

### 6.4 实现形态

建议分两类实现：

- 本地实现：直接调用 `mcpclient.Manager`
- 远程实现：通过内部 RPC 调 executor

### 6.5 设计要求

- 接口定义靠近 `service` 层
- 不提前做成“神接口”
- 不让 transport 细节泄漏到 handler 层
- 不让 control-plane 直接持有 `Manager`

---

## 7. RuntimeStore 设计

### 7.1 设计定位

`RuntimeStore` 只负责：

- 保存运行态快照
- 提供 control-plane 查询来源
- 提供跨副本共享读模型

`RuntimeStore` **不是真连接真相来源**。

### 7.2 建议实现

- `InMemoryRuntimeStore`
- `RedisRuntimeStore`

### 7.3 运行态快照建议字段

- `service_id`
- `executor_id`
- `epoch`
- `status`
- `failure_count`
- `last_error`
- `transport_type`
- `updated_at`
- `ttl`

### 7.4 查询优先级

为了贴合当前项目真实状态查询模型，建议明确以下优先级：

- `all / executor` 模式：**优先本地执行器的一手运行态**
- `control-plane` 模式：**读取 `RuntimeStore` 快照**
- 无本地运行态且无有效快照时：回退为数据库中的持久化视图

### 7.5 一致性要求

- 运行态快照必须带 `executor_id` 与 `epoch`
- control-plane 查询状态时，应区分：
  - 持久化视图
  - 最新运行态快照
- 过期快照不能被当作真实连接状态

---

## 8. 连接空闲回收与业务活跃度治理

当前项目的连接生命周期由 `mcpclient.Manager` 统一管理，因此“空闲断开”能力应作为执行面连接治理的一部分纳入架构设计，但需要避免与健康检查、状态探活、监听型连接产生语义冲突。

### 8.1 设计目标

- 支持按服务配置空闲断开策略，而不是全局硬编码
- 区分“连接仍存活”与“最近发生真实业务使用”
- 避免健康检查把连接永久保活
- 避免监听型连接被误回收
- 避免执行中的工具调用被后台回收中断

### 8.2 服务级配置

建议在 `MCPService` 上新增服务级空闲配置：

- `idle_timeout`：单位秒，`0` 表示禁用空闲回收

原因：

- 不同 transport 的连接成本和复用价值不同
- `stdio`、`sse`、`streamable_http` 的适配策略不同
- 当前项目已经采用“服务自带连接配置”的模型，适合继续沿用服务级配置而不是全局配置

### 8.3 运行态字段拆分

当前运行态中的 `LastSeenAt` 更适合表示“最近一次确认连接仍活着”的时间，不适合直接复用为“业务活跃时间”。因此建议新增独立字段：

- `LastSeenAt`：连接存活探测时间
- `LastUsedAt`：最近一次真实业务使用时间

两者必须严格分离，避免健康检查和被动通知错误延长连接生命周期。

### 8.4 何时刷新 `LastUsedAt`

建议仅在以下真实业务操作成功后刷新：

- `Connect`
- 业务侧 `ListTools`
- `CallTool`
- 显式 `Status`

### 8.5 哪些动作不能刷新 `LastUsedAt`

以下动作只能刷新连接存活信息，不得刷新业务活跃时间：

- `Ping`
- 健康检查触发的探活逻辑
- 健康检查 fallback 的 `ListTools`
- 被动通知 / 监听回调

> 对当前项目尤其要注意：健康检查在 ping 不支持时会 fallback 到 `ListTools`，因此必须区分“业务 ListTools”和“探活 ListTools”的语义。

### 8.6 监听型连接的默认策略

对于 `listen_enabled=true` 的服务，建议默认 **不参与空闲回收**，或后续通过单独配置显式放开。原因是这类连接天然承担长驻监听职责，不应按普通空闲请求连接处理。

### 8.7 Idle Reaper 设计

建议增加独立后台任务 `idle reaper`，运行在：

- `all`
- `executor`

其职责是：

- 周期扫描 `Manager` 当前连接池
- 判断连接是否超过 `idle_timeout`
- 满足条件时发起安全断开

注意：

- idle reaper 不能只调用内存层 `manager.Disconnect`
- 断开后还必须同步更新数据库状态并记录原因，避免出现“内存已断开但 DB 仍显示 CONNECTED”的不一致
- 该行为可沿用类似健康检查的回调模式，由应用层负责持久化状态更新与审计记录

### 8.8 正在执行请求保护

空闲回收必须具备 in-flight 保护，避免在工具调用过程中误断开连接。

建议在受管连接上增加：

- `in_flight` 计数
- `closing` 标记

只有在以下条件同时满足时，idle reaper 才允许断开：

- `idle_timeout > 0`
- `listen_enabled = false` 或显式允许回收
- `in_flight = 0`
- `closing = false`
- `now - LastUsedAt > idle_timeout`

建议采用“两段式关闭”：

1. 先标记 `closing = true`
2. 再确认 `in_flight = 0`
3. 然后执行 disconnect

---

## 9. 启动后一致性修复（必须补充）

在进入分布式前，必须先解决**进程重启后的运行态一致性问题**。

### 9.1 问题

进程重启后：

- 本地 `Manager` 清空
- 数据库存储的服务状态可能仍然显示为 `CONNECTED` / `ERROR`

这会让 API 返回看似“还连着”的状态，但实际连接已经不存在。

### 9.2 建议策略

启动时必须执行以下两类策略中的至少一种：

#### 策略 A：启动重置

将无本地运行态支撑的服务状态统一重置为：

- `DISCONNECTED`

#### 策略 B：启动 reconcile

通过本地 `Manager` 实际状态重新对齐数据库中的状态字段。

### 9.3 建议

当前阶段优先采用：

- **单体模式下启动重置**
- 后续在 executor 模式引入更细粒度 reconcile

---

## 10. TokenBlacklistStore 设计

### 10.1 设计目标

把当前 `JWTService` 对进程内 `TokenBlacklist` 的直接依赖替换为抽象存储接口。

### 10.2 建议实现

- `InMemoryTokenBlacklistStore`
- `RedisTokenBlacklistStore`

### 10.3 迁移要求

- 单机模式可继续使用内存实现
- 多副本模式切换到 Redis 实现
- 可选本地短 TTL 缓存作为性能优化
- Redis 不可用时必须定义明确降级策略

---

## 11. 历史 / 审计链路治理

### 11.1 当前定位

当前项目中，历史与审计链路并不对称：

- `Audit` 已具备 sink 抽象，可继续演进
- `History` 仍绑定在调用路径里直接写仓储

因此不能把两者都视为“已有可替换 sink，只需平滑切换”。

### 11.2 建议演进方向

优先形成统一模式：

- `SyncDBAuditSink`
- `AsyncAuditSink`
- `HistorySink`（新增抽象）
- `SyncDBHistorySink`
- `AsyncHistorySink`

### 11.3 当前阶段的重点

在单体阶段，优先级应是：

1. 保持同步写可用
2. 先把 History 从直接 repo 写入中抽离为可替换抽象
3. 改造导出链路为真正流式 / 游标式
4. 异步 sink 作为可选增强，而不是第一阶段前置条件

### 11.4 导出治理要求

审计导出、历史导出不应继续依赖“大分页 + 一次性拼装全量结果”的方式，而应支持：

- 游标
- 流式写出
- 分批读取

---

## 12. transport 调度模型

该章节方向保留，但明确它属于 **Phase 2 之后能力**。

### 12.1 必须 sticky 的场景

以下服务建议 sticky 到某个 executor：

- `stdio`
- `sse`
- `streamable_http` 且 `session_mode=required`
- `streamable_http` 且 `listen_enabled=true`

### 12.2 可无 owner 的场景

以下服务可按请求即时调度：

- `streamable_http` 且无 session 依赖
- `session_mode=disabled`
- `listen_enabled=false`

### 12.3 调度规则

- sticky 服务：优先 owner 路由
- 无 owner 时：尝试抢占 owner
- 非 sticky 服务：从健康 executor 中选择执行，不建立长期 owner

---

## 13. 分阶段实施

## 13.1 Phase 0：单体基座修正（必须先做）

这是必须先完成的阶段，目标不是引入未来态能力，而是先把**当前单体的状态真相、装配边界、测试基建**做对。

### 实现内容

#### 启动编排重构

- 拆分 `buildApp`
- 把启动装配拆成可组合构建单元
- 为未来 `all / control-plane / executor` 预留装配入口，但**本阶段不改变部署形态**
- 明确健康检查器、清理任务、HTTP 装配的归属边界

#### 当前单体真相修复

- 启动时将**无本地运行态支撑**的 `CONNECTED / ERROR` 重置为 `DISCONNECTED`
- 明确数据库状态、内存运行态、对外 `status` 视图三者关系
- `status` 返回应开始区分：
  - `persisted_status`
  - `runtime_status`
  - `status_source`
  - `snapshot_freshness`

#### 最小边界抽象

仅引入后续演进所需的最小接口，不先引入大网关：

- `ServiceConnector`
- `RuntimeStatusReader`
- `ToolCatalogExecutor`
- `ToolInvoker`
- `TokenBlacklistStore`（仅接口预留）
- `RuntimeStore`（仅接口与 DTO 预留）

> `HistorySink` / `AuditSink` / 完整 RPC / lease 机制不作为本阶段必须项，避免 Phase 0 过重。

#### 测试基建升级

- 将 `tests/testutil/app.go` 演进为**可配置测试装配器**
- 支持注入 db / manager / runtime 相关依赖
- 为后续 SQLite / PostgreSQL、单体 / 双角色测试矩阵做准备

#### 观测与配置预埋（收缩版）

保留必要日志与错误分类，允许增加**最小健康 / 状态观测埋点**；但以下内容**不作为本阶段退出条件**：

- 完整 Prometheus 指标体系
- `pprof`
- 全套压测基线

可预留但默认不启用：

- `app.role`
- `runtime.*`
- `redis.*`
- `rpc.*`
- `metrics.*`

### 验收标准

- 单体运行逻辑不变
- 启动后不会继续暴露明显错误的“伪 CONNECTED / ERROR”状态
- service 层不再直接硬编码依赖本地 `Manager`
- `status` 语义开始具备 persisted / runtime / source 区分能力
- 测试装配器已可支撑后续 PostgreSQL / Redis / dual-role 场景

---

## 13.2 Phase 1：单体内 PostgreSQL 能力落地

此阶段仍保持**单体部署**。目标是先完成数据层双引擎，而不是同时引入 Redis / 双角色。

### 实现内容

#### PostgreSQL 支持

- `database.driver = sqlite | postgres`
- 数据库驱动、初始化与连接池支持 PostgreSQL
- 保持现有实体模型尽量稳定
- 保持 `repository` 分层不变

必须显式拆成三类工作：

1. **配置 / 驱动 / 初始化层**
   - `config.Validate` 不再拒绝 postgres
   - `database.Init` 真正支持 postgres
   - `go.mod` 引入 PostgreSQL 驱动
2. **repository / 方言兼容层**
   - 唯一冲突判断归一化，不能依赖 SQLite 错误字符串
   - migration 与 partial index 兼容性验证
   - `internal/repository/tool_repository.go` 中的 `BatchUpsert` 改造成 PostgreSQL 友好的原子 upsert，这应视为 **Phase 1 必改项**
   - `tags` 查询从 SQLite `LIKE` 语义转向 PostgreSQL 友好的 JSON / JSONB 语义
3. **迁移 / 回滚层**
   - 明确 SQLite → PostgreSQL 数据迁移步骤
   - 明确校验方法与失败回切路径

#### JSON / JSONB 决策

- 对 PostgreSQL 中有查询诉求的 JSON 字段，优先统一落为 `jsonb`
- `mcp_services.tags` 明确使用 `jsonb` + 合适索引
- Phase 1 **不引入 tags 关系表**，优先保留现有实体模型，控制改造面

#### 测试矩阵

- SQLite / PostgreSQL 双矩阵
- migration / rollback / smoke test
- 仓储层重点覆盖：唯一冲突、partial index、upsert、分页 / 排序、JSON 字段兼容

### 验收标准

- 单体模式可在 SQLite / PostgreSQL 下正常运行
- `sync-tools` 在 PostgreSQL 下具备原子 upsert 语义
- `tags` 查询不再依赖 SQLite `LIKE`
- SQLite → PostgreSQL 的迁移、校验与回切步骤已演练，而不是只停留在文档层

---

## 13.3 Phase 2：分布式前置契约与 Redis 最小接入

此阶段仍不拆角色。目标是**先收敛契约，再接入 Redis**，避免未来 Redis 改造演变成横切式重写。

### 实现内容

#### TokenBlacklistStore 接口化

- `JWTService` 不再直接绑定内存 `*TokenBlacklist`
- 先保留内存实现
- 再接入 Redis 实现
- Redis 异常时必须有明确降级行为

#### RuntimeStore 契约定义

- 先定义 `RuntimeStore` 接口、快照结构与状态来源字段
- 明确快照字段至少包括：
  - `last_seen_at`
  - `last_used_at`
  - `in_flight`
  - `listen_active`
  - `snapshot_freshness`
  - `status_source`

#### 运行态采集补全

- 当前只有 `LastSeenAt`，本阶段先补 `LastUsedAt / in_flight` 等最小埋点
- `idle_timeout`、reaper 相关配置只做前置字段准备
- **不把完整 idle reaper 作为本阶段硬验收项**

#### Redis 最小接入范围

本阶段 Redis 只承载：

- token blacklist
- 轻量 runtime snapshot

以下内容**只保留抽象或文档，不正式启用**：

- owner lease
- epoch / fencing
- executor 调度负载治理

### 验收标准

- 单体模式在“无 Redis / 有 Redis”两种模式都能工作
- 登录 / 刷新 / 登出走抽象黑名单 store
- `status` 能表达 persisted / runtime / source / freshness
- 新增运行态字段不会破坏当前 API 语义

---

## 13.4 Phase 3：双角色灰度（先部署与探针，再最小 RPC）

完成前三阶段后，再进入本阶段。目标是**最小可灰度**，而不是一次性把双角色、RPC、lease/fencing 全打包上线。

### 实现内容

#### role-aware 启动与部署

- 支持：
  - `app.role = all`
  - `app.role = control-plane`
  - `app.role = executor`
- control-plane 不持有 MCP 长连接
- executor 持有 `Manager` 与健康检查器
- 提供 role-aware 配置、部署工件与 smoke 脚本

#### `/health` 与 `/ready`

- `/health`：liveness，只表示进程存活
- `/ready`：按角色检查关键依赖
- 双角色灰度前，必须先具备 readiness 语义与探针切换策略

#### 最小内部 RPC

首版 RPC 只要求打通最小能力集：

- `ConnectService`
- `DisconnectService`
- `ListTools`
- `InvokeTool`
- `GetRuntimeStatus`
- `PingExecutor`

约束如下：

- 只暴露执行器原生能力，不直接承载应用层复合语义
- 先验证接口与装配边界，不先追求大而全协议
- `SyncTools` 仍由 control-plane 决定何时、如何写库

#### 本阶段明确不做

- owner lease + epoch / fencing 正式启用
- executor 复杂负载调度
- 完整 transport 分池

这些能力应在双角色稳定后再进入下一阶段。

### 验收标准

- `all / control-plane / executor` 三种模式均可启动
- `/ready` 可真实反映角色依赖是否就绪
- 最小 RPC 路径可跑通
- dual-role 可灰度发布，且能明确回切到 `all` 模式

---

## 13.5 Phase 4：执行面增强（双角色稳定后）

只在双角色已经稳定、灰度与回切路径已验证后，再进入本阶段。

### 实现内容

- executor 级并发限制
- service 级并发限制
- 用户级限流
- 历史 / 审计 / 告警逐步异步 sink 化
- 可选新增 async invoke API
- 长耗时调用支持取消、超时透传、任务查询
- 若 `LastUsedAt / in_flight` 语义已稳定，再正式引入 idle reaper
- 若双角色调度中已出现真实 owner 冲突，再正式引入 owner lease + epoch / fencing

### 验收标准

- 在目标并发下系统退化可控
- 队列积压不拖垮同步链路
- 长任务可查询与取消
- idle reaper / lease / fencing 如启用，均必须支持关闭与回退

---

## 13.6 Phase 5：热点驱动拆分

仅在以下条件满足时再考虑：

- 历史 / 审计查询显著压垮主库
- 某类 transport 成为独立容量热点
- 团队规模与运维能力已支持独立服务治理

### 优先拆分方向

- 历史 / 审计读链路独立
- executor 按 transport 拆池
- 不默认拆 `auth/user` 为独立微服务

---
## 14. Redis 设计定位

Redis 的职责应按阶段分层定义，而不是一次性全部启用：

### Phase 2 立即落地

- token 黑名单
- 运行态快照（共享读模型，不是真相源）

### Phase 4 视真实需求启用

- owner lease
- epoch / fencing token
- 限流与并发计数
- executor 心跳与负载信息

### 14.1 建议 Key

- `mcp:auth:blacklist:{token_sha256}`
- `mcp:service:runtime:{service_id}`
- `mcp:service:owner:{service_id}`
- `mcp:service:epoch:{service_id}`
- `mcp:service:sem:{service_id}`
- `mcp:executor:load:{executor_id}`

### 14.2 租约规则

- sticky 服务通过 `SET NX EX` 抢占 owner
- executor 周期续租
- 抢占成功后递增 `epoch`
- 运行态写入带 `epoch`
- 旧 epoch owner 不允许继续更新状态或执行敏感操作

> Lease / epoch 机制**不得**在首个 dual-role 灰度阶段就作为硬前提启用；只有当最小双角色路径已稳定、且确实存在 owner 冲突问题时，才进入正式发布面。

---
## 15. 内部 RPC 设计建议

建议在 Phase 3 引入内部执行服务边界，但首版只做**最小能力集**，不做大而全抽象。

### 15.1 建议能力

- `ConnectService`
- `DisconnectService`
- `ListTools`
- `InvokeTool`
- `GetRuntimeStatus`
- `PingExecutor`

### 15.2 设计说明

- RPC 只暴露执行器原生能力，不直接承担应用层“同步工具并落库”语义
- `SyncTools` 仍应由 control-plane / `ToolService` 调用 `ListTools` 后决定何时、如何写库
- 先用本地接口实现验证边界是否合理，再固化 wire contract
- owner lease / epoch / fencing 与复杂调度不属于首版 RPC 范围，应在执行面增强阶段按需叠加

### 15.3 关键字段建议

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

### 15.4 关键约束

- sticky 服务上的执行类操作必须校验 `expected_epoch`
- epoch 不匹配应直接拒绝
- `request_id` 用于日志关联与排障
- 第一版不要求业务级强幂等

---

## 16. 数据库与索引建议

### 16.1 PostgreSQL 替换原则

- 保持实体模型尽量稳定
- 保持 `repository` 分层不变
- 尽量减少 service 层重写
- 优先修复数据库方言耦合点

### 16.2 必加索引建议

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

- 保留活跃记录唯一索引

#### `tools`

- 保留 `(mcp_service_id, name)` 活跃唯一索引

### 16.3 JSON / JSONB 策略

当前实体中大量字段使用 `gorm:"type:json"`，迁移 PostgreSQL 时不能只停留在“后续再评估”的层面，需要给出更贴合当前仓库查询形态的明确策略。

#### Phase 1 决策

- 对 PostgreSQL 中有查询诉求的 JSON 字段，**优先统一落为 `jsonb`**
- 其中 `mcp_services.tags` 作为现有查询入口，**明确使用 `jsonb` + GIN 索引**
- 当前 SQLite 阶段对 `tags` 的 `LIKE` 查询应在 PostgreSQL 阶段替换为 `@>` 等 JSON 查询语义
- Phase 1 **不引入 tags 关系表**，优先保留现有实体模型，通过 `jsonb` 收敛改造范围

#### 迁移要求

- 明确 GORM tag 是否统一调整为 PostgreSQL 友好的 JSON 类型
- 明确哪些 JSON 字段参与查询、过滤或索引
- 迁移脚本与回滚脚本要覆盖 JSON 类型变更
- 对请求历史、审计日志等大 JSON 字段，至少完成兼容性测试与必要索引评估

> 简单说：Phase 1 不再维持 `tags LIKE '%...%'` 这种 SQLite 风格查询，而是明确转向 PostgreSQL 的 `jsonb` 查询与索引方案。

### 16.4 代码层注意事项

迁移 PostgreSQL 时必须统一处理：

- 唯一冲突错误归一化
- migration 与索引兼容
- upsert 行为一致性
- JSON 字段兼容性
- 排序与分页稳定性

---

## 17. 健康检查、就绪探针与状态 API 语义

### 17.1 `/health` 与 `/ready`

当前项目只有简单的 `/health` 路由，拆角色后需要显式区分：

- `/health`：liveness，只表示进程存活
- `/ready`：readiness，按角色检查关键依赖是否可用

建议：

- `control-plane /ready`
  - DB
  - Redis
  - 内部 RPC client（如启用）
- `executor /ready`
  - Redis
  - 内部 RPC server
  - 本地 runtime 组件
  - DB（若 executor 仍需写历史/审计）

### 17.2 `status` API 的最终语义

当前项目中的状态查询本质上是**数据库持久化状态 + 本地 manager 运行态覆盖后的聚合结果**。进入双角色后，应显式定义 control-plane 对外返回的状态语义：

- `persisted_status`：数据库中的持久化状态
- `runtime_snapshot`：来自 `RuntimeStore` 或本地执行器的一手运行态
- `snapshot_freshness`：快照新鲜度 / 是否过期
- `source`：状态来源（local / runtime_store / db）

> control-plane 返回的 `status` 应被定义为“聚合视图”，而不是活连接真相。

---

## 18. API 语义兼容说明

升级过程中应尽量保持现有 REST API 路径与基础语义不变：

- `connect`
- `disconnect`
- `status`
- `sync-tools`
- `invoke`

但在非 sticky transport 场景下，应明确以下语义：

- `connect` 是否仍表示建立长期连接，还是仅表示预热/准备
- `disconnect` 是否仍具有强语义
- `status` 返回的是持久化视图、运行态快照，还是活连接状态
- 非 sticky 场景是否允许 `invoke` 直接走即时调度而跳过显式 connect

文档在 Phase 2 落地前必须明确这些对外语义。

---

## 19. 对现有代码的影响边界

### 19.1 handler

- REST API 尽量保持兼容
- 新接口优先增量增加，不破坏现有路径
- 需要明确查询类 handler 的边界规范

### 19.2 查询链路规范

当前仓库并不是所有查询都走 service：

- `HistoryHandler` 直接依赖 `RequestHistoryRepository`
- `AuditHandler` 走 `AuditService`

为避免后续 control-plane 改造时继续放大这种分层不一致，本文档在当前阶段明确选择：

- **History 查询暂时保留 handler 直连 repository，作为已知例外**
- **Audit 查询继续走 `AuditService`**
- 当 History 查询出现跨源聚合需求（例如运行态增强、跨库聚合、权限模型复杂化）时，再引入 `HistoryReadService`

这样做的原因是：

- 更贴合当前仓库真实实现
- 避免在 Phase 0/1 同时引入过多读路径重构
- 为后续 control-plane 查询面演进预留明确升级点

### 19.3 service

优先把以下服务改造成面向抽象编程：

- `MCPService`
- `ToolService`
- `ToolInvokeService`
- `AuthService`（针对 blacklist 抽象）

### 19.4 mcpclient

- 保留 executor 内核心实现
- `Manager` 最终只存在于 `executor` 或 `all` 模式

### 19.5 repository

- 保留 GORM 仓储模式
- 升级数据库驱动
- 统一错误归一化
- 优化 upsert 与索引

### 19.6 config

新增但保留默认值：

- `app.role`
- `database.driver`
- `database.dsn`
- `redis.*`
- `rpc.*`
- `runtime.*`
- `invoke.*`
- `metrics.*`

---

## 20. 测试与验收策略

### 20.1 Phase 0

- 为执行抽象补充单测 / mock 测试
- 保留现有 sqlite 单测与 integration 测试
- 增加启动 reset / reconcile 测试
- 将 `tests/testutil/app.go` 演进为**可配置测试装配器**，而不是继续只服务于“单 Gin app + sqlite in-memory + 本地 Manager”的固定模式
- 增加 `status` 语义测试，覆盖 persisted / runtime / source 差异

### 20.2 Phase 1

新增 SQLite / PostgreSQL 双矩阵测试，重点验证：

- 唯一冲突
- partial index
- upsert
- 分页 / 排序
- JSON / JSONB 字段兼容
- SQLite → PostgreSQL 迁移、校验与回切流程

### 20.3 Phase 2

新增 Redis 相关测试，重点验证：

- token 在多副本下立即失效
- 黑名单 store 的降级路径
- 运行态快照的 `source / freshness`
- `LastUsedAt / in_flight` 等运行态字段不会破坏现有 API 语义

### 20.4 Phase 3

新增 dual-role 集成测试矩阵：

- `all`
- `control-plane + executor`
- PostgreSQL + Redis
- `/health` 与 `/ready`
- 最小 RPC 路径
- 灰度发布与回切 `all` 模式

### 20.5 功能回归

- 登录 / 刷新 / 登出
- 用户管理
- 服务 CRUD
- connect / disconnect / status
- sync-tools
- invoke
- history list / get
- audit list / export

### 20.6 执行面与分布式行为验证

仅在 Phase 4 正式启用相关能力后，再要求以下验证：

- sticky 服务不会双 owner 执行
- executor 宕机后可接管
- epoch 不匹配时旧 owner 不得继续更新状态或执行调用
- `idle_timeout=0` 时不会发生自动断开
- 业务 `ListTools` / `CallTool` 会刷新 `LastUsedAt`
- `Ping` 与健康检查 fallback 不会刷新 `LastUsedAt`
- `listen_enabled=true` 默认不会被 idle reaper 回收
- 工具调用执行中不会被 idle reaper 误断开

---
## 21. 当前阶段明确不建议做的事

以下动作不建议作为当前阶段默认动作：

- 直接全面微服务化
- 在没有抽象层前把 service 逻辑整体改成 RPC
- 一开始就引入 MQ 承担同步 request-reply
- 把 Redis 定义为唯一运行态真相
- 在单体阶段让 lease / epoch 大规模侵入业务逻辑
- 过早拆 `auth/user` 独立服务
- 在 Phase 0 前锁死 gRPC 为唯一内部协议

---

## 22. 最终结论

**这次升级的正确方向不是“立刻拆分式重构”，而是“先把当前单体做成可分离单体”。**

对于当前 `mcp-manager`，更合适的执行顺序是：

1. **启动编排重构**
2. **观测与边界抽象**
3. **单体内基础设施升级**
4. **双角色部署**
5. **执行面增强**
6. **按热点拆分**

一句话总结：

> 先把系统做成“可分离单体”，再把它拆成“控制面 + 执行面”。
