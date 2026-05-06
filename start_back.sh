#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(pwd)"
LOCAL_DATA_DIR="$ROOT_DIR/.localdata"
LOG_DIR="$LOCAL_DATA_DIR/logs"
BACKEND_LOG="$LOG_DIR/backend.log"
FRONTEND_LOG="$LOG_DIR/frontend.log"
BACKEND_CONFIG_LOCAL="$ROOT_DIR/backend/configs/config.local.json"

BACKEND_PORT=8080
FRONTEND_PORT=5173

mkdir -p "$LOG_DIR"

# 检测端口占用
if ss -tlnp 2>/dev/null | grep -q ":${BACKEND_PORT} "; then
  echo "错误: 后端端口 ${BACKEND_PORT} 已被占用，请先释放该端口"
  exit 1
fi

if ss -tlnp 2>/dev/null | grep -q ":${FRONTEND_PORT} "; then
  echo "错误: 前端端口 ${FRONTEND_PORT} 已被占用，请先释放该端口"
  exit 1
fi

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

echo "==> 启动前端开发服务器（后台永久运行）"
nohup npm run dev -- --host 0.0.0.0 > "$FRONTEND_LOG" 2>&1 &
FRONTEND_PID=$!


echo "==> 启动后端服务（后台永久运行）"
cd "$ROOT_DIR/backend"
nohup go run ./cmd/server > "$BACKEND_LOG" 2>&1 &
BACKEND_PID=$!

# 改动2：将 PID 写入文件，方便后续手动停止
echo "$FRONTEND_PID" > "$LOCAL_DATA_DIR/frontend.pid"
echo "$BACKEND_PID" > "$LOCAL_DATA_DIR/backend.pid"

echo
echo "OpenShare 已启动（守护模式）"
echo "Public: http://localhost:5173/"
echo "Admin : http://localhost:5173/admin"
echo "Health: http://127.0.0.1:8080/healthz"
echo "Logs  : $LOG_DIR"
echo "PID文件: $LOCAL_DATA_DIR/frontend.pid , $LOCAL_DATA_DIR/backend.pid"
echo

# 改动3：仍然尝试显示超级管理员密码（从日志中读取，可能稍晚出现）
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

echo "服务已在后台运行，退出终端不会中断"
echo "如需停止服务，请执行："
echo "  kill \$(cat $LOCAL_DATA_DIR/frontend.pid) \$(cat $LOCAL_DATA_DIR/backend.pid) 2>/dev/null"