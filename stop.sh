#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$ROOT_DIR/build"
DEV_DATA_DIR="$ROOT_DIR/.localdata"

# 默认停止开发服务（start.sh / start_back.sh 启动的实例）；
# --daemon 停止生产服务（deploy.sh --daemon 启动的实例）。
MODE="dev"
for arg in "$@"; do
  case "$arg" in
    --daemon) MODE="daemon" ;;
    --dev)    MODE="dev" ;;
  esac
done

# 终止 PID 并等待退出，SIGKILL 兜底
stop_pid() {
  local pid="$1"
  local name="$2"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
    echo "  $name 未在运行 (PID ${pid:-unknown})"
    return 0
  fi
  echo "  停止 $name (PID $pid)..."
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 20); do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 0.5
  done
  echo "  $name 未响应 SIGTERM，发送 SIGKILL"
  kill -9 "$pid" 2>/dev/null || true
}

if [[ "$MODE" == "daemon" ]]; then
  # 停止生产服务（deploy.sh --daemon 启动的实例）
  PID_FILE="$BUILD_DIR/openshare.pid"
  if [[ -f "$PID_FILE" ]]; then
    OLD_PID="$(cat "$PID_FILE" 2>/dev/null || true)"
    stop_pid "$OLD_PID" "生产服务"
    rm -f "$PID_FILE"
  else
    echo "未找到 PID 文件 $PID_FILE（生产服务未通过 deploy.sh 启动？）"
  fi
  exit 0
fi

# 停止开发服务（start_back.sh 落了 PID 文件）
echo "==> 停止开发服务"
for name in backend frontend; do
  PID_FILE="$DEV_DATA_DIR/$name.pid"
  if [[ -f "$PID_FILE" ]]; then
    stop_pid "$(cat "$PID_FILE" 2>/dev/null || true)" "开发 $name"
    rm -f "$PID_FILE"
  fi
done

# start.sh 前台模式不落 PID 文件：按端口兜底清理残留的开发进程
DEV_BACKEND_PORT="${BACKEND_PORT:-8080}"
if command -v lsof >/dev/null 2>&1; then
  for port in "$DEV_BACKEND_PORT" 5173; do
    PIDS="$(lsof -ti :"$port" -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$PIDS" ]]; then
      for pid in $PIDS; do
        echo "  端口 :$port 仍有监听进程 (PID $pid)，停止之"
        stop_pid "$pid" "端口 :$port 进程"
      done
    fi
  done
else
  echo "警告: 未安装 lsof，无法按端口清理残留监听进程"
  echo "      请手动检查并结束占用 :${DEV_BACKEND_PORT} 与 :5173 的进程（如 \`ps aux | grep openshare-dev\`）"
fi

echo "==> 开发服务已停止"
