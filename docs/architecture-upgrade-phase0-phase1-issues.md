# mcp-manager Phase 0 / Phase 1 可直接开发的 Issue / Task 列表

> 目标：把 `docs/architecture-upgrade-plan.md` 与 `docs/architecture-upgrade-phase0-phase1-checklist.md` 进一步落成可直接进入开发排期的 issue 清单。

## 使用说明

- `Epic`：可拆分为多个 issue 的主题
- `Issue`：可以直接进入看板的开发项
- `Task`：Issue 下的细分执行步骤
- 优先级：`P0` > `P1` > `P2`
- 估时：按单人开发粗估，单位为**理想工作日**

---

## 1. 推荐排期顺序

### Phase 0

1. ISSUE-00 启动装配拆分与角色预埋
2. ISSUE-01 Metrics 与 pprof 接入
3. ISSUE-02 ExecutorGateway 抽象
4. ISSUE-03 TokenBlacklistStore 抽象
5. ISSUE-04 RuntimeStore 抽象
6. ISSUE-05 History/Audit Sink 抽象补齐
7. ISSUE-06 Phase 0 测试与基线文档

### Phase 1

8. ISSUE-10 PostgreSQL 配置与数据库初始化支持
9. ISSUE-11 Repository 错误归一化与 DB 兼容层
10. ISSUE-12 PostgreSQL 集成测试与迁移验证
11. ISSUE-13 Redis 黑名单实现
12. ISSUE-14 Redis RuntimeStore 实现
13. ISSUE-15 审计导出治理
14. ISSUE-16 Async Sink 可选实现
15. ISSUE-17 Phase 1 回归与发布准备

---

## 2. Epic 列表

## EPIC-P0-OBS：观测与启动边界

目标：

- 让当前单体可观测
- 为 `app.role` 和后续双角色部署预留启动结构

包含：

- ISSUE-00
- ISSUE-01
- ISSUE-06

---

## EPIC-P0-EXEC：执行与状态抽象

目标：

- 让 service 层摆脱对具体本地实现的硬依赖
- 为后续 Redis / RPC 演进留边界

包含：

- ISSUE-02
- ISSUE-03
- ISSUE-04
- ISSUE-05

---

## EPIC-P1-DB：数据库升级

目标：

- 完成 SQLite -> PostgreSQL 的单体模式升级
- 清理仓储层对 SQLite 细节的依赖

包含：

- ISSUE-10
- ISSUE-11
- ISSUE-12

---

## EPIC-P1-STATE：状态外置与写入治理

目标：

- 完成 Redis 黑名单与运行态存储落地
- 降低导出与写入对主链路的影响

包含：

- ISSUE-13
- ISSUE-14
- ISSUE-15
- ISSUE-16
- ISSUE-17

---

## 3. Issue 明细

## ISSUE-00：拆分启动装配并预埋 `app.role`

- Phase：Phase 0
- 优先级：P0
- 估时：1.5 ~ 2 天
- 依赖：无

### 背景

当前 `cmd/server/main.go` 直接拼装数据库、JWT、Manager、health checker、router，后续很难平滑支持 `all / control-plane / executor`。

### 范围

- `cmd/server/main.go`
- `internal/config/config.go`
- 可新增：`internal/app/` 或 `internal/bootstrap/`

### 交付物

- 更清晰的启动装配函数
- `app.role` 配置字段与默认值
- 仍保持当前默认单体行为不变

### 开发任务

- [ ] 抽出 repository 装配函数
- [ ] 抽出 store/gateway/service 装配函数
- [ ] 抽出 HTTP server 装配函数
- [ ] 新增 `app.role` 配置与校验
- [ ] 默认 `app.role=all`
- [ ] 回归现有启动流程

### 验收标准

- `go run ./cmd/server` 仍可启动
- 默认行为与当前一致
- main 文件复杂度明显下降
- 后续可在不重写 main 的情况下接入新角色

---

## ISSUE-01：接入 Metrics 与 pprof

- Phase：Phase 0
- 优先级：P0
- 估时：1 ~ 1.5 天
- 依赖：ISSUE-00 推荐先完成

### 范围

- `cmd/server/main.go`
- `internal/router/`
- 新增：`internal/metrics/`
- `internal/config/config.go`

### 交付物

- metrics 开关与监听配置
- HTTP 指标
- 工具调用耗时指标
- 健康检查指标
- DB 连接池指标
- pprof 入口

### 开发任务

