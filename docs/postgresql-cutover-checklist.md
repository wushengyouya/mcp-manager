# PostgreSQL 切换校验清单

## 切换前
- [ ] 已确认维护窗口
- [ ] 已备份 SQLite 文件
- [ ] PostgreSQL 连接信息已验证
- [ ] 应用 migration 已在 PostgreSQL 成功执行
- [ ] 默认 SQLite 回退配置已保留

## 数据校验
- [ ] `users` 行数一致
- [ ] `mcp_services` 行数一致
- [ ] `tools` 行数一致
- [ ] `request_histories` 行数一致
- [ ] `audit_logs` 行数一致
- [ ] active unique：`mcp_services(name) WHERE deleted_at IS NULL` 抽样通过
- [ ] active unique：`tools(mcp_service_id, name) WHERE deleted_at IS NULL` 抽样通过
- [ ] JSON 字段可反序列化抽样通过
- [ ] tag exact-match：`alpha` 不命中 `alphabet`
- [ ] soft delete + tool upsert 抽样通过

## 应用冒烟
- [ ] 默认管理员可登录
- [ ] service 创建成功
- [ ] service 查询成功
- [ ] service 列表成功
- [ ] sync-tools 成功
- [ ] history 写入成功
- [ ] audit 写入成功

## 回切准备
- [ ] SQLite 回退配置已复核
- [ ] SQLite 备份文件路径已复核
- [ ] 回切 smoke 步骤已复核

## 结论
- [ ] 准许解除冻结
- [ ] 若未勾选，执行回切
