#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(pwd)"
LOCAL_DATA_DIR="$ROOT_DIR/.localdata"
LOG_DIR="$LOCAL_DATA_DIR/logs"
BACKEND_LOG="$LOG_DIR/backend.log"
FRONTEND_LOG="$LOG_DIR/frontend.log"
BACKEND_CONFIG_LOCAL="$ROOT_DIR/backend/configs/config.local.json"

mkdir -p "$LOG_DIR"

if [ ! -f "$BACKEND_CONFIG_LOCAL" ]; then
  echo "==> 创建本地配置"
  cat > "$BACKEND_CONFIG_LOCAL" <<EOF
{
  "database": {
    "path": "$LOCAL_DATA_DIR/openshare.db"
  },
  "storage": {
    "root": "$LOCAL_DATA_DIR"
  },
  "session": {
    "secret": "dev-local-session-secret"
  }
}
EOF
else
  echo "==> 使用现有本地配置"
fi

echo "==> 安装前端依赖"
cd "$ROOT_DIR/frontend"
npm install > "$FRONTEND_LOG" 2>&1

echo "==> 启动前端开发服务器"
npm run dev -- --host 0.0.0.0 > "$FRONTEND_LOG" 2>&1 &
FRONTEND_PID=$!

echo "==> 启动后端服务"
cd "$ROOT_DIR/backend"
go run ./cmd/server > "$BACKEND_LOG" 2>&1 &
BACKEND_PID=$!

echo
echo "OpenShare 已启动"
echo "Public: http://localhost:5173/"
echo "Admin : http://localhost:5173/admin"
echo "Health: http://127.0.0.1:8080/healthz"
echo "Logs  : $LOG_DIR"
echo

attempts=30
for ((i = 1; i <= attempts; i++)); do
  if [[ -f "$BACKEND_LOG" ]]; then
    line="$(grep -E '\[bootstrap\] super admin initialized; username=.* password=.*' "$BACKEND_LOG" | tail -n 1 || true)"
    if [[ -n "$line" ]]; then
      echo
      echo "超级管理员初始凭据："
      echo "$line"
      echo
      break
    fi
  fi
  sleep 1
done

echo "按 Ctrl+C 停止服务"

trap 'kill $FRONTEND_PID $BACKEND_PID 2>/dev/null' EXIT
wait