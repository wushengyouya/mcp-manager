# PostgreSQL 切换演练模板

- 演练日期：
- 演练环境：
- 执行人：
- Reviewer：
- 目标版本：
- PostgreSQL DSN（脱敏）：
- 默认数据库：PostgreSQL
- SQLite 回退方式：

## 前置检查
- SQLite 备份：
- PostgreSQL migration：
- 维护窗口：
- Docker/Compose 默认路径已切到 PostgreSQL：

## 执行记录
| 步骤 | 命令/动作 | 结果 | 备注 |
| --- | --- | --- | --- |
| 1 | 冻结写入 |  |  |
| 2 | 备份 SQLite |  |  |
| 3 | 导出 SQLite |  |  |
| 4 | 导入 PostgreSQL |  |  |
| 5 | 行数校验 |  |  |
| 6 | exact-match 校验 |  |  |
| 7 | tool upsert 校验 |  |  |
| 8 | 登录与 service smoke |  |  |
| 9 | 解除冻结 / 回切 |  |  |
| 10 | SQLite 显式回退 smoke |  |  |

## 问题记录
- 

## 结论
- 演练结论：
- 是否可用于正式窗口：
- 后续动作：
