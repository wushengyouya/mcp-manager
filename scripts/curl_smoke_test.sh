#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
APP_PORT=${APP_PORT:-18080}
MOCK_PORT=${MOCK_PORT:-18081}
APP_BASE_URL=${APP_BASE_URL:-"http://127.0.0.1:${APP_PORT}"}
MCP_SERVER_URL=${MCP_SERVER_URL:-"http://127.0.0.1:${MOCK_PORT}/mcp"}

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/mcp-manager-smoke-XXXXXX")
RESULT_DIR="${TMP_DIR}/results"
CONFIG_DIR="${TMP_DIR}/config"
DB_PATH="${TMP_DIR}/mcp_manager.sqlite"
APP_BIN="${TMP_DIR}/mcp-manager-server"
APP_LOG="${TMP_DIR}/app.log"
MOCK_LOG="${TMP_DIR}/mock.log"
mkdir -p "${RESULT_DIR}" "${CONFIG_DIR}"

APP_PID=""
MOCK_PID=""

cleanup() {
  set +e
  if [[ -n "${APP_PID}" ]]; then
    kill "${APP_PID}" >/dev/null 2>&1 || true
    wait "${APP_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${MOCK_PID}" ]]; then
    kill "${MOCK_PID}" >/dev/null 2>&1 || true
    wait "${MOCK_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 1
  fi
}

wait_for_http() {
  local url=$1
  local retries=${2:-40}
  local interval=${3:-0.5}
  local i
  for ((i = 1; i <= retries; i++)); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep "${interval}"
  done
  echo "等待服务就绪失败: ${url}" >&2
  return 1
}

wait_for_port() {
  local host=$1
  local port=$2
  local retries=${3:-40}
  local interval=${4:-0.5}
  local i
  for ((i = 1; i <= retries; i++)); do
    if (echo >/dev/tcp/"${host}"/"${port}") >/dev/null 2>&1; then
      return 0
    fi
    sleep "${interval}"
  done
  echo "等待端口就绪失败: ${host}:${port}" >&2
  return 1
}

call_json() {
  local name=$1
  local method=$2
  local url=$3
  local payload=${4-}
  local token=${5-}
  local body_file="${RESULT_DIR}/${name}.body"
  local code_file="${RESULT_DIR}/${name}.code"

  if [[ -n "${payload}" && -n "${token}" ]]; then
    curl -sS -o "${body_file}" -w '%{http_code}' -X "${method}" "${url}" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${token}" \
      --data "${payload}" > "${code_file}"
  elif [[ -n "${payload}" ]]; then
    curl -sS -o "${body_file}" -w '%{http_code}' -X "${method}" "${url}" \
      -H 'Content-Type: application/json' \
      --data "${payload}" > "${code_file}"
  elif [[ -n "${token}" ]]; then
    curl -sS -o "${body_file}" -w '%{http_code}' -X "${method}" "${url}" \
      -H "Authorization: Bearer ${token}" > "${code_file}"
  else
    curl -sS -o "${body_file}" -w '%{http_code}' -X "${method}" "${url}" > "${code_file}"
  fi
}

call_plain() {
  local name=$1
  local method=$2
  local url=$3
  local token=${4-}
  local body_file="${RESULT_DIR}/${name}.body"
  local code_file="${RESULT_DIR}/${name}.code"

  if [[ -n "${token}" ]]; then
    curl -sS -o "${body_file}" -w '%{http_code}' -X "${method}" "${url}" \
      -H "Authorization: Bearer ${token}" > "${code_file}"
  else
    curl -sS -o "${body_file}" -w '%{http_code}' -X "${method}" "${url}" > "${code_file}"
  fi
}

