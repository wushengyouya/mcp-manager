# curl 接口测试文档

本文档记录了 2026-03-06 在本地通过 `curl` 对项目全部 HTTP 接口的实测过程。主服务使用 `http://127.0.0.1:18080`，MCP Mock 服务使用 `http://127.0.0.1:18081/mcp`。

## 1. 测试环境

- Go 版本：`go version go1.26.0 linux/amd64`
- 主服务地址：`http://127.0.0.1:18080`
- MCP Mock 地址：`http://127.0.0.1:18081/mcp`
- 默认管理员：`root / admin123456`
- 本次实测动态数据：
  - `ROOT_ID=d71ec889-73b1-4030-9243-d4e6e89abe2d`
  - `OPERATOR_ID=dafe99bf-1fb7-48d7-a7b7-91ba15d8e648`
  - `SERVICE_ID=effee9ff-0c08-4bdc-91a2-01fb9d80694a`
  - `TOOL_ID=8e705b9e-e9c3-4d38-bc32-4b7a8478eadc`
  - `HISTORY_ID=38dd1114-4a64-4188-b0d1-02b8e5a5157a`

## 2. 启动方式

启动本地 MCP Mock 服务：

```bash
go run ./scripts/mock_mcp_server
```

启动主服务：

```bash
go run ./cmd/server
```

本次测试为了避免污染现有 SQLite 数据，实际使用了临时 `config.yaml` 和临时数据库文件，并将主服务启动在 `18080` 端口。

## 3. 变量准备

```bash
BASE_URL=http://127.0.0.1:18080
MCP_SERVER_URL=http://127.0.0.1:18081/mcp

ROOT_ACCESS_TOKEN='<登录后获取>'
ROOT_REFRESH_TOKEN='<登录后获取>'
OP_ACCESS_TOKEN='<operator 登录后获取>'
OP_REFRESH_TOKEN='<operator 登录后获取>'

ROOT_ID=d71ec889-73b1-4030-9243-d4e6e89abe2d
OPERATOR_ID=dafe99bf-1fb7-48d7-a7b7-91ba15d8e648
SERVICE_ID=effee9ff-0c08-4bdc-91a2-01fb9d80694a
TOOL_ID=8e705b9e-e9c3-4d38-bc32-4b7a8478eadc
HISTORY_ID=38dd1114-4a64-4188-b0d1-02b8e5a5157a
```

## 4. 覆盖结果

| 接口 | 方法 | HTTP 状态码 |
| --- | --- | --- |
| `/health` | `GET` | `200` |
| `/swagger/index.html` | `GET` | `200` |
| `/api/v1/auth/login` | `POST` | `200` |
| `/api/v1/auth/refresh` | `POST` | `200` |
| `/api/v1/auth/logout` | `POST` | `200` |
| `/api/v1/users` | `GET` | `200` |
| `/api/v1/users` | `POST` | `201` |
| `/api/v1/users/{id}` | `PUT` | `200` |
| `/api/v1/users/{id}` | `DELETE` | `200` |
| `/api/v1/users/{id}/password` | `PUT` | `200` |
| `/api/v1/services` | `GET` | `200` |
| `/api/v1/services` | `POST` | `201` |
| `/api/v1/services/{id}` | `GET` | `200` |
| `/api/v1/services/{id}` | `PUT` | `200` |
| `/api/v1/services/{id}` | `DELETE` | `200` |
| `/api/v1/services/{id}/connect` | `POST` | `200` |
| `/api/v1/services/{id}/disconnect` | `POST` | `200` |
| `/api/v1/services/{id}/status` | `GET` | `200` |
| `/api/v1/services/{id}/tools` | `GET` | `200` |
| `/api/v1/services/{id}/sync-tools` | `POST` | `200` |
| `/api/v1/tools/{id}` | `GET` | `200` |
| `/api/v1/tools/{id}/invoke` | `POST` | `200` |
| `/api/v1/history` | `GET` | `200` |
| `/api/v1/history/{id}` | `GET` | `200` |
| `/api/v1/audit-logs` | `GET` | `200` |
| `/api/v1/audit-logs/export` | `GET` | `200` |