- [ ] 设计 metrics 命名规范
- [ ] 明确 metrics label 白名单，禁止引入 `user_id`、`request_id`、原始 `service_id` 等高基数标签
- [ ] 给 HTTP 路由增加请求总数/耗时/状态码指标
- [ ] 给 tool invoke 增加调用耗时与成功失败指标
- [ ] 给 health check 增加执行次数/失败次数指标
- [ ] 暴露 DB pool 指标
- [ ] 接入 pprof
- [ ] 更新配置示例

### 验收标准

- 可在本地看到指标输出
- 可区分接口类型与状态码
- 指标实现不引入高基数 label 风险
- 不开启时不影响现有行为

---

## ISSUE-02：引入 `ExecutorGateway` 并替换 service 层直接依赖

- Phase：Phase 0
- 优先级：P0
- 估时：2 ~ 3 天
- 依赖：ISSUE-00

### 范围

- `internal/service/mcp_service.go`
- `internal/service/tool_service.go`
- `internal/service/tool_invoke_service.go`
- `internal/mcpclient/manager.go`
- 新增：`internal/gateway/executor/`

### 交付物

- `ExecutorGateway` 接口
- `LocalExecutorGateway` 实现
- service 层依赖注入改造

### 开发任务

- [ ] 定义 `ExecutorGateway` 接口
- [ ] 用 `LocalExecutorGateway` 包装现有 Manager
- [ ] `MCPService` 改为调用 gateway
- [ ] `ToolService` 改为调用 gateway
- [ ] `ToolInvokeService` 改为调用 gateway
- [ ] 补接口 mock 与单元测试

### 验收标准

- service 层不再直接引用 `*mcpclient.Manager`
- 现有 connect/sync/invoke 行为不变
- 为未来 `RemoteExecutorGateway` 预留稳定接口

---

## ISSUE-03：抽象 `TokenBlacklistStore`

- Phase：Phase 0
- 优先级：P0
- 估时：1 天
- 依赖：ISSUE-00

### 范围

- `pkg/crypto/jwt.go`
- `pkg/crypto/blacklist.go`
- 可新增：`internal/store/tokenblacklist/` 或 `pkg/crypto/store.go`

### 交付物

- 黑名单接口抽象
- 内存实现
- JWTService 依赖接口而非具体类型

### 开发任务

- [ ] 定义 `TokenBlacklistStore` 接口
- [ ] 让内存黑名单实现该接口
- [ ] 修改 JWTService 构造函数
- [ ] 补单元测试，覆盖 add/contains/expired/并发场景

### 验收标准

- JWTService 不再依赖具体内存结构
- 现有登录、刷新、登出测试全部通过

---

## ISSUE-04：抽象 `RuntimeStore`

- Phase：Phase 0
- 优先级：P0
- 估时：2 天
- 依赖：ISSUE-02

### 范围

- `internal/service/mcp_service.go`
- `internal/mcpclient/health_checker.go`
- `internal/mcpclient/manager.go`
- 新增：`internal/store/runtime/`

### 交付物

- `RuntimeStore` 接口
- 内存实现
- 状态查询与健康检查可通过 store 协作

### 开发任务

- [ ] 定义 runtime 读写接口
- [ ] 实现内存版 RuntimeStore
- [ ] `Status()` 查询优先读 store
- [ ] 健康检查状态同步通过 store 进行
- [ ] 补单元测试和集成测试

### 验收标准

- 状态读取不再只能依赖本进程 `Manager.GetStatus`
- 健康检查状态同步路径更清晰
- 不改变现有 API 响应结构

---

## ISSUE-05：统一 `HistorySink / AuditSink` 抽象

- Phase：Phase 0
- 优先级：P1
- 估时：1.5 天
- 依赖：ISSUE-02

### 范围

- `internal/service/tool_invoke_service.go`
- `internal/service/audit_sink.go`
- 新增：`internal/sink/history/`、`internal/sink/audit/`

### 交付物

- 明确的 history sink 抽象
- 现有 audit sink 命名与实现收敛
- 默认同步实现不变

### 开发任务

- [ ] 定义 `HistorySink`
- [ ] 保留/整理 `AuditSink`
- [ ] Tool invoke 改为写 history sink
- [ ] 保持 DB 同步实现为默认路径
- [ ] 补测试

### 验收标准

- History/Audit 路径都通过 sink 抽象访问
- 现有功能无行为变化
- 为异步实现预留扩展点

---

## ISSUE-06：形成 Phase 0 基线与验证文档

- Phase：Phase 0
- 优先级：P1
- 估时：1 天
- 依赖：ISSUE-01、ISSUE-02、ISSUE-03、ISSUE-04

