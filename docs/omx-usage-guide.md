# MCP Manager 的 OMX 使用建议

这个仓库是典型的 Go 后端项目，主链路基本是：

`cmd/server -> router -> handler -> service -> repository -> database`

再叠加：

- `internal/mcpclient` 负责 MCP 服务连接、健康检查、工具同步
- `internal/middleware` 负责 JWT、权限等横切逻辑
- `tests/integration` / `tests/e2e` 负责端到端验证

所以最适合的 OMX 用法不是“泛泛地让它写代码”，而是把任务说成一个带边界、带验证要求的后端工作单元。

## 推荐入口

如果你在终端里直接敲 `omx` 偶尔失效，优先用项目包装脚本：

```bash
./scripts/omx.sh --help
./scripts/omx.sh doctor
./scripts/omx.sh --madmax --high
```

这个脚本会自动：

- 补 `~/.npm-global/bin` 到 `PATH`
- 固定 `CODEX_HOME=./.codex`
- 切到仓库根目录启动

## 最常用的提法

### 1. 新功能先规划

适用于新增接口、MCP 能力扩展、权限改动。

```text
$plan 为服务管理接口增加批量启停能力，先梳理 router/handler/service/repository 影响面，再给出测试点
```

### 2. 后端缺陷直接持续修

适用于 bug 必须修到验证通过。

```text
ralph 修复 MCP 服务断开后状态未回写的问题，沿着 router -> handler -> service -> repository 排查，补测试并运行 go test ./...
```

### 3. 认证和权限改动先审查

适用于登录、JWT、中间件、用户权限。

```text
code review 最近认证链路改动，重点看 auth handler、middleware、token 刷新、权限绕过和缺失测试
```

```text
security review 这次用户与 JWT 相关改动，检查越权、弱口令、token 生命周期和接口暴露面
```

### 4. 大改动先分析边界

适用于不确定从哪改、担心牵一发动全身。

```text
$architect 分析 MCP 服务连接、健康检查、工具同步这三段的边界和状态流，指出最脆弱的地方
```

### 5. 重构时显式说明“先锁行为”

这个仓库已经有不少单测、集成测试和 e2e 测试，重构要先锁行为。

```text
cleanup 重构 tool invoke 相关逻辑，先补回归测试锁住行为，再做小步清理，最后跑 go test ./...
```

### 6. 多模块工作再用 team

只有在改动明显跨模块、适合并行时才值得上团队模式。

```text
$team 3:executor 并行整理 handler/service/repository 的重复错误处理，最终统一验证 go test ./...
```

## 这个仓库里最值得你显式说出来的约束

你对 OMX 说任务时，建议把这些要求一起带上：

- `保持现有接口契约`
- `不要新增依赖`
- `优先复用现有 service/repository 模式`
- `补单测或集成测试`
- `最后跑 go test ./...`
- `涉及 HTTP 行为时补 tests/integration 或 tests/e2e`

例如：

```text
ralph 为服务列表增加标签过滤，保持现有接口契约，不要新增依赖，补 handler/service/repository 测试，最后跑 go test ./...
```

## 推荐验证语句

这个项目常见验证命令：

```text
最后运行 go test ./...
最后运行 go test ./tests/integration ./tests/e2e
最后运行 go vet ./...
```

如果是纯文档、脚本或 OMX 配置工作，可以改成：

```text
最后验证脚本可执行并输出报告
```

## 什么时候优先用哪种模式

- `plan`
  需求还没收敛，或者你知道改动会跨 `handler/service/repository`
- `ralph`
  你要的是“别停，修完并验证”
- `code review`
  你已经有改动，想先抓隐藏 bug、行为回归和漏测
- `security review`
  涉及认证、权限、JWT、服务调用边界
- `cleanup` / `refactor`
  你明确要做清理、去重复、降复杂度
- `team`
  任务足够大，且可以拆成互不冲突的子块并行

## 不推荐的说法

下面这些说法太空，会让结果变差：

- `帮我看看这个项目`
- `优化一下代码`
- `把这个功能做好`

更好的说法是直接点出：

- 功能目标
- 影响模块
- 不能破坏的约束
- 需要的验证方式

## 查看当前 OMX 使用情况

仓库里已经补了一个基于当前版本日志格式的报告脚本：

```bash
./scripts/omx_usage_report.sh
```

它会读取：

- `.omx/metrics.json`
- `.omx/logs/session-history.jsonl`
- `.omx/logs/turns-*.jsonl`
- `.codex/config.toml`

用来判断：

- 近期用了哪些 workflow
- 会话有多频繁
- 当前 shell 是否能直接调用 `omx`
- 这个仓库更适合你接下来怎么提需求