## 5. 测试过程

### 5.1 GET /health

请求地址：`http://127.0.0.1:18080/health`

```bash
curl -sS http://127.0.0.1:18080/health
```

返回：`HTTP 200`

```json
{"status":"ok"}
```

### 5.2 GET /swagger/index.html

请求地址：`http://127.0.0.1:18080/swagger/index.html`

```bash
curl -sS http://127.0.0.1:18080/swagger/index.html
```

返回：`HTTP 200`

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Swagger UI</title>
  <link rel="stylesheet" type="text/css" href="./swagger-ui.css" >
```

### 5.3 POST /api/v1/auth/login

请求地址：`http://127.0.0.1:18080/api/v1/auth/login`

请求数据：

```json
{
  "username": "root",
  "password": "admin123456"
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"root","password":"admin123456"}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "<redacted-access-token>",
    "refresh_token": "<redacted-refresh-token>",
    "expires_in": 7200,
    "user": {
      "id": "d71ec889-73b1-4030-9243-d4e6e89abe2d",
      "username": "root",
      "email": "root@example.com",
      "role": "admin",
      "is_first_login": true
    }
  }
}
```

### 5.4 POST /api/v1/auth/refresh

请求地址：`http://127.0.0.1:18080/api/v1/auth/refresh`

请求数据：

```json
{
  "refresh_token": "<root refresh token>"
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/auth/refresh" \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"'$ROOT_REFRESH_TOKEN'"}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "<redacted-access-token>",
    "refresh_token": "<redacted-refresh-token>",
    "expires_in": 7200
  }
}
```

### 5.5 POST /api/v1/users

请求地址：`http://127.0.0.1:18080/api/v1/users`

请求数据：

```json
{
  "username": "operator1",
  "password": "operator123",
  "email": "operator1@example.com",
  "role": "operator"
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/users" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN" \
  -d '{"username":"operator1","password":"operator123","email":"operator1@example.com","role":"operator"}'
```

返回：`HTTP 201`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "dafe99bf-1fb7-48d7-a7b7-91ba15d8e648",
    "username": "operator1",
    "email": "operator1@example.com",
    "role": "operator",
    "is_active": true,
    "is_first_login": true
  }
}
```

### 5.6 GET /api/v1/users

请求地址：`http://127.0.0.1:18080/api/v1/users?page=1&page_size=10`

```bash
curl -sS "$BASE_URL/api/v1/users?page=1&page_size=10" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": "dafe99bf-1fb7-48d7-a7b7-91ba15d8e648",
        "username": "operator1",
        "role": "operator"
      },
      {
        "id": "d71ec889-73b1-4030-9243-d4e6e89abe2d",
        "username": "root",
        "role": "admin"
      }
    ],
    "page": 1,
    "page_size": 10,
    "total": 2
  }
}
```

### 5.7 POST /api/v1/auth/login

请求地址：`http://127.0.0.1:18080/api/v1/auth/login`

请求数据：

```json
{
  "username": "operator1",
  "password": "operator123"
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"operator1","password":"operator123"}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "<redacted-access-token>",
    "refresh_token": "<redacted-refresh-token>",
    "user": {
      "id": "dafe99bf-1fb7-48d7-a7b7-91ba15d8e648",
      "username": "operator1",
      "role": "operator",
      "is_first_login": true
    }
  }
}
```

### 5.8 PUT /api/v1/users/{id}/password

请求地址：`http://127.0.0.1:18080/api/v1/users/$OPERATOR_ID/password`

请求数据：

```json
{
  "old_password": "operator123",
  "new_password": "operator456"
}
```

