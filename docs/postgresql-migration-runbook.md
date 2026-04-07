# PostgreSQL 离线迁移 Runbook

## 目标
在保持单体部署形态不变的前提下，将默认部署切换到 PostgreSQL，并保留 SQLite 显式回退能力。

## 范围边界
- 只支持冻结写入后的单窗口切换。
- 不支持在线迁移、双写、灰度切流、Redis、dual-role、`/ready`、RPC。
- Docker/Compose 与生产环境默认 PostgreSQL；SQLite 仅作为显式回退路径保留。

## 前置条件
1. 已备份 SQLite 文件，例如 `data/mcp_manager.db`。
2. PostgreSQL 目标库可连接，且 `postgres` 容器已健康就绪。
3. 维护窗口已确认，切换期间不会接收新的写请求。
4. 已准备校验清单：见 `docs/postgresql-cutover-checklist.md`。
5. 默认部署路径已经指向 PostgreSQL，SQLite 回退环境变量已预置。

## 切换前检查
1. 记录当前版本、分支、提交哈希、操作者、窗口开始时间。
2. 执行 SQLite 主路径验证：
   - `go test ./internal/database ./internal/repository ./internal/bootstrap ./tests/integration`
   - `go build ./...`
   - 这一步用于确认 SQLite 显式回退路径仍然可用，不代表当前默认路径是 SQLite。
3. 预迁移 PostgreSQL schema：
   - 设置 `database.driver=postgres`
   - 设置 `database.dsn=<postgres-dsn>`
   - 启动应用一次，确认 `Init + Migrate + Health` 成功
4. 确认 PostgreSQL 已存在以下关键索引：
   - `idx_mcp_services_name_active`
   - `idx_service_tool_active`
5. 记录 Docker/Compose 默认路径已是 PostgreSQL，SQLite 仅用于回切。

## 迁移步骤
1. **冻结写入**
   - 停止对外写请求进入应用。
   - 若有上游网关或发布器，设置维护模式。
2. **备份 SQLite**
   - 复制 `data/mcp_manager.db` 到带时间戳的备份文件。
3. **导出 SQLite 数据**
   - 依次导出：`users`、`mcp_services`、`tools`、`request_histories`、`audit_logs`。
   - JSON 字段需保持为合法 JSON 文本。
4. **导入 PostgreSQL**
   - 按表顺序导入数据。
   - 对软删除数据保留 `deleted_at` 原值。
5. **执行校验**
   - 按 `docs/postgresql-cutover-checklist.md` 执行行数、唯一约束、JSON 抽样、tag exact-match、tool upsert 校验。
6. **PostgreSQL 冒烟**
   - 登录
   - service 创建 / 查询 / 列表
   - tag exact-match 过滤
   - sync-tools
   - history / audit 写入
7. **解除冻结**
   - 仅在全部校验与冒烟通过后解除维护模式。

## 回切步骤
1. 任一步失败，保持冻结，不开放新写入。
2. 将配置切回：
   - `database.driver=sqlite`
   - `database.dsn=data/mcp_manager.db` 或对应备份文件
   - 若使用容器部署，可通过 `MCP_DATABASE_DRIVER=sqlite` / `MCP_DATABASE_DSN=...` 显式覆盖
   - 若要在容器内持久化 SQLite，请额外挂载 `/app/data` 或其他持久化目录
3. 重启单体应用。
4. 执行最小 SQLite smoke：
   - 登录
   - service 创建 / 查询 / 列表
   - tag exact-match
   - sync-tools
5. 记录失败现场，不做 PostgreSQL -> SQLite 增量回灌。

## 结果记录
- 窗口开始/结束时间
- 执行人 / Reviewer
- 校验结果
- 冒烟结果
- 是否回切
- 问题与结论