assert_code() {
  local name=$1
  local expected=$2
  local actual
  actual=$(<"${RESULT_DIR}/${name}.code")
  if [[ "${actual}" != "${expected}" ]]; then
    echo "接口 ${name} 返回状态码异常: 期望 ${expected}, 实际 ${actual}" >&2
    echo "响应体:" >&2
    cat "${RESULT_DIR}/${name}.body" >&2
    exit 1
  fi
}

print_step() {
  echo
  echo "==> $1"
}

require_cmd curl
require_cmd jq
require_cmd go
require_cmd sed

print_step "生成临时配置"
cp "${ROOT_DIR}/config.yaml" "${CONFIG_DIR}/config.yaml"
sed -i "s/port: 8080/port: ${APP_PORT}/" "${CONFIG_DIR}/config.yaml"
sed -i "s#dsn: \"data/mcp_manager.db\"#dsn: \"${DB_PATH}\"#" "${CONFIG_DIR}/config.yaml"
sed -i "s/enabled: true/enabled: false/" "${CONFIG_DIR}/config.yaml"

print_step "构建主服务二进制"
(
  cd "${ROOT_DIR}"
  go build -o "${APP_BIN}" ./cmd/server
)

print_step "启动 MCP Mock 服务"
(
  cd "${ROOT_DIR}"
  MCP_TEST_SERVER_ADDR=":${MOCK_PORT}" go run ./scripts/mock_mcp_server >"${MOCK_LOG}" 2>&1
) &
MOCK_PID=$!
wait_for_port 127.0.0.1 "${MOCK_PORT}"

print_step "启动主服务"
(
  cd "${CONFIG_DIR}"
  "${APP_BIN}" >"${APP_LOG}" 2>&1
) &
APP_PID=$!
wait_for_http "${APP_BASE_URL}/health"

print_step "开始执行 curl 冒烟测试"
call_plain health GET "${APP_BASE_URL}/health"
assert_code health 200

call_plain swagger GET "${APP_BASE_URL}/swagger/index.html"
assert_code swagger 200

ROOT_LOGIN_PAYLOAD='{"username":"root","password":"admin123456"}'
call_json auth_login_root POST "${APP_BASE_URL}/api/v1/auth/login" "${ROOT_LOGIN_PAYLOAD}"
assert_code auth_login_root 200
ROOT_ACCESS_TOKEN=$(jq -r '.data.access_token' "${RESULT_DIR}/auth_login_root.body")
ROOT_REFRESH_TOKEN=$(jq -r '.data.refresh_token' "${RESULT_DIR}/auth_login_root.body")
ROOT_ID=$(jq -r '.data.user.id' "${RESULT_DIR}/auth_login_root.body")

ROOT_REFRESH_PAYLOAD=$(jq -nc --arg token "${ROOT_REFRESH_TOKEN}" '{refresh_token:$token}')
call_json auth_refresh_root POST "${APP_BASE_URL}/api/v1/auth/refresh" "${ROOT_REFRESH_PAYLOAD}"
assert_code auth_refresh_root 200
ROOT_ACCESS_TOKEN=$(jq -r '.data.access_token' "${RESULT_DIR}/auth_refresh_root.body")
ROOT_REFRESH_TOKEN=$(jq -r '.data.refresh_token' "${RESULT_DIR}/auth_refresh_root.body")

CREATE_USER_PAYLOAD='{"username":"operator1","password":"operator123","email":"operator1@example.com","role":"operator"}'
call_json users_create POST "${APP_BASE_URL}/api/v1/users" "${CREATE_USER_PAYLOAD}" "${ROOT_ACCESS_TOKEN}"
assert_code users_create 201
OPERATOR_ID=$(jq -r '.data.id' "${RESULT_DIR}/users_create.body")

call_plain users_list GET "${APP_BASE_URL}/api/v1/users?page=1&page_size=10" "${ROOT_ACCESS_TOKEN}"
assert_code users_list 200

