# mcp-manager Phase 0 / Phase 1 改造检查清单

> 用于把 `docs/architecture-upgrade-plan.md` 中的“渐进式升级”落成可执行、可验收、可回退的阶段清单。

## 1. 使用原则

- 一次只推进一个 issue，不把启动装配、数据库升级、Redis 外置混在同一个提交里。
- 每一阶段都要先验证“默认单体行为不变”，再验证新增能力。
- 优先补抽象和观测，再替换底层基础设施。
- 所有改动都要保留 `app.role=all` 的本地开发路径。

## 2. Phase 0：观测与边界抽象

Phase 0 的目标不是加新能力，而是让当前单体变成：

- 可观测
- 可替换依赖
- 可拆启动装配
- 可为 PostgreSQL / Redis / 双角色部署预留边界

### 2.1 ISSUE-00：启动装配拆分与 `app.role`

- [ ] 从 `cmd/server/main.go` 中拆出数据库装配
- [ ] 拆出 store / gateway / service 装配
- [ ] 拆出 router / HTTP server 装配
- [ ] 新增 `app.role = all | control-plane | executor`
- [ ] 默认值设为 `all`
- [ ] 默认启动路径保持与当前单体一致

验收：

- [ ] `go run ./cmd/server` 可直接启动
- [ ] 默认行为与当前一致
- [ ] main 文件复杂度明显下降

### 2.2 ISSUE-01：metrics 与 pprof

- [ ] 设计 metrics 命名规范
- [ ] 只使用低基数 label（`route`、`method`、`status_code`、`result`、`operation`）
- [ ] 禁止 `user_id`、`request_id`、原始 `service_id`
- [ ] 新增 HTTP 请求总数 / 耗时 / 状态码指标
- [ ] 新增 tool invoke 成功失败与耗时指标
- [ ] 新增健康检查次数 / 失败次数指标
- [ ] 新增 DB pool 指标
- [ ] 接入 pprof
- [ ] 默认关闭 metrics / pprof

验收：

- [ ] 开启后可看到 `/metrics`
- [ ] pprof 仅显式开启时暴露
- [ ] 不开启时不影响当前行为

### 2.3 ISSUE-02：ExecutorGateway 抽象

- [ ] 定义 `ExecutorGateway` 接口
- [ ] 提供 `LocalExecutorGateway`
- [ ] `MCPService` 通过 gateway 调用连接管理
- [ ] `ToolService` 通过 gateway 获取工具能力
- [ ] `ToolInvokeService` 通过 gateway 发起调用
- [ ] 为接口补 mock 与单测

验收：

- [ ] service 层不再直接依赖 `*mcpclient.Manager`
- [ ] 默认仍走本地执行实现

### 2.4 ISSUE-03：TokenBlacklistStore 抽象

- [ ] 抽出 `TokenBlacklistStore` 接口
- [ ] 保留当前内存实现作为默认实现
- [ ] 统一登出 / refresh 作废 token 写入入口
- [ ] 为过期、命中、并发场景补测试

验收：

- [ ] 当前单机模式下行为不变
- [ ] 后续可无缝替换为 Redis 实现

### 2.5 ISSUE-04：RuntimeStore 抽象

- [ ] 抽出运行态查询 / 写回接口
- [ ] 健康检查改为通过 RuntimeStore 回写
- [ ] 状态查询改为优先读 store
- [ ] 定义 stale / miss 的回退策略
- [ ] 保留本地内存实现

验收：

- [ ] 当前状态查询结果不回退
- [ ] 运行态更新路径清晰、单一

### 2.6 ISSUE-05：History / Audit Sink 抽象

- [ ] 抽出 `HistorySink`
- [ ] 抽出 `AuditSink`
- [ ] 默认仍同步写库
- [ ] 明确同步 / 异步切换边界

验收：

- [ ] tool invoke 主链路不再直接耦合具体持久化实现

### 2.7 ISSUE-06：Phase 0 基线与文档

- [ ] 记录 metrics / pprof 使用方式
- [ ] 增加 smoke / 压测说明
- [ ] 形成 Phase 0 验证模板
- [ ] 补齐 README / deployment 中需要的配置说明

验收：

