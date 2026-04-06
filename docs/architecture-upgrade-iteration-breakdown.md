# 架构升级计划拆分：迭代版本与任务列表（修订版）

## 1. 需求摘要

目标是把 `docs/architecture-upgrade-plan.md` 拆成**可执行、低风险、依赖闭合、可验证、可回退**的迭代版本，而不是直接照搬未来态设计。

当前仓库的关键约束：

- 启动装配仍集中在 `buildApp`，包含数据库、迁移、JWT、Manager、健康检查、清理任务与路由装配，属于强耦合单体启动模型：`cmd/server/main.go:116-170`
- `MCPService`、`ToolService`、`ToolInvokeService` 仍直接依赖 `*mcpclient.Manager`，说明执行面边界还未抽离：`internal/service/mcp_service.go:45-61`、`internal/service/tool_service.go:23-36`、`internal/service/tool_invoke_service.go:31-42`
- 配置与数据库初始化当前仅支持 SQLite：`internal/config/config.go:205-214`、`internal/database/database.go:20-54`
- 路由当前只有 `/health`，没有 `/ready`，部署配置仍是单服务单角色：`internal/router/router.go:25-32`、`deployments/docker/docker-compose.yml:1-14`
- 运行态只有 `LastSeenAt` / `ListenActive` 等字段，没有 `LastUsedAt` / `in_flight`，说明 idle reaper 和复杂调度还没有地基：`internal/mcpclient/managed_client.go:214-233,343-352`
- 测试装配器写死 sqlite in-memory + 本地 manager，说明一旦开始拆角色，测试基建必须先升级：`tests/testutil/app.go:31-81`
- `go.mod` 当前没有 PostgreSQL 驱动，数据库切换不是简单配置改动：`go.mod:5-22`

---

## 2. 规划原则

1. **先修正当前单体真相，再引入未来态能力**：先解决启动后伪状态、`status` 语义和测试装配器问题，再进入 PG / Redis / 双角色。
2. **先稳定契约，再替换实现**：先抽 `TokenBlacklistStore`、`RuntimeStore`、执行接口，再接 Redis / RPC。
3. **先双基础设施，再双角色**：先完成 PostgreSQL 可用和迁移回滚，再进入 control-plane / executor。
4. **先部署与探针，再复杂调度**：首个双角色阶段只做最小可灰度，不把 owner lease / epoch / fencing 绑进去。
5. **每个版本必须可验证、可回退**：不能出现“只能靠人工理解未来态才能验收”的版本。

---

## 3. 迭代版本拆分总览

| 迭代版本 | 对应主计划 | 目标 | 是否改变部署形态 |
| --- | --- | --- | --- |
| V1.0 单体基座修正 | Phase 0 | 修正启动状态真相、装配边界、测试基建 | 否 |
| V1.1 PostgreSQL 能力落地 | Phase 1（上半） | 完成 SQLite / PostgreSQL 双支持与迁移回滚 | 否 |
| V1.2 分布式前置契约 | Phase 2（收缩版） | 完成 Redis 最小接入前的接口与运行态契约收敛 | 否 |
| V1.3 双角色灰度 | Phase 3 | 完成 role-aware 部署、`/ready`、最小 RPC 与灰度回退 | 是 |
| V1.4 执行面增强 | Phase 4 | 限流、异步化、idle reaper、lease/fencing 按需启用 | 是 |
| V2.0 热点驱动拆分（条件触发） | Phase 5 | 独立读链路 / transport 分池 | 是，按条件 |

说明：相较旧版，把 PostgreSQL、Redis/RuntimeStore、双角色灰度、lease/fencing 明确拆开，避免同一版本承担过多风险源。

---

## 4. 版本拆分详情

## V1.0 单体基座修正

### 目标

在**不改变部署形态**的前提下，先把当前单体最关键的错误前提修正掉，让后续迭代建立在真实状态基线上。

### 范围

- 拆分 `buildApp` 装配职责：`cmd/server/main.go:116-170`
- 抽离执行面最小接口，去掉 service 对 `*mcpclient.Manager` 的硬编码依赖：`internal/service/mcp_service.go:45-61`、`internal/service/tool_service.go:23-36`、`internal/service/tool_invoke_service.go:31-42`
- 补齐启动后状态一致性治理，避免“伪 CONNECTED/ERROR”
- 升级测试装配器，支持可配置构建：`tests/testutil/app.go:31-81`
- 收敛 `status` 语义，显式区分 persisted / runtime / source
- 不把完整 metrics/pprof/压测都塞进首迭代