```bash
curl -sS -X PUT "$BASE_URL/api/v1/users/$OPERATOR_ID/password" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $OP_ACCESS_TOKEN" \
  -d '{"old_password":"operator123","new_password":"operator456"}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "ok": true
  }
}
```

### 5.9 POST /api/v1/auth/login

请求地址：`http://127.0.0.1:18080/api/v1/auth/login`

请求数据：

```json
{
  "username": "operator1",
  "password": "operator456"
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"operator1","password":"operator456"}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "<redacted-access-token>",
    "refresh_token": "<redacted-refresh-token>",
    "user": {
      "id": "dafe99bf-1fb7-48d7-a7b7-91ba15d8e648",
      "username": "operator1",
      "role": "operator",
      "is_first_login": false
    }
  }
}
```

### 5.10 PUT /api/v1/users/{id}

请求地址：`http://127.0.0.1:18080/api/v1/users/$OPERATOR_ID`

请求数据：

```json
{
  "email": "operator1-updated@example.com",
  "role": "operator",
  "is_active": true
}
```

```bash
curl -sS -X PUT "$BASE_URL/api/v1/users/$OPERATOR_ID" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN" \
  -d '{"email":"operator1-updated@example.com","role":"operator","is_active":true}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "dafe99bf-1fb7-48d7-a7b7-91ba15d8e648",
    "email": "operator1-updated@example.com",
    "role": "operator",
    "is_active": true,
    "is_first_login": false
  }
}
```

### 5.11 POST /api/v1/services

请求地址：`http://127.0.0.1:18080/api/v1/services`

请求数据：

```json
{
  "name": "mock-echo",
  "description": "curl test service",
  "transport_type": "streamable_http",
  "url": "http://127.0.0.1:18081/mcp",
  "session_mode": "auto",
  "compat_mode": "off",
  "listen_enabled": true,
  "timeout": 10,
  "custom_headers": {},
  "tags": ["curl", "mock"]
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/services" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN" \
  -d '{"name":"mock-echo","description":"curl test service","transport_type":"streamable_http","url":"http://127.0.0.1:18081/mcp","session_mode":"auto","compat_mode":"off","listen_enabled":true,"timeout":10,"custom_headers":{},"tags":["curl","mock"]}'
```

返回：`HTTP 201`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
    "name": "mock-echo",
    "transport_type": "streamable_http",
    "url": "http://127.0.0.1:18081/mcp",
    "status": "DISCONNECTED",
    "tags": ["curl", "mock"]
  }
}
```

### 5.12 GET /api/v1/services

请求地址：`http://127.0.0.1:18080/api/v1/services?page=1&page_size=10`

```bash
curl -sS "$BASE_URL/api/v1/services?page=1&page_size=10" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
        "name": "mock-echo",
        "transport_type": "streamable_http",
        "status": "DISCONNECTED"
      }
    ],
    "page": 1,
    "page_size": 10,
    "total": 1
  }
}
```

### 5.13 GET /api/v1/services/{id}

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID`

```bash
curl -sS "$BASE_URL/api/v1/services/$SERVICE_ID" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
    "name": "mock-echo",
    "transport_type": "streamable_http",
    "url": "http://127.0.0.1:18081/mcp",
    "status": "DISCONNECTED"
  }
}
```

### 5.14 PUT /api/v1/services/{id}

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID`

请求数据：

```json
{
  "name": "mock-echo-updated",
  "description": "curl test service updated",
  "transport_type": "streamable_http",
  "url": "http://127.0.0.1:18081/mcp",
  "session_mode": "auto",
  "compat_mode": "off",
  "listen_enabled": true,
  "timeout": 12,
  "custom_headers": {
    "X-Test": "curl"
  },
  "tags": ["curl", "mock", "updated"]
}
```

