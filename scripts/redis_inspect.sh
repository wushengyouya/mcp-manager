#!/usr/bin/env bash

set -euo pipefail

COMPOSE_FILE="../deployments/docker/docker-compose.yml"
SERVICE_NAME="redis"
PATTERN="*"
SHOW_VALUES=0
SHOW_TTL=0
ONLY_KEY=""

usage() {
  cat <<'EOF'
用法：
  scripts/redis_inspect.sh [选项]

说明：
  默认进入 docker compose 的 redis 容器，列出键名。

选项：
  --pattern <glob>     按模式过滤键，默认 *
  --key <name>         仅查看指定键
  --values             同时打印键的类型和值
  --ttl                同时打印 TTL
  --compose-file <p>   指定 compose 文件，默认 deployments/docker/docker-compose.yml
  --service <name>     指定 redis 服务名，默认 redis
  -h, --help           显示帮助

示例：
  scripts/redis_inspect.sh
  scripts/redis_inspect.sh --pattern 'mcp-manager:*'
  scripts/redis_inspect.sh --values --ttl
  scripts/redis_inspect.sh --key 'mcp-manager:runtime:snapshot:svc-1' --values --ttl
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pattern)
      PATTERN="${2:?缺少 pattern 参数}"
      shift 2
      ;;
    --key)
      ONLY_KEY="${2:?缺少 key 参数}"
      shift 2
      ;;
    --values)
      SHOW_VALUES=1
      shift
      ;;
    --ttl)
      SHOW_TTL=1
      shift
      ;;
    --compose-file)
      COMPOSE_FILE="${2:?缺少 compose-file 参数}"
      shift 2
      ;;
    --service)
      SERVICE_NAME="${2:?缺少 service 参数}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v docker >/dev/null 2>&1; then
  echo "未找到 docker 命令" >&2
  exit 1
fi

if [[ ! -f "$COMPOSE_FILE" ]]; then
  echo "compose 文件不存在: $COMPOSE_FILE" >&2
  exit 1
fi

exec_redis() {
  docker compose -f "$COMPOSE_FILE" exec -T "$SERVICE_NAME" redis-cli "$@"
}

print_value_by_type() {
  local key="$1"
  local type
  type="$(exec_redis TYPE "$key" | tr -d '\r')"

  echo "TYPE: $type"
  case "$type" in
    string)
      exec_redis GET "$key"
      ;;
    hash)
      exec_redis HGETALL "$key"
      ;;
    list)
      exec_redis LRANGE "$key" 0 -1
      ;;
    set)
      exec_redis SMEMBERS "$key"
      ;;
    zset)
      exec_redis ZRANGE "$key" 0 -1 WITHSCORES
      ;;
    none)
      echo "(键不存在)"
      ;;
    *)
      echo "(暂不支持该类型的值展示)"
      ;;
  esac
}

print_key() {
  local key="$1"
  echo "== $key =="
  if [[ "$SHOW_TTL" -eq 1 ]]; then
    echo "TTL: $(exec_redis TTL "$key" | tr -d '\r')"
  fi
  if [[ "$SHOW_VALUES" -eq 1 ]]; then
    print_value_by_type "$key"
  fi
  echo
}

if [[ -n "$ONLY_KEY" ]]; then
  print_key "$ONLY_KEY"
  exit 0
fi

mapfile -t KEYS < <(exec_redis --scan --pattern "$PATTERN" | sed '/^$/d')

if [[ "${#KEYS[@]}" -eq 0 ]]; then
  echo "未找到匹配的键，pattern=$PATTERN"
  exit 0
fi

for key in "${KEYS[@]}"; do
  if [[ "$SHOW_VALUES" -eq 1 || "$SHOW_TTL" -eq 1 ]]; then
    print_key "$key"
  else
    echo "$key"
  fi
done