### 范围

- `docs/`
- `scripts/`（如需压测脚本）

### 交付物

- 基线采集说明
- 指标说明
- Phase 0 验证报告模板

### 开发任务

- [ ] 补压测或 smoke 脚本
- [ ] 补基线记录模板
- [ ] 记录 Phase 0 完成定义

### 验收标准

- 团队能按文档复现实验
- 能留存 Phase 0 前后对比数据

---

## ISSUE-10：支持 PostgreSQL 配置与数据库初始化

- Phase：Phase 1
- 优先级：P0
- 估时：2 天
- 依赖：ISSUE-00

### 范围

- `internal/config/config.go`
- `internal/database/database.go`
- `config.yaml`
- `deployments/`

### 交付物

- `database.driver = postgres` 可用
- PostgreSQL DSN 配置
- 初始化逻辑按 driver 分流

### 开发任务

- [ ] 扩展数据库配置结构
- [ ] 增加 PostgreSQL driver 初始化
- [ ] 保留 SQLite 兼容
- [ ] 更新默认配置与部署样例

### 验收标准

- SQLite 与 PostgreSQL 两种模式都能启动
- 数据库 health 检查正常

---

## ISSUE-11：统一 repository 错误归一化与数据库兼容层

- Phase：Phase 1
- 优先级：P0
- 估时：1.5 ~ 2 天
- 依赖：ISSUE-10

### 范围

- `internal/repository/common.go`
- `internal/repository/*.go`

### 交付物

- SQLite / PostgreSQL 通用唯一冲突识别
- 仓储层错误归一化规则

### 开发任务

- [ ] 识别依赖 SQLite 错误文本的逻辑
- [ ] 统一错误判断函数
- [ ] 为两种数据库补测试

### 验收标准

- 仓储层不再依赖 SQLite 特定错误字符串
- Create/Update/Upsert 行为在两种 DB 下都可预测

---

## ISSUE-12：补 PostgreSQL 集成测试与迁移验证

- Phase：Phase 1
- 优先级：P0
- 估时：2 天
- 依赖：ISSUE-10、ISSUE-11

### 范围

- `tests/integration/`
- `tests/e2e/`
- `scripts/`

### 交付物

- PostgreSQL 模式测试入口
- CRUD / sync-tools / invoke 基本覆盖
- 索引与迁移行为验证

### 开发任务

- [ ] 增加 PostgreSQL 测试环境说明
- [ ] 给关键链路增加 PostgreSQL 集成测试
- [ ] 验证迁移脚本与初始化管理员逻辑

### 验收标准

- PostgreSQL 模式下核心测试通过
- 迁移和初始化流程稳定

---

### 共享 Redis 基础能力建议

- 建议把 Redis client、序列化、命名空间、超时配置统一放在共享初始化层
- 避免 `ISSUE-13` 与 `ISSUE-14` 各自创建一套 Redis 依赖注入与错误处理逻辑

## ISSUE-13：实现 Redis 黑名单

- Phase：Phase 1
- 优先级：P0
- 估时：1.5 天
- 依赖：ISSUE-03

### 范围

- `pkg/crypto/jwt.go`
- 新增：`internal/store/tokenblacklist/redis.go`
- `internal/config/config.go`

### 交付物

- Redis 黑名单实现
- 配置与依赖注入
- 降级策略说明

### 开发任务

- [ ] 增加 Redis 配置结构
- [ ] 实现 Redis blacklist store
- [ ] 接入 JWTService
- [ ] 明确 `login` / `refresh` / `logout` / 鉴权中间件在 Redis 不可用时的系统行为
- [ ] 约定并记录默认安全策略，避免 silent fail-open
- [ ] 补一致性测试
- [ ] 补 Redis 异常场景测试

### 验收标准

- 多副本场景下 token 失效可共享
- Redis 异常时关键认证路径行为明确、可观测、不可 silent fail-open
- Redis 异常时有日志和指标

---

## ISSUE-14：实现 Redis RuntimeStore

- Phase：Phase 1
- 优先级：P0
- 估时：2 天
- 依赖：ISSUE-04

### 范围

- `internal/store/runtime/`
- `internal/service/mcp_service.go`
- `internal/mcpclient/health_checker.go`
- `internal/config/config.go`

### 交付物

- Redis RuntimeStore
- 本地执行 + Redis 运行态缓存的单体模式
- TTL 与序列化策略

### 开发任务

