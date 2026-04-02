# mcp-manager 架构升级实施路线图

> 基于 `docs/architecture-upgrade-plan.md` 的渐进式落地版。

## 1. 目标

把当前 `mcp-manager` 从“状态型单体”逐步演进为“可分离单体 -> 双角色部署架构”，同时保持：

- 外部 REST API 基本兼容
- 单机开发体验不被破坏
- 每个阶段都可单独发布、验证、回退

## 2. 实施原则

- 一次只做一类变化，避免基础设施升级和部署架构重构同时发生
- 每个阶段先补测试与观测，再改实现
- 优先引入抽象层，不先大面积搬代码
- `all` 模式长期保留，用于开发、测试、回退

---

## 3. 里程碑总览

| 阶段 | 目标 | 产出 | 是否可独立上线 |
| --- | --- | --- | --- |
| Phase 0 | 补观测与抽象边界 | 指标、pprof、执行抽象接口 | 是 |
| Phase 1 | 单体内基础设施升级 | PostgreSQL、Redis 黑名单、运行态存储抽象 | 是 |
| Phase 2 | 双角色部署 | control-plane / executor + 内部 RPC | 是 |
| Phase 3 | 执行面增强 | 限流、异步 sink、异步调用接口 | 是 |
| Phase 4 | 热点拆分 | transport 分池、读链路拆分 | 触发式 |

---

## 4. 推荐按周计划

## 第 1 周：观测基线与现状锁定

### 目标

在不改业务行为的前提下，补齐可观测性，形成后续比较基线。

### 任务

- 增加 HTTP 指标
- 增加工具调用耗时指标
- 增加健康检查指标
- 增加数据库连接池指标
- 开启 `pprof`
- 补一份压测脚本或压测说明
- 记录当前基线结果

### 验收

- 可以看到 p50 / p95 / p99
- 可以区分读接口、写接口、工具调用耗时
- 可以看到健康检查失败率

### 风险

- 指标埋点过细导致噪音过多
- 只加指标但没有统一命名规范

---

## 第 2 周：执行边界抽象

### 目标

把 service 层从直接依赖 `mcpclient.Manager` 改成依赖抽象接口。

### 任务

- 引入 `ExecutorGateway`
- 引入 `RuntimeStore`
- 引入 `TokenBlacklistStore`
- 把 `MCPService`、`ToolService`、`ToolInvokeService` 改为依赖抽象
- 保持默认实现仍然走本地执行
- 为抽象层补单元测试

### 验收

- 当前单体行为不变
- service 层不再直接依赖具体 `Manager`
- 新增接口有默认本地实现

### 风险

- 抽象层设计过重，影响开发效率
- 过早引入过多未使用字段

---

## 第 3 周：数据库升级准备

### 目标

为 SQLite -> PostgreSQL 迁移做兼容性清理。

### 任务

- 识别 SQLite 特有逻辑
- 整理索引策略
- 统一仓储错误归一化
- 修复唯一索引冲突识别逻辑
- 补 PostgreSQL 兼容测试
- 梳理迁移脚本与初始化路径

### 验收

- 代码中不再依赖 SQLite 错误字符串作为业务判断依据
- 仓储层可对 PostgreSQL 错误统一归一化
- 实体与索引方案完成评审

### 风险

- GORM 行为在两种数据库上不完全一致
- JSON 字段、软删除唯一索引语义不同

---

## 第 4 周：单体内数据库升级

### 目标

完成单体模式下 PostgreSQL 运行。

### 任务

- 支持 `database.driver = postgres`
- 增加 PostgreSQL 配置与连接初始化
- 迁移初始化管理员、CRUD、工具同步、历史/审计路径
- 回归现有测试
- 更新部署与本地开发说明

### 验收

- 单体模式下 PostgreSQL 稳定运行
- 现有集成测试通过
- 基础性能优于 SQLite 阶段

### 风险

- 迁移脚本与现有数据库初始化顺序冲突
- 本地开发环境门槛变高

---

## 第 5 周：Redis 黑名单与运行态存储抽象

