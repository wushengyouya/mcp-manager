# PostgreSQL 切换演练样例

> 说明：本文件记录一次本地演练样例。若正式环境执行，请复制 `docs/postgresql-drill-template.md` 另存并填写真实时间、人员与结果。

- 演练日期：2026-04-07
- 演练环境：本地单体 / SQLite 默认配置 + PostgreSQL 验证环境
- 执行人：Codex
- Reviewer：待补充
- 目标版本：v1.1 PostgreSQL capability
- PostgreSQL DSN（脱敏）：`postgres://postgres:***@127.0.0.1:55432/mcp_manager?sslmode=disable`

## 演练步骤摘要
1. 保留 SQLite 默认配置，确认 `database.driver=sqlite` 可回退。
2. 使用 PostgreSQL DSN 运行 repository/database/app matrix。
3. 执行登录、service CRUD、列表、tag exact-match、tool upsert 验证。
4. 若任一步失败，则不解除冻结并切回 SQLite。

## 样例记录表
| 步骤 | 命令/动作 | 结果 | 备注 |
| --- | --- | --- | --- |
| 1 | 备份 SQLite 文件 | 待正式执行 | 本地代码层面保留默认 sqlite 回退配置 |
| 2 | PostgreSQL migration 验证 | 待正式执行 | 依赖真实 DSN / 环境 |
| 3 | 行数与 JSON 校验 | 待正式执行 | 需真实迁移数据 |
| 4 | 登录 / service smoke | 已由集成测试覆盖 | 见 `tests/integration/app_test.go` |
| 5 | tag exact-match / tool upsert | 已由 repository matrix 覆盖 | 见 `internal/repository/*_test.go` |
| 6 | 回切 smoke | 待正式执行 | 正式窗口失败时执行 |

## 结论
- 代码层能力、默认 SQLite 回退路径与文档已就位。
- 正式切换前仍需在真实 PostgreSQL 环境补齐一次完整离线演练记录。
