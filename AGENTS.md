# Repository Guidelines

## 项目结构与模块组织
入口程序位于 `cmd/server`。核心业务按分层放在 `internal`：`config` 负责配置加载，`database` 负责数据库初始化与迁移，`handler`、`service`、`repository` 分别对应接口层、业务层、持久层，`mcpclient` 与 `task` 处理 MCP 连接和后台任务，`domain/entity` 定义实体。通用能力放在 `pkg`，如 `crypto`、`logger`、`email`、`response`。Swagger 产物在 `api/docs`，迁移与种子数据在 `db/`，Docker 相关文件在 `deployments/docker`，测试代码集中在 `tests/integration`、`tests/e2e`、`tests/mocks`、`tests/testutil`。

## 构建、测试与开发命令
常用命令以 `Makefile` 为准：

- `go run ./cmd/server`：本地启动服务。
- `make build`：编译二进制到 `bin/mcp-manager`。
- `make test`：运行覆盖率测试、集成测试和竞态测试。
- `make test-e2e`：执行 `tests/integration` 与 `tests/e2e`。
- `make test-race`：执行 `go test -race ./...`。
- `make vet`：运行 `go vet ./...`。
- `make swagger`：根据 `cmd/server/main.go` 注解刷新 `api/docs`。

## 编码风格与命名约定
提交前必须通过 `gofmt`；CI 会显式检查未格式化的 `.go` 文件，并执行 `go vet`。遵循 Go 默认风格，使用制表符缩进，包名保持小写短名。文件名按职责命名，例如 `user_service.go`、`mcp_service_repository.go`、`handler_test.go`。新增导出符号时补充简洁中文注释，错误信息和日志保持可定位、可检索。

## 测试指南
单元测试优先与实现文件同目录放置，文件名使用 `*_test.go`；跨模块流程测试放到 `tests/integration`，真实 HTTP 场景放到 `tests/e2e`。修改 `service`、`repository`、`handler` 或 `router` 时，应补对应层测试。提交前至少执行 `make test`；定向排查时可用 `go test ./internal/service -run TestAuthService`。

## 提交与 Pull Request 规范
提交信息遵循 Conventional Commits，仓库中已使用 `feat:`、`refactor:`、`docs:`、`test:`、`style:`、`ci:`、`chore:`。主题行保持简短，优先使用中文，例如 `feat: 增加服务健康检查告警`。提交前先自查潜在 bug，再发起 PR。PR 描述应说明变更范围、配置或迁移影响、执行过的测试命令；若修改接口或 Swagger，请一并更新 `api/docs` 并附示例请求。

## 配置与安全
应用默认读取仓库根目录 `config.yaml`，并允许 `MCP_*` 环境变量覆盖，例如 `MCP_JWT_SECRET`。开发环境可使用默认 SQLite 数据库 `data/mcp_manager.db`，但不要在提交中带入真实密钥或无关运行时数据。