### 目标

完成跨副本一致性的第一步。

### 任务

- 为 JWT 黑名单增加 Redis 实现
- 为运行态存储增加 Redis 实现
- 支持“本地执行 + Redis 缓存/协调”的单体模式
- 定义 Redis 不可用时的降级策略
- 补多副本一致性测试

### 验收

- token 失效能跨副本生效
- 状态查询不再强绑定本进程内存
- Redis 不可用时系统行为明确

### 风险

- 黑名单缓存策略处理不好会导致短时间不一致
- 运行态缓存与真实连接状态漂移

---

## 第 6 周：导出与写入治理

### 目标

降低查询/导出路径对主业务线程的影响。

### 任务

- 审计导出改为流式/分页游标式实现
- History/Audit sink 统一抽象
- 同步 sink 保持默认
- 异步 sink 做可选实现与灰度开关

### 验收

- 导出不再一次性读全量数据到内存
- sink 切换不影响现有业务接口
- 异步实现可被开关控制

### 风险

- 过早启用异步 sink 可能带来排查复杂度
- 导出接口响应语义变化需要前端确认

---

## 第 7-8 周：双角色部署能力

### 目标

引入 control-plane / executor，但仍保留 `all` 模式。

### 任务

- 增加 `app.role = all | control-plane | executor`
- control-plane 引入 `RemoteExecutorGateway`
- executor 引入本地 `Manager`
- 增加内部 RPC
- 健康检查仅在 executor 生效
- 增加 executor 注册 / 心跳 / 负载上报

### 验收

- 双角色模式可跑通 connect / sync-tools / invoke / status
- `all` 模式继续可用
- 本地开发与 CI 不被破坏

### 风险

- 启动流程复杂度明显提升
- 同一代码库三种角色行为容易分叉

---

## 第 9 周：sticky owner / epoch

### 目标

把真正有状态的 transport 安全迁移到多 executor 调度。

### 任务

- 增加 owner lease
- 增加 epoch / fencing token
- sticky / non-sticky 分类路由
- executor 宕机接管测试
- 旧 owner 拒写测试

### 验收

- sticky 服务不会双 owner 并发执行
- executor 宕机后可接管
- epoch 不匹配时拒绝执行

### 风险

- 续租与心跳节奏不合理导致误切换
- Redis 抖动带来误判

---

## 第 10 周及以后：执行面增强

### 目标

在稳定双角色之后再做增强能力。

### 任务

- 每 executor / 每 service / 每 user 限流
- 长耗时任务查询与取消
- 异步调用接口
- 按 transport 分池
- 历史/审计读链路拆分

### 验收

- 高并发退化可控
- 长耗时任务可管理
- 热点 transport 可独立扩容

---

## 5. 发布策略建议

### 5.1 发布顺序

- 先发 Phase 0
- 再发 Phase 1
- 再发 Phase 2
- Phase 3 与 Phase 4 视业务压力触发

### 5.2 回退策略

- 所有阶段都保留 feature flag / 配置开关
- `all` 模式作为重要回退手段
- Redis / PostgreSQL 切换尽量通过配置完成
- 异步 sink 默认关闭，灰度后再放量

---

## 6. 每阶段完成定义

一个阶段完成，至少应满足：

- 功能回归测试通过
- 集成测试通过
- 文档更新完成
- 部署脚本或配置样例更新完成
- 至少一条可量化指标证明阶段目标达成

---

## 7. 建议同步维护的配套文档

建议在实施过程中同步维护：

- `docs/architecture-upgrade-phase0-phase1-issues.md`
- `docs/architecture-upgrade-plan.md`
- `docs/architecture-upgrade-roadmap.md`
- `docs/architecture-upgrade-phase0-phase1-checklist.md`
- `docs/deployment.md`
- `README.md`

---

## 8. 最终建议

对当前项目来说，最重要的不是“尽快拆成分布式”，而是：

1. 先把执行边界抽出来
2. 先把状态外置能力做好
3. 保持每一步都能上线、验证、回退

这样升级成本最低，失败面也最小。
