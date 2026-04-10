#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://0.0.0.0:8080}"
USERNAME="${USERNAME:-root}"
PASSWORD="${PASSWORD:-admin123456}"
SERVICE_ID="${SERVICE_ID:-}"
SERVICE_NAME="${SERVICE_NAME:-streamhttp-test}"
TOOL_NAME="${TOOL_NAME:-inspect_request}"
TOOL_ARGS="${TOOL_ARGS:-{}}"
INVOKE_FIRST_TOOL="${INVOKE_FIRST_TOOL:-false}"
DISCONNECT_ON_EXIT="${DISCONNECT_ON_EXIT:-false}"
HISTORY_PAGE_SIZE="${HISTORY_PAGE_SIZE:-5}"

TOKEN=""
RESOLVED_SERVICE_ID=""
SELECTED_TOOL_ID=""
SELECTED_TOOL_NAME=""

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 1
  fi
}

cleanup() {
  if [[ "${DISCONNECT_ON_EXIT}" != "true" ]]; then
    return
  fi
  if [[ -z "${TOKEN}" || -z "${RESOLVED_SERVICE_ID}" ]]; then
    return
  fi

  echo "== cleanup: disconnect existing service =="
  curl -sS -X POST "${BASE_URL}/api/v1/services/${RESOLVED_SERVICE_ID}/disconnect" \
    -H "Authorization: Bearer ${TOKEN}" | jq . || true
  echo
}
trap cleanup EXIT

api_call() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local auth="${4:-false}"
  local response_file http_code

  response_file="$(mktemp)"
  if [[ "${auth}" == "true" ]]; then
    if [[ -n "${body}" ]]; then
      http_code="$(
        curl -sS -o "${response_file}" -w "%{http_code}" -X "${method}" "${BASE_URL}${path}" \
          -H 'Content-Type: application/json' \
          -H "Authorization: Bearer ${TOKEN}" \
          --data "${body}"
      )"
    else
      http_code="$(
        curl -sS -o "${response_file}" -w "%{http_code}" -X "${method}" "${BASE_URL}${path}" \
          -H "Authorization: Bearer ${TOKEN}"
      )"
    fi
  else
    if [[ -n "${body}" ]]; then
      http_code="$(
        curl -sS -o "${response_file}" -w "%{http_code}" -X "${method}" "${BASE_URL}${path}" \
          -H 'Content-Type: application/json' \
          --data "${body}"
      )"
    else
      http_code="$(
        curl -sS -o "${response_file}" -w "%{http_code}" -X "${method}" "${BASE_URL}${path}"
      )"
    fi
  fi

  printf 'HTTP %s\n' "${http_code}" >&2
  jq . "${response_file}" >&2 || cat "${response_file}" >&2
  echo >&2

  if [[ ! "${http_code}" =~ ^2 ]]; then
    echo "请求失败: ${method} ${path}" >&2
    rm -f "${response_file}"
    exit 1
  fi

  if jq -e 'has("code") and .code != 0' "${response_file}" >/dev/null 2>&1; then
    echo "业务失败: ${method} ${path}" >&2
    rm -f "${response_file}"
    exit 1
  fi

  cat "${response_file}"
  rm -f "${response_file}"
}

resolve_service_id() {
  local list_resp

  if [[ -n "${SERVICE_ID}" ]]; then
    RESOLVED_SERVICE_ID="${SERVICE_ID}"
    return
  fi

  if [[ -z "${SERVICE_NAME}" ]]; then
    echo "必须提供 SERVICE_ID 或 SERVICE_NAME" >&2
    exit 1
  fi

  echo "== 3. 查询服务列表并按名称定位 =="
  list_resp="$(api_call GET "/api/v1/services?page=1&page_size=100" "" true)"
  RESOLVED_SERVICE_ID="$(
    printf '%s' "${list_resp}" | jq -r --arg name "${SERVICE_NAME}" '.data.items[] | select(.name == $name) | .id' | head -n 1
  )"
  if [[ -z "${RESOLVED_SERVICE_ID}" || "${RESOLVED_SERVICE_ID}" == "null" ]]; then
    echo "未找到服务: ${SERVICE_NAME}" >&2
    exit 1
  fi
}

select_tool() {
  local tools_json="$1"

  if [[ -n "${TOOL_NAME}" ]]; then
    SELECTED_TOOL_ID="$(
      printf '%s' "${tools_json}" | jq -r --arg name "${TOOL_NAME}" '.data[] | select(.name == $name) | .id' | head -n 1
    )"
    SELECTED_TOOL_NAME="${TOOL_NAME}"
    if [[ -z "${SELECTED_TOOL_ID}" || "${SELECTED_TOOL_ID}" == "null" ]]; then
      echo "未找到工具: ${TOOL_NAME}" >&2
      exit 1
    fi
    return
  fi

  if [[ "${INVOKE_FIRST_TOOL}" == "true" ]]; then
    SELECTED_TOOL_ID="$(printf '%s' "${tools_json}" | jq -r '.data[0].id // empty')"
    SELECTED_TOOL_NAME="$(printf '%s' "${tools_json}" | jq -r '.data[0].name // empty')"
  fi
}

require_bin curl
require_bin jq

echo "== 1. 健康检查 =="
api_call GET "/health" >/dev/null

echo "== 2. 登录管理端 =="
login_resp="$(api_call POST "/api/v1/auth/login" "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
TOKEN="$(printf '%s' "${login_resp}" | jq -r '.data.access_token')"
if [[ -z "${TOKEN}" || "${TOKEN}" == "null" ]]; then
  echo "登录成功但未获取 access_token" >&2
  exit 1
fi

resolve_service_id

echo "== 4. 查询服务详情 =="
api_call GET "/api/v1/services/${RESOLVED_SERVICE_ID}" "" true >/dev/null

echo "== 5. 连接已存在服务 =="
api_call POST "/api/v1/services/${RESOLVED_SERVICE_ID}/connect" "" true >/dev/null

echo "== 6. 查看服务状态 =="
api_call GET "/api/v1/services/${RESOLVED_SERVICE_ID}/status" "" true >/dev/null

echo "== 7. 同步工具 =="
api_call POST "/api/v1/services/${RESOLVED_SERVICE_ID}/sync-tools" "" true >/dev/null

echo "== 8. 查询工具列表 =="
tools_resp="$(api_call GET "/api/v1/services/${RESOLVED_SERVICE_ID}/tools" "" true)"

select_tool "${tools_resp}"

if [[ -n "${SELECTED_TOOL_ID}" && "${SELECTED_TOOL_ID}" != "null" ]]; then
  echo "== 9. 调用工具 ${SELECTED_TOOL_NAME} =="
  api_call POST "/api/v1/tools/${SELECTED_TOOL_ID}/invoke" "{\"arguments\":${TOOL_ARGS}}" true >/dev/null
else
  echo "== 9. 跳过工具调用 =="
  echo "未指定 TOOL_NAME，且 INVOKE_FIRST_TOOL != true"
  echo
fi

echo "== 10. 查询该服务历史记录 =="
api_call GET "/api/v1/history?page=1&page_size=${HISTORY_PAGE_SIZE}&service_id=${RESOLVED_SERVICE_ID}" "" true >/dev/null