```bash
curl -sS -X PUT "$BASE_URL/api/v1/services/$SERVICE_ID" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN" \
  -d '{"name":"mock-echo-updated","description":"curl test service updated","transport_type":"streamable_http","url":"http://127.0.0.1:18081/mcp","session_mode":"auto","compat_mode":"off","listen_enabled":true,"timeout":12,"custom_headers":{"X-Test":"curl"},"tags":["curl","mock","updated"]}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
    "name": "mock-echo-updated",
    "timeout": 12,
    "custom_headers": {
      "X-Test": "curl"
    },
    "status": "DISCONNECTED"
  }
}
```

### 5.15 POST /api/v1/services/{id}/connect

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID/connect`

```bash
curl -sS -X POST "$BASE_URL/api/v1/services/$SERVICE_ID/connect" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "service_id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
    "status": "CONNECTED",
    "transport_type": "streamable_http",
    "session_id_exists": true,
    "protocol_version": "2025-11-25",
    "listen_enabled": true,
    "listen_active": true
  }
}
```

### 5.16 GET /api/v1/services/{id}/status

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID/status`

```bash
curl -sS "$BASE_URL/api/v1/services/$SERVICE_ID/status" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
    "name": "mock-echo-updated",
    "status": "CONNECTED",
    "transport_type": "streamable_http",
    "session_id_exists": true,
    "protocol_version": "2025-11-25",
    "listen_active": true
  }
}
```

### 5.17 POST /api/v1/services/{id}/sync-tools

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID/sync-tools`

```bash
curl -sS -X POST "$BASE_URL/api/v1/services/$SERVICE_ID/sync-tools" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": "8e705b9e-e9c3-4d38-bc32-4b7a8478eadc",
      "name": "echo",
      "description": "回显文本"
    },
    {
      "id": "a4573d91-bc77-40db-8d4b-a427231129cd",
      "name": "sum",
      "description": "计算两个数字之和"
    }
  ]
}
```

### 5.18 GET /api/v1/services/{id}/tools

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID/tools`

```bash
curl -sS "$BASE_URL/api/v1/services/$SERVICE_ID/tools" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": "8e705b9e-e9c3-4d38-bc32-4b7a8478eadc",
      "name": "echo"
    },
    {
      "id": "a4573d91-bc77-40db-8d4b-a427231129cd",
      "name": "sum"
    }
  ]
}
```

### 5.19 GET /api/v1/tools/{id}

请求地址：`http://127.0.0.1:18080/api/v1/tools/$TOOL_ID`

```bash
curl -sS "$BASE_URL/api/v1/tools/$TOOL_ID" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "8e705b9e-e9c3-4d38-bc32-4b7a8478eadc",
    "mcp_service_id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
    "name": "echo",
    "description": "回显文本",
    "is_enabled": true
  }
}
```

### 5.20 POST /api/v1/tools/{id}/invoke

请求地址：`http://127.0.0.1:18080/api/v1/tools/$TOOL_ID/invoke`

请求数据：

```json
{
  "arguments": {
    "text": "hello-from-curl"
  }
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/tools/$TOOL_ID/invoke" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $OP_ACCESS_TOKEN" \
  -d '{"arguments":{"text":"hello-from-curl"}}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "result": {
      "transport_type": "streamable_http",
      "payload": {
        "content": [
          {
            "text": "echo:hello-from-curl",
            "type": "text"
          }
        ],
        "is_error": false
      }
    },
    "duration_ms": 0
  }
}
```

### 5.21 GET /api/v1/history

请求地址：`http://127.0.0.1:18080/api/v1/history?page=1&page_size=10`

```bash
curl -sS "$BASE_URL/api/v1/history?page=1&page_size=10" \
  -H "Authorization: Bearer $OP_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": "38dd1114-4a64-4188-b0d1-02b8e5a5157a",
        "mcp_service_id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
        "tool_name": "echo",
        "user_id": "dafe99bf-1fb7-48d7-a7b7-91ba15d8e648",
        "status": "success",
        "duration_ms": 0
      }
    ],
    "page": 1,
    "page_size": 10,
    "total": 1
  }
}
```

### 5.22 GET /api/v1/history/{id}