- [ ] 定义 Redis key 结构
- [ ] 实现 runtime 序列化/反序列化
- [ ] 接入状态查询路径
- [ ] 接入健康检查写回路径
- [ ] 设计 TTL、`updated_at` 和过期处理
- [ ] 明确本地连接真相与 Redis 缓存之间的读优先级与回退规则

### 验收标准

- 状态查询不再只依赖本地内存
- Redis 中可看到最新运行态缓存
- stale/miss 场景下读路径行为明确
- API 响应兼容当前结构

---

## ISSUE-15：审计导出改为分页/流式实现

- Phase：Phase 1
- 优先级：P1
- 估时：1.5 天
- 依赖：ISSUE-10

### 范围

- `internal/service/audit_service.go`
- `internal/repository/audit_log_repository.go`
- `internal/handler/audit_handler.go`

### 交付物

- 分页/游标式导出实现
- 大数据量导出时更低内存占用

### 开发任务

- [ ] 调整 repository 导出读取方式
- [ ] 视需要将 handler 改为 streaming response
- [ ] 增加大页场景测试

### 验收标准

- 导出不再一次性加载大量数据到内存
- 现有导出结果格式保持兼容

---

## ISSUE-16：实现可选 Async Sink

- Phase：Phase 1
- 优先级：P1
- 估时：2 天
- 依赖：ISSUE-05

### 范围

- `internal/sink/history/`
- `internal/sink/audit/`
- `internal/config/config.go`

### 交付物

- 可选异步 sink
- 队列长度、降级与日志策略
- 默认关闭开关

### 开发任务

- [ ] 实现内存队列版 async history sink
- [ ] 实现内存队列版 async audit sink
- [ ] 队列满时回退同步写
- [ ] 增加指标与日志
- [ ] 补测试

### 验收标准

- 异步开关关闭时，行为与当前一致
- 异步开关打开时，队列积压不会直接丢数据而无日志

---

## ISSUE-17：Phase 1 回归、文档与发布准备

- Phase：Phase 1
- 优先级：P1
- 估时：1 天
- 依赖：ISSUE-10 ~ ISSUE-16

### 范围

- `README.md`
- `docs/`
- `deployments/`
- `tests/`

### 交付物

- Phase 1 发布说明
- 本地开发说明
- 回退说明
- 验证记录

### 开发任务

- [ ] 更新 README 与 deployment 文档
- [ ] 更新配置示例
- [ ] 补 Phase 1 验证清单
- [ ] 记录回退策略

### 验收标准

- 新成员可按文档跑起 SQLite / PostgreSQL / Redis 模式
- 上线与回退步骤明确

---

## 4. 看板建议

建议在项目看板中按如下列组织：

- Backlog
- Ready
- In Progress
- Review
- Verify
- Done

建议标签：

- `phase-0`
- `phase-1`
- `infra`
- `observability`
- `gateway`
- `runtime`
- `postgres`
- `redis`
- `docs`
- `test`

---

## 5. 推荐首批并行开发组合

### 组合 A：先稳边界

- 开发 A：ISSUE-00
- 开发 B：ISSUE-01
- 开发 C：ISSUE-03

### 组合 B：抽象主链路

在组合 A 完成后：

- 开发 A：ISSUE-02
- 开发 B：ISSUE-04
- 开发 C：ISSUE-05

### 组合 C：基础设施升级

在 Phase 0 收口后：

- 开发 A：ISSUE-10
- 开发 B：ISSUE-11
- 开发 C：ISSUE-13
- `ISSUE-12` 在 `ISSUE-10` 与 `ISSUE-11` 完成后启动
- `ISSUE-14` 在 `ISSUE-04` 与 Redis 接入方案稳定后启动

### 组合 D：治理收尾

- 开发 A：ISSUE-15
- 开发 B：ISSUE-16
- 开发 C：ISSUE-17

---

## 6. 每个 Issue 建议统一模板

```md
## 背景

## 目标

## 范围

## 非目标

## 开发任务
- [ ]

## 验收标准
- [ ]

## 风险

## 依赖
```

---

## 7. 结论

如果要马上开工，建议按下面顺序直接建 issue：

1. ISSUE-00
2. ISSUE-01
3. ISSUE-03
4. ISSUE-02
5. ISSUE-04
6. ISSUE-05
7. ISSUE-06
8. ISSUE-10
9. ISSUE-11
10. ISSUE-12
11. ISSUE-13
12. ISSUE-14
13. ISSUE-15
14. ISSUE-16
15. ISSUE-17

这样能先把边界打稳，再做基础设施切换，整体风险最低。
