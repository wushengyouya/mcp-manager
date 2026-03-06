# 部署说明

## 本地运行

```bash
go run ./cmd/server
```

## Docker 运行

```bash
docker compose -f deployments/docker/docker-compose.yml up --build
```

## 环境变量

- `MCP_JWT_SECRET`：JWT 签名密钥
- `MCP_SERVER_PORT`：服务端口
- `MCP_DATABASE_DSN`：SQLite 文件路径