- [ ] 团队成员可以按文档复现实验
- [ ] 后续阶段有稳定基线可对比

## 3. Phase 1：基础设施升级

Phase 1 的目标是在**不拆部署角色**的前提下完成：

- SQLite -> PostgreSQL
- 进程内黑名单 -> Redis
- 进程内运行态缓存 -> Redis 协调 / 缓存
- 历史 / 审计写入治理

### 3.1 ISSUE-10：PostgreSQL 初始化支持

- [ ] 支持 `database.driver=postgres`
- [ ] 增加 PostgreSQL DSN / 连接池配置
- [ ] 初始化逻辑兼容 SQLite 与 PostgreSQL
- [ ] 默认仍保留 SQLite

验收：

- [ ] SQLite / PostgreSQL 两种模式都能启动
- [ ] 基础 CRUD 正常

### 3.2 ISSUE-11：Repository 错误归一化

- [ ] 清理 SQLite 特有错误字符串判断
- [ ] 统一唯一冲突 / not found / constraint 错误识别
- [ ] 为双数据库行为补测试

验收：

- [ ] repository 层不再依赖 SQLite 文本错误
- [ ] PostgreSQL 下业务语义不漂移

### 3.3 ISSUE-12：PostgreSQL 集成验证

- [ ] 增加 PostgreSQL 集成测试入口
- [ ] 验证 migration
- [ ] 验证初始化管理员
- [ ] 验证服务 CRUD / sync-tools / invoke / history

验收：

- [ ] PostgreSQL 模式通过集成验证

### 3.4 ISSUE-13：Redis 黑名单实现

- [ ] 增加 Redis blacklist store
- [ ] 定义 key 结构、TTL、序列化方式
- [ ] 定义 Redis 不可用时的安全策略
- [ ] logout / refresh 失败时有明确返回与日志

验收：

- [ ] 多副本下 token 失效可共享
- [ ] 不出现 silent fail-open

### 3.5 ISSUE-14：Redis RuntimeStore 实现

- [ ] 增加 Redis RuntimeStore
- [ ] 定义写回、TTL、读回退规则
- [ ] 保持“本地连接真相优先，Redis 做协调缓存”
- [ ] 增加 stale / miss 场景测试

验收：

- [ ] 状态查询支持跨副本
- [ ] 不把 Redis 误当成连接存活的一手真相

### 3.6 ISSUE-15：审计导出治理

- [ ] 导出接口改为分页 / 流式
- [ ] 避免一次性全量读入内存
- [ ] 保持外部接口契约基本稳定

验收：

- [ ] 大数据量导出不显著放大内存占用

### 3.7 ISSUE-16：Async Sink 可选实现

- [ ] 提供异步 sink 开关
- [ ] 队列满时回退同步写
- [ ] 增加日志 / 指标
- [ ] 不改变默认同步行为

验收：

- [ ] 开关关闭时行为与当前一致
- [ ] 开关开启时无静默丢数

### 3.8 ISSUE-17：Phase 1 回归与发布准备

- [ ] 更新 README / deployment / config 示例
- [ ] 增加 SQLite / PostgreSQL / Redis 模式说明
- [ ] 记录上线 / 回退步骤
- [ ] 记录最小 smoke 命令

验收：

- [ ] 新成员可按文档跑起全部模式
- [ ] 发布与回退流程清晰

## 4. 阶段切换守门条件

### Phase 0 -> Phase 1

- [ ] 启动装配已拆分
- [ ] metrics / pprof 已接入并默认关闭
- [ ] service 层已通过抽象依赖执行面
- [ ] 黑名单、运行态、sink 已完成接口化
- [ ] Phase 0 基线文档齐备

### Phase 1 结束

- [ ] SQLite / PostgreSQL 双模式可运行
- [ ] Redis 黑名单与 RuntimeStore 可选启用
- [ ] 默认单体模式仍可作为回退路径
- [ ] README / deployment / config 示例已更新

## 5. 推荐验证命令

```bash
make build
make vet
go test ./...
go test ./tests/integration ./tests/e2e
go test -race ./...
```

如果是纯文档或脚本阶段，至少补：

```bash
make omx-doctor
make omx-report
```
