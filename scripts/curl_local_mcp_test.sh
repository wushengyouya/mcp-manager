#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
MCP_SERVER_URL="${MCP_SERVER_URL:-http://127.0.0.1:28080/mcp}"
TOKEN=""
SERVICE_ID=""
TOOL_ID=""

cleanup() {
  if [[ -n "${SERVICE_ID}" && "${SERVICE_ID}" != "null" && -n "${TOKEN}" ]]; then
    echo "== cleanup: disconnect =="
    curl -sS -X POST "${BASE_URL}/api/v1/services/${SERVICE_ID}/disconnect" \
      -H "Authorization: Bearer ${TOKEN}" | jq . || true
    echo

    echo "== cleanup: delete =="
    curl -sS -X DELETE "${BASE_URL}/api/v1/services/${SERVICE_ID}" \
      -H "Authorization: Bearer ${TOKEN}" | jq . || true
    echo
  fi
}
trap cleanup EXIT

echo "== 1. 直连 MCP 服务: GET /mcp =="
curl -i -sS "${MCP_SERVER_URL}"
echo
echo

echo "== 2. 直连 MCP 服务: initialize =="
INIT_RESP=$(
  curl -i -sS -X POST "${MCP_SERVER_URL}" \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    --data '{
      "jsonrpc":"2.0",
      "id":1,
      "method":"initialize",
      "params":{
        "protocolVersion":"2025-06-18",
        "capabilities":{},
        "clientInfo":{"name":"curl-test","version":"1.0.0"}
      }
    }'
)
printf '%s\n\n' "${INIT_RESP}"

echo "== 3. 健康检查 =="
curl -sS "${BASE_URL}/health" | jq .
echo

echo "== 4. 登录管理端 =="
LOGIN_RESP=$(
  curl -sS -X POST "${BASE_URL}/api/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d '{"username":"root","password":"admin123456"}'
)
printf '%s\n' "${LOGIN_RESP}" | jq .
TOKEN=$(printf '%s' "${LOGIN_RESP}" | jq -r '.data.access_token')
echo

echo "== 5. 创建 MCP 服务 =="
CREATE_RESP=$(
  curl -sS -X POST "${BASE_URL}/api/v1/services" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${TOKEN}" \
    -d "{
      \"name\":\"streamhttp-test\",
      \"description\":\"test against localhost:28080/mcp\",
      \"transport_type\":\"streamable_http\",
      \"url\":\"${MCP_SERVER_URL}\",
      \"session_mode\":\"auto\",
      \"compat_mode\":\"off\",
      \"listen_enabled\":true,
      \"timeout\":10,
      \"custom_headers\":{},
      \"tags\":[\"curl\",\"manual\"]
    }"
)
printf '%s\n' "${CREATE_RESP}" | jq .
SERVICE_ID=$(printf '%s' "${CREATE_RESP}" | jq -r '.data.id')
echo

echo "== 6. 连接服务 =="
curl -sS -X POST "${BASE_URL}/api/v1/services/${SERVICE_ID}/connect" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
echo

echo "== 7. 查看状态 =="
curl -sS "${BASE_URL}/api/v1/services/${SERVICE_ID}/status" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
echo

echo "== 8. 同步工具 =="
SYNC_RESP=$(
  curl -sS -X POST "${BASE_URL}/api/v1/services/${SERVICE_ID}/sync-tools" \
    -H "Authorization: Bearer ${TOKEN}"
)
printf '%s\n' "${SYNC_RESP}" | jq .
TOOL_ID=$(printf '%s' "${SYNC_RESP}" | jq -r '.data[0].id')
echo

echo "== 9. 查询工具列表 =="
curl -sS "${BASE_URL}/api/v1/services/${SERVICE_ID}/tools" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
echo

echo "== 10. 调用第一个工具 =="
curl -sS -X POST "${BASE_URL}/api/v1/tools/${TOOL_ID}/invoke" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{"arguments":{"text":"hello from curl","uppercase":true}}' | jq .
echo

echo "== 11. 查看历史记录 =="
curl -sS "${BASE_URL}/api/v1/history?page=1&page_size=5" \
  -H "Authorization: Bearer ${TOKEN}" | jq .
echo