请求地址：`http://127.0.0.1:18080/api/v1/history/$HISTORY_ID`

```bash
curl -sS "$BASE_URL/api/v1/history/$HISTORY_ID" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "38dd1114-4a64-4188-b0d1-02b8e5a5157a",
    "mcp_service_id": "effee9ff-0c08-4bdc-91a2-01fb9d80694a",
    "tool_name": "echo",
    "request_body": {
      "text": "hello-from-curl"
    },
    "response_body": {
      "content": [
        {
          "text": "echo:hello-from-curl",
          "type": "text"
        }
      ],
      "is_error": false
    },
    "status": "success"
  }
}
```

### 5.23 GET /api/v1/audit-logs

请求地址：`http://127.0.0.1:18080/api/v1/audit-logs?page=1&page_size=20`

```bash
curl -sS "$BASE_URL/api/v1/audit-logs?page=1&page_size=20" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "action": "auth.login",
        "resource_type": "user"
      },
      {
        "action": "service.connect",
        "resource_type": "service"
      },
      {
        "action": "tool.invoke",
        "resource_type": "tool"
      }
    ],
    "page": 1,
    "page_size": 20,
    "total": 10
  }
}
```

### 5.24 GET /api/v1/audit-logs/export

请求地址：`http://127.0.0.1:18080/api/v1/audit-logs/export?action=service.connect`

```bash
curl -sS "$BASE_URL/api/v1/audit-logs/export?action=service.connect" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```csv
id,user_id,username,action,resource_type,resource_id,detail,ip_address,user_agent,created_at
4,,"","service.connect","service","effee9ff-0c08-4bdc-91a2-01fb9d80694a","连接 MCP 服务","127.0.0.1","curl/8.x","2026-03-06T16:41:11+08:00"
```

### 5.25 POST /api/v1/auth/logout

请求地址：`http://127.0.0.1:18080/api/v1/auth/logout`

请求数据：

```json
{
  "refresh_token": "<operator refresh token>"
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/auth/logout" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $OP_ACCESS_TOKEN" \
  -d '{"refresh_token":"'$OP_REFRESH_TOKEN'"}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "ok": true
  }
}
```

### 5.26 POST /api/v1/services/{id}/disconnect

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID/disconnect`

```bash
curl -sS -X POST "$BASE_URL/api/v1/services/$SERVICE_ID/disconnect" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "ok": true
  }
}
```

### 5.27 DELETE /api/v1/services/{id}

请求地址：`http://127.0.0.1:18080/api/v1/services/$SERVICE_ID`

```bash
curl -sS -X DELETE "$BASE_URL/api/v1/services/$SERVICE_ID" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "ok": true
  }
}
```

### 5.28 DELETE /api/v1/users/{id}

请求地址：`http://127.0.0.1:18080/api/v1/users/$OPERATOR_ID`

```bash
curl -sS -X DELETE "$BASE_URL/api/v1/users/$OPERATOR_ID" \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN"
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "ok": true
  }
}
```

### 5.29 POST /api/v1/auth/logout

请求地址：`http://127.0.0.1:18080/api/v1/auth/logout`

请求数据：

```json
{
  "refresh_token": "<root refresh token>"
}
```

```bash
curl -sS -X POST "$BASE_URL/api/v1/auth/logout" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ROOT_ACCESS_TOKEN" \
  -d '{"refresh_token":"'$ROOT_REFRESH_TOKEN'"}'
```

返回：`HTTP 200`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "ok": true
  }
}
```

## 6. 补充说明

- 登录、刷新、登出接口中的 JWT 已在本文档中做脱敏处理。
- `POST /api/v1/tools/{id}/invoke` 的返回体不会直接返回 `history_id`，历史详情中的 `HISTORY_ID` 来自 `GET /api/v1/history` 的真实返回。
- 本次测试覆盖了当前 `internal/router/router.go` 中注册的全部 HTTP 路由。