OP_LOGIN_PAYLOAD='{"username":"operator1","password":"operator123"}'
call_json auth_login_operator_old POST "${APP_BASE_URL}/api/v1/auth/login" "${OP_LOGIN_PAYLOAD}"
assert_code auth_login_operator_old 200
OP_ACCESS_TOKEN=$(jq -r '.data.access_token' "${RESULT_DIR}/auth_login_operator_old.body")
OP_REFRESH_TOKEN=$(jq -r '.data.refresh_token' "${RESULT_DIR}/auth_login_operator_old.body")

CHANGE_PASSWORD_PAYLOAD='{"old_password":"operator123","new_password":"operator456"}'
call_json users_change_password PUT "${APP_BASE_URL}/api/v1/users/${OPERATOR_ID}/password" "${CHANGE_PASSWORD_PAYLOAD}" "${OP_ACCESS_TOKEN}"
assert_code users_change_password 200

OP_LOGIN_NEW_PAYLOAD='{"username":"operator1","password":"operator456"}'
call_json auth_login_operator_new POST "${APP_BASE_URL}/api/v1/auth/login" "${OP_LOGIN_NEW_PAYLOAD}"
assert_code auth_login_operator_new 200
OP_ACCESS_TOKEN=$(jq -r '.data.access_token' "${RESULT_DIR}/auth_login_operator_new.body")
OP_REFRESH_TOKEN=$(jq -r '.data.refresh_token' "${RESULT_DIR}/auth_login_operator_new.body")

UPDATE_USER_PAYLOAD='{"email":"operator1-updated@example.com","role":"operator","is_active":true}'
call_json users_update PUT "${APP_BASE_URL}/api/v1/users/${OPERATOR_ID}" "${UPDATE_USER_PAYLOAD}" "${ROOT_ACCESS_TOKEN}"
assert_code users_update 200

CREATE_SERVICE_PAYLOAD=$(jq -nc --arg url "${MCP_SERVER_URL}" \
  '{name:"mock-echo",description:"curl test service",transport_type:"streamable_http",url:$url,session_mode:"auto",compat_mode:"off",listen_enabled:true,timeout:10,custom_headers:{},tags:["curl","mock"]}')
call_json services_create POST "${APP_BASE_URL}/api/v1/services" "${CREATE_SERVICE_PAYLOAD}" "${ROOT_ACCESS_TOKEN}"
assert_code services_create 201
SERVICE_ID=$(jq -r '.data.id' "${RESULT_DIR}/services_create.body")

call_plain services_list GET "${APP_BASE_URL}/api/v1/services?page=1&page_size=10" "${ROOT_ACCESS_TOKEN}"
assert_code services_list 200

call_plain services_get GET "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}" "${ROOT_ACCESS_TOKEN}"
assert_code services_get 200

UPDATE_SERVICE_PAYLOAD=$(jq -nc --arg url "${MCP_SERVER_URL}" \
  '{name:"mock-echo-updated",description:"curl test service updated",transport_type:"streamable_http",url:$url,session_mode:"auto",compat_mode:"off",listen_enabled:true,timeout:12,custom_headers:{"X-Test":"curl"},tags:["curl","mock","updated"]}')
call_json services_update PUT "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}" "${UPDATE_SERVICE_PAYLOAD}" "${ROOT_ACCESS_TOKEN}"
assert_code services_update 200

call_json services_connect POST "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}/connect" "" "${ROOT_ACCESS_TOKEN}"
assert_code services_connect 200

call_plain services_status GET "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}/status" "${ROOT_ACCESS_TOKEN}"
assert_code services_status 200

call_json tools_sync POST "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}/sync-tools" "" "${ROOT_ACCESS_TOKEN}"
assert_code tools_sync 200

call_plain tools_list_by_service GET "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}/tools" "${ROOT_ACCESS_TOKEN}"
assert_code tools_list_by_service 200
TOOL_ID=$(jq -r '.data[0].id' "${RESULT_DIR}/tools_list_by_service.body")

