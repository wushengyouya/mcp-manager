# MCP 服务管理平台

基于 Go 1.26、Gin、GORM、SQLite 与 `mcp-go` 的单机版 MCP 服务管理平台。

## 功能

- 本地账号密码登录与 JWT 鉴权
- MCP 服务管理，支持 `stdio`、`streamable_http`、`sse`
- 服务连接、断开、状态查询、健康检查
- 工具同步、工具调用、调用历史
- 审计日志导出、Swagger 入口、Docker 部署

## 快速开始

```bash
go mod tidy
go run ./cmd/server
```

默认管理员：

- 用户名：`root`
- 密码：`admin123456`

## 主要接口

- `POST /api/v1/auth/login`
- `GET /api/v1/services`
- `POST /api/v1/services/:id/connect`
- `POST /api/v1/services/:id/sync-tools`
- `POST /api/v1/tools/:id/invoke`
- `GET /api/v1/history`

## Docker

```bash
docker build -t mcp-manager:latest -f deployments/docker/Dockerfile .
docker run -p 8080:8080 -e MCP_JWT_SECRET=change-me mcp-manager:latest
```
