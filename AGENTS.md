# Repository Guidelines

## 项目结构与模块组织
入口位于 `cmd/server`。核心代码在 `internal/`：`config` 负责配置加载，`database` 负责数据库初始化与迁移，`handler` / `service` / `repository` 分别对应接口层、业务层、持久层，`mcpclient` 与 `rpc` 负责运行时执行与双角色通信，`task` 放后台任务。领域模型在 `internal/domain/entity`。通用能力在 `pkg/`。Swagger 产物在 `api/docs`，迁移与种子数据在 `db/`，Docker 文件在 `deployments/docker`，测试在 `tests/integration`、`tests/e2e`、`tests/pgtest`、`tests/testutil`。

## 构建、测试与开发命令
- `go run ./cmd/server`：本地启动服务。
- `make build`：编译生成 `bin/mcp-manager`。
- `make test`：执行覆盖率、集成/E2E、竞态测试。
- `make test-e2e`：仅运行 `tests/integration` 与 `tests/e2e`。
- `make test-race`：执行 `go test -race ./...`。
- `make vet`：执行 `go vet ./...`。
- `make swagger`：基于 `cmd/server/main.go` 刷新 Swagger 文档。

## 编码风格与命名约定
提交前必须执行 `gofmt`；CI 会检查格式与 `go vet`。遵循标准 Go 风格：制表符缩进、包名小写、文件名按职责命名，如 `user_service.go`、`handler_test.go`。优先复用现有工具和模式，避免引入无必要的新抽象。

## 测试指南
单元测试优先与实现文件同目录放置，命名使用 `*_test.go`。跨模块流程放入 `tests/integration`，真实 HTTP 场景放入 `tests/e2e`。修改 handler、service、repository、router 时，应同步补齐或更新对应测试。快速验证可用 `go test ./...`，提交前建议执行 `make test`。

## 提交与 PR 规范
提交信息使用 Conventional Commits，例如：`feat: 增加服务健康检查告警`、`fix: 修复工具调用超时处理`、`docs: 更新部署说明`。主题行简洁明确。PR 需说明变更范围、配置或迁移影响、验证命令；若接口或 Swagger 变更，请同步更新 `api/docs`，必要时附示例请求或截图。

## 安全与配置提示
默认配置文件为 `config.yaml`，可通过 `MCP_*` 环境变量覆盖，例如 `MCP_JWT_SECRET`。默认数据库可能因配置不同使用 PostgreSQL 或 SQLite；不要提交真实密钥、测试凭据或无关运行时数据。