### 交付物

- 模块化装配入口
- 本地执行接口（`ServiceConnector` / `RuntimeStatusReader` / `ToolCatalogExecutor` / `ToolInvoker`）
- 启动时 reset / reconcile 逻辑
- 可配置测试装配器
- 最小状态一致性与语义测试

### 任务列表

| ID | 任务 | 触点文件 | 依赖 | 完成标准 |
| --- | --- | --- | --- | --- |
| V1.0-01 | 拆分 `buildApp` 为可组合构建单元 | `cmd/server/main.go` | 无 | 主函数只保留启动/关闭编排 |
| V1.0-02 | 为 `MCPService` / `ToolService` / `ToolInvokeService` 抽最小执行接口 | `internal/service/*` | V1.0-01 | service 层不再直接依赖具体 `Manager` |
| V1.0-03 | 增加启动状态重置 / 一致性修复逻辑 | `cmd/server/main.go`、`internal/service/mcp_service.go` | V1.0-01 | 重启后不会继续暴露错误的 `CONNECTED/ERROR` |
| V1.0-04 | 收敛 `status` 输出契约 | `internal/service/mcp_service.go` | V1.0-02~03 | 返回 persisted / runtime / source 相关字段 |
| V1.0-05 | 将测试装配器改为可配置构建器 | `tests/testutil/app.go` | V1.0-01~02 | 测试可切换 db / manager / runtime 依赖 |
| V1.0-06 | 引入 `app.role`、`runtime.*` 等配置占位，但默认不启用 | `internal/config/config.go`、`config.yaml` | V1.0-01 | 新配置可解析且默认不改变行为 |
| V1.0-07 | 为启动一致性、状态语义、接口抽象补充测试 | `tests/integration`、`tests/testutil/app.go` | V1.0-03~05 | 单测/集成测试覆盖 reset/reconcile 与新接口注入 |

### 验收标准

- 单体运行逻辑不变
- 启动后状态接口不会返回明显错误的连接真相
- 测试装配器能支撑后续 PostgreSQL / Redis / dual-role 测试
- 仍可用现有单体模式回退

### 建议验证

- `go test ./internal/service ./internal/mcpclient ./tests/integration`
- `go test ./...`

---

## V1.1 PostgreSQL 能力落地

### 目标

在保持单体部署的前提下，让仓储层同时支持 SQLite 和 PostgreSQL，并具备真实迁移 / 校验 / 回切能力。

### 范围

- 扩展 `database.driver = sqlite | postgres`
- 扩展 `database.Init`、配置校验与驱动依赖
- 修复 `tags` 查询、唯一冲突、`BatchUpsert` 等方言耦合点
- 明确 SQLite → PostgreSQL 数据迁移、校验、回切流程

### 交付物

- PostgreSQL 初始化与迁移支持
- repository 方言兼容层
- 原子 upsert 实现
- JSON / JSONB 策略落地
- SQLite + PostgreSQL 双栈仓储测试
- 迁移 / 回切操作说明与演练记录

### 任务列表

| ID | 任务 | 触点文件 | 依赖 | 完成标准 |
| --- | --- | --- | --- | --- |
| V1.1-01 | 扩展配置模型，允许 `database.driver=postgres` | `internal/config/config.go`、`config.yaml` | V1.0 | 配置校验不再拒绝 postgres |
| V1.1-02 | 扩展 `database.Init` 支持 PostgreSQL 驱动与连接池 | `internal/database/database.go`、`go.mod` | V1.1-01 | SQLite/PG 都能完成 Init + Health |
| V1.1-03 | 梳理 migration、partial index 与 JSON / JSONB 策略 | `internal/database/*`、实体定义 | V1.1-02 | SQLite/PG 两边 migration 都可重复执行 |
| V1.1-04 | 将服务标签查询改为方言感知实现 | `internal/repository/mcp_service_repository.go` | V1.1-02 | SQLite 与 PG 查询语义一致 |
| V1.1-05 | 将 `ToolRepository.BatchUpsert` 改为原子 upsert | `internal/repository/tool_repository.go` | V1.1-02 | 高并发下不会出现读后写竞争与重复写入 |
| V1.1-06 | 统一唯一冲突、not found、分页排序行为 | `internal/repository/*` | V1.1-03~05 | SQLite/PG 的错误与分页语义统一 |
| V1.1-07 | 提供 SQLite → PostgreSQL 数据迁移、校验、回切步骤 | `internal/database/*`、运维脚本/文档 | V1.1-02~06 | 存在可执行迁移与回滚路径 |
| V1.1-08 | 增加 PostgreSQL 仓储测试矩阵 | `internal/repository/*_test.go`、`tests/testutil/app.go` | V1.1-02~07 | 仓储与切换路径均有测试 |