call_plain tools_get GET "${APP_BASE_URL}/api/v1/tools/${TOOL_ID}" "${ROOT_ACCESS_TOKEN}"
assert_code tools_get 200

INVOKE_PAYLOAD='{"arguments":{"text":"hello-from-curl"}}'
call_json tools_invoke POST "${APP_BASE_URL}/api/v1/tools/${TOOL_ID}/invoke" "${INVOKE_PAYLOAD}" "${OP_ACCESS_TOKEN}"
assert_code tools_invoke 200

call_plain history_list GET "${APP_BASE_URL}/api/v1/history?page=1&page_size=10" "${OP_ACCESS_TOKEN}"
assert_code history_list 200
HISTORY_ID=$(jq -r '.data.items[0].id' "${RESULT_DIR}/history_list.body")

call_plain history_get GET "${APP_BASE_URL}/api/v1/history/${HISTORY_ID}" "${ROOT_ACCESS_TOKEN}"
assert_code history_get 200

call_plain audit_logs GET "${APP_BASE_URL}/api/v1/audit-logs?page=1&page_size=20" "${ROOT_ACCESS_TOKEN}"
assert_code audit_logs 200

call_plain audit_logs_export GET "${APP_BASE_URL}/api/v1/audit-logs/export?action=service.connect" "${ROOT_ACCESS_TOKEN}"
assert_code audit_logs_export 200

OP_LOGOUT_PAYLOAD=$(jq -nc --arg token "${OP_REFRESH_TOKEN}" '{refresh_token:$token}')
call_json auth_logout_operator POST "${APP_BASE_URL}/api/v1/auth/logout" "${OP_LOGOUT_PAYLOAD}" "${OP_ACCESS_TOKEN}"
assert_code auth_logout_operator 200

call_json services_disconnect POST "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}/disconnect" "" "${ROOT_ACCESS_TOKEN}"
assert_code services_disconnect 200

call_plain services_delete DELETE "${APP_BASE_URL}/api/v1/services/${SERVICE_ID}" "${ROOT_ACCESS_TOKEN}"
assert_code services_delete 200

call_plain users_delete DELETE "${APP_BASE_URL}/api/v1/users/${OPERATOR_ID}" "${ROOT_ACCESS_TOKEN}"
assert_code users_delete 200

ROOT_LOGOUT_PAYLOAD=$(jq -nc --arg token "${ROOT_REFRESH_TOKEN}" '{refresh_token:$token}')
call_json auth_logout_root POST "${APP_BASE_URL}/api/v1/auth/logout" "${ROOT_LOGOUT_PAYLOAD}" "${ROOT_ACCESS_TOKEN}"
assert_code auth_logout_root 200

print_step "关键断言"
jq -e '.status == "ok"' "${RESULT_DIR}/health.body" >/dev/null
jq -e '.data.user.username == "root"' "${RESULT_DIR}/auth_login_root.body" >/dev/null
jq -e '.data.status == "CONNECTED"' "${RESULT_DIR}/services_connect.body" >/dev/null
jq -e '.data[0].name == "echo"' "${RESULT_DIR}/tools_list_by_service.body" >/dev/null
jq -e '.data.result.payload.content[0].text == "echo:hello-from-curl"' "${RESULT_DIR}/tools_invoke.body" >/dev/null
jq -e '.data.items[0].tool_name == "echo"' "${RESULT_DIR}/history_list.body" >/dev/null

print_step "测试完成"
echo "ROOT_ID=${ROOT_ID}"
echo "OPERATOR_ID=${OPERATOR_ID}"
echo "SERVICE_ID=${SERVICE_ID}"
echo "TOOL_ID=${TOOL_ID}"
echo "HISTORY_ID=${HISTORY_ID}"
echo "结果目录: ${RESULT_DIR}"
echo "主服务日志: ${APP_LOG}"
echo "Mock 服务日志: ${MOCK_LOG}"
