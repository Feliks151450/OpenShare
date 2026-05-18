#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$ROOT_DIR/build"

BACKEND_PORT="${BACKEND_PORT:-5173}"
DATA_DIR="${DATA_DIR:-$ROOT_DIR/.production}"
DEV_DATA_DIR="$ROOT_DIR/.localdata"
LOG_DIR="$DATA_DIR/logs"

# 默认内嵌前端（单端口），传 --separated 使用前后端分离（需额外部署 Nginx/Caddy）
MODE="embedded"
DAEMON=false
for arg in "$@"; do
  case "$arg" in
    --separated) MODE="separated" ;;
    --daemon)    DAEMON=true ;;
  esac
done

# 检测开发与生产数据库，自动同步较新的版本
DEV_DB="$DEV_DATA_DIR/openshare.db"
PROD_DB="$DATA_DIR/openshare.db"


if $DAEMON; then
  mkdir -p "$LOG_DIR"
  PID_FILE="$BUILD_DIR/openshare.pid"
  LOG_FILE="$LOG_DIR/server.log"

  # 先停掉旧进程
  if [[ -f "$PID_FILE" ]]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
      echo "  停止旧进程 (PID $OLD_PID)..."
      kill "$OLD_PID"
      sleep 1
    fi
    rm -f "$PID_FILE"
  fi
fi