### 验收标准

- 单体模式可在 SQLite / PostgreSQL 下正常运行
- `sync-tools` 在 PostgreSQL 下具备原子 upsert 语义
- `tags` 查询不再依赖 SQLite `LIKE`
- 迁移与回切步骤已演练，不只是文档声明

### 建议验证

- `go test ./internal/repository/...`
- `go test ./tests/integration/...`
- 双数据库 smoke：服务 CRUD、connect、sync-tools、invoke

---

## V1.2 分布式前置契约

### 目标

在仍不拆角色的情况下，先收敛未来跨进程能力的接口与状态契约，再把 Redis 作为可关闭增强项接入。

### 范围

- 抽象 `TokenBlacklistStore`
- 抽象 `RuntimeStore`
- 补全 `LastUsedAt / in_flight` 等运行态字段
- Redis 只承载黑名单与轻量 runtime snapshot
- 不要求本版正式启用 idle reaper、lease、fencing

### 交付物

- `InMemoryTokenBlacklistStore` + `RedisTokenBlacklistStore`
- `RuntimeStore` 接口与快照结构
- `JWTService` 基于 store 接口而非具体内存实现
- `status` 聚合中的 source / freshness 语义
- 运行态字段前置采集能力

### 任务列表

| ID | 任务 | 触点文件 | 依赖 | 完成标准 |
| --- | --- | --- | --- | --- |
| V1.2-01 | 抽象 `TokenBlacklistStore` 并适配 `JWTService` | `pkg/crypto/*` | V1.0 | JWT 不再只依赖内存黑名单 |
| V1.2-02 | 接入 Redis 黑名单实现与降级策略 | `pkg/crypto/*`、配置文件 | V1.2-01 | 多副本 token 可共享失效，Redis 异常有降级 |
| V1.2-03 | 抽象 `RuntimeStore`，定义快照模型 | `internal/service/mcp_service.go` | V1.0 | 快照模型可表达 status / source / freshness |
| V1.2-04 | 补全 `LastUsedAt / in_flight` 等运行态字段 | `internal/mcpclient/*` | V1.2-03 | 字段可采集但不改变默认策略 |
| V1.2-05 | 在 `all` 模式下写入 Redis 运行态快照，但读取优先本地运行态 | `internal/service/mcp_service.go`、`internal/mcpclient/*` | V1.2-03~04 | 快照仅作共享读模型 |
| V1.2-06 | 预埋 `idle_timeout`、`redis.*`、`runtime.*` 等配置 | `internal/config/config.go`、实体定义 | V1.2-03 | 配置完成向后兼容扩展 |
| V1.2-07 | 增加 Redis 相关单测/集成测试 | `tests/integration`、`tests/testutil/app.go` | V1.2-01~06 | token 失效一致性、快照新鲜度、降级路径可测试 |

### 验收标准

- 单体模式在“无 Redis / 有 Redis”两种模式都能工作
- 登录/刷新/登出可使用抽象黑名单 store
- `status` 在本地运行态缺失时具备快照兜底能力，但不会把快照当连接真相
- idle 生命周期字段存在且不破坏当前 API 行为

### 建议验证

- `go test ./pkg/crypto ./internal/service ./internal/mcpclient`
- Redis integration test：token 失效、一手运行态优先、快照过期处理

---

## V1.3 双角色灰度

### 目标

正式引入 `control-plane` / `executor` 双角色，但只做**最小可灰度版本**，同时保留 `all` 作为回退与开发模式。

### 范围

- 支持 `app.role = all | control-plane | executor`
- 引入 `/ready` 与 role-aware 探针
- 引入最小内部 RPC 边界
- control-plane 不再持有本地 `Manager`
- executor 持有运行态与健康检查器
- 暂不把 owner lease / epoch / fencing 作为首版硬前提

### 交付物

- role-aware 启动流程
- role-aware 配置与部署工件
- 内部 RPC server/client
- 本地执行实现 + 远程执行实现
- dual-role 集成测试矩阵
- 回切 `all` 模式的发布路径

### 任务列表

