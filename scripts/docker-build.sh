#!/usr/bin/env bash
set -euo pipefail
docker build -t mcp-manager:latest -f deployments/docker/Dockerfile .