| ID | 任务 | 触点文件 | 依赖 | 完成标准 |
| --- | --- | --- | --- | --- |
| V1.3-01 | 完成 role-aware 启动装配 | `cmd/server/main.go`、配置模块 | V1.0~V1.2 | `all/control-plane/executor` 三种模式均可启动 |
| V1.3-02 | 提供 role-aware 配置、部署工件与 smoke 脚本 | `deployments/docker/*`、配置文件 | V1.3-01 | dual-role 有真实部署入口 |
| V1.3-03 | 增加 `/ready` 并区分角色依赖检查 | `internal/router/router.go` | V1.3-01 | readiness 语义清晰 |
| V1.3-04 | 为 control-plane 注入远程执行实现 | `internal/service/*` | V1.0、V1.2 | 服务层可在本地/远程执行实现间切换 |
| V1.3-05 | 实现最小 RPC（Connect/Disconnect/ListTools/Invoke/Status/PingExecutor） | 新增 `internal/rpc` 或等价模块 | V1.3-01~04 | executor 暴露最小执行 RPC；control-plane 可调用 |
| V1.3-06 | 明确 `connect/disconnect/status/invoke` 在双角色下的 API 语义 | handler/service 文档与实现 | V1.3-03~05 | 非 sticky 是否允许无显式 connect 必须明确并测试 |
| V1.3-07 | 增加 dual-role 集成测试矩阵 | `tests/integration`、`tests/e2e`、`tests/testutil/app.go` | V1.3-01~06 | `all` 与 `control-plane+executor` 两种部署都通过验证 |

### 验收标准

- dual-role 可以真实部署，不是仅代码层可运行
- `/ready` 可真实反映角色就绪情况
- 最小 RPC 路径可跑通
- `all` 模式仍可作为本地开发与回退模式

### 建议验证

- 双角色 smoke：登录、服务 CRUD、connect、sync-tools、invoke、status
- readiness / rollout / rollback 验证

---

## V1.4 执行面增强

### 目标

在双角色稳定后，再增强执行面吞吐、限流、异步化与长任务能力，并按需启用 idle reaper / lease / fencing。

### 范围

- executor / service / user 三级限流
- 历史 / 审计 / 告警逐步异步 sink 化
- async invoke、取消、超时透传、任务查询
- 若运行态字段已经稳定，再引入 idle reaper
- 若 owner 冲突成为真实问题，再启用 lease / epoch / fencing

### 交付物

- 并发/限流中间层
- 异步 sink 实现
- 长任务控制协议
- idle reaper（可选）
- owner lease + epoch/fencing（可选）

### 任务列表

| ID | 任务 | 触点文件 | 依赖 | 完成标准 |
| --- | --- | --- | --- | --- |
| V1.4-01 | executor 级并发限制 | 执行调度模块 | V1.3 | 可配置上限，超限时行为明确 |
| V1.4-02 | service / user 级限流 | API/执行链路 | V1.3 | 限流规则可命中且可审计 |
| V1.4-03 | History/Audit/Alert 异步 sink 开关化 | `internal/service/*`、history 写链路 | V1.3 | 默认同步，开关后异步且不丢关键数据 |
| V1.4-04 | async invoke API 与任务查询 | handler/service/rpc | V1.3 | 长任务可异步提交、查询状态、取消 |
| V1.4-05 | idle reaper（可选）正式落地 | `internal/mcpclient/*` | V1.2、V1.3 | 仅在启用时生效，且不会误伤 in-flight 请求 |
| V1.4-06 | owner lease + epoch/fencing（可选）正式落地 | Redis / 调度模块 | V1.3 | 旧 owner 不得继续执行敏感操作 |
| V1.4-07 | 队列积压、取消、超时透传的观测与测试 | tests + metrics | V1.4-01~06 | 可测、可观察、可恢复 |

### 验收标准

- 目标并发下系统退化可控
- 队列积压不拖垮同步链路
- 长任务可查询、取消、超时透传
- idle reaper / lease / fencing 如启用，均可关闭回退

### 建议验证

- 并发压测
- 限流命中测试
- 异步 sink 回压测试
- 长任务取消/超时/查询集成测试

---

## V2.0 热点驱动拆分（条件触发）

### 目标

仅在双角色稳定且出现真实容量/团队边界问题后，再做进一步微服务拆分。

### 触发条件

- 历史 / 审计查询显著压垮主库
- 某类 transport 成为独立容量热点
- 团队已经能承接独立服务运维

### 交付物

- 独立历史/审计读链路，或
- executor 按 transport 拆池

### 任务列表

| ID | 任务 | 前提 | 完成标准 |
| --- | --- | --- | --- |
| V2.0-01 | 评估历史/审计读链路是否独立 | V1.4 稳定运行 | 有明确 QPS/延迟/主库压力证据 |
| V2.0-02 | 评估 executor 是否需要按 transport 分池 | V1.4 稳定运行 | 具备独立容量热点或隔离需求 |
| V2.0-03 | 形成单独 ADR 与迁移方案 | V2.0-01 或 V2.0-02 成立 | 基于指标而非概念拆分 |

### 验收标准

- 拆分来自真实瓶颈，不来自概念完备性追求
- `auth/user` 默认不独立拆服务

---

## 5. 跨迭代任务清单（按优先级）

### P0：必须先做

1. 拆 `buildApp` 装配边界
2. 抽 service -> 执行面最小接口
3. 修复启动后伪 `CONNECTED/ERROR`
4. 收敛 `status` 语义
5. 升级测试装配器
6. PostgreSQL 配置 / 驱动 / Init 支持
7. 修复 `tags` 查询和 `BatchUpsert` 的数据库方言耦合

### P1：双角色前必须完成

1. PostgreSQL 迁移、校验、回切路径
2. `TokenBlacklistStore` 抽象与 Redis 实现
3. `RuntimeStore` 抽象与快照读模型
4. `LastUsedAt / in_flight` 等运行态字段
5. role-aware 启动
6. `/ready` 与角色探针语义
7. 内部 RPC 最小能力集
8. dual-role 集成测试矩阵

### P2：双角色稳定后再做

1. owner lease + epoch/fencing
2. executor/service/user 级限流
3. async invoke / async sink
4. idle reaper 正式落地
5. 热点驱动拆分 ADR

---

## 6. 风险与缓解

| 风险 | 说明 | 缓解措施 |
| --- | --- | --- |
| 首阶段过重导致长期无交付 | 启动重构、观测、接口、配置、压测全部混在一起易失控 | 收缩 V1.0，只保留装配、状态一致性、接口与测试基建 |
| PostgreSQL 与 Redis 同迭代引入风险叠加 | 数据层兼容与分布式一致性是两类问题 | 拆成 V1.1 与 V1.2 |
| 双角色阶段同时叠加 RPC + lease/fencing | 当前代码还没有 role-aware 运行模式 | V1.3 只做最小 RPC，lease/fencing 后移到 V1.4 |
| 发布面没有 readiness 与回切路径 | 当前只有 `/health`，没有 role-aware 部署工件 | 在 V1.3 之前先补 `/ready`、部署工件与 smoke |
| 测试基建跟不上架构变化 | 当前测试装配器绑定 sqlite + 本地 manager | V1.0 先改测试装配器，再推进后续迭代 |

---

## 7. 验证步骤

### 版本级验证

- V1.0：单体启动、状态一致性、service 抽象依赖测试
- V1.1：SQLite/PG 仓储测试矩阵、迁移 / 回切演练、服务 CRUD / sync-tools / invoke smoke
- V1.2：Redis token 失效、运行态快照、降级路径测试
- V1.3：dual-role 集成测试、`/ready` 检查、灰度发布与回切验证
- V1.4：并发压测、限流、异步任务取消与查询测试；如启用 lease/fencing，则补 owner 行为验证
- V2.0：基于指标的拆分前后对比验证

### 建议命令

- `go test ./...`
- `make test`
- `make test-e2e`
- `make test-race`
- PostgreSQL / Redis 环境下补充分层集成测试与 smoke 脚本

---

## 8. 建议实施节奏

- **第 1 轮**：V1.0
- **第 2 轮**：V1.1
- **第 3 轮**：V1.2
- **第 4~5 轮**：V1.3（建议拆成“灰度准备 + 灰度稳定”两个小冲刺）
- **第 6 轮以后**：V1.4
- **V2.0**：仅在触发条件满足后启动

---

## 9. 结论

修订后的版本拆分更贴合当前仓库现实：

- 先修正当前单体的真实问题：启动后伪状态、Manager 直连、SQLite-only、测试装配器固定化
- 再拆开 PostgreSQL 与 Redis 两类改造风险
- 再进入 role-aware 部署、`/ready`、最小 RPC 的双角色灰度
- 最后在双角色稳定后再讨论 idle reaper、lease/fencing 与热点拆分

这样拆分后，每个迭代都具备更清晰的代码触点、验收标准与回退边界。