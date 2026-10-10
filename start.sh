#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_DATA_DIR="$ROOT_DIR/.localdata"
LOG_DIR="$LOCAL_DATA_DIR/logs"
BACKEND_LOG="$LOG_DIR/backend.log"
FRONTEND_LOG="$LOG_DIR/frontend.log"
BACKEND_CONFIG_LOCAL="$ROOT_DIR/backend/configs/config.local.json"

BACKEND_PORT=8080
FRONTEND_PORT=5173

mkdir -p "$LOG_DIR"

# shellcheck source=scripts/lib.sh
source "$ROOT_DIR/scripts/lib.sh"
require_dev_dependencies

# 原占用检测替换为：
if check_port_in_use $BACKEND_PORT; then
  echo "错误: 后端端口 ${BACKEND_PORT} 已被占用，请先释放该端口"
  show_port_occupants $BACKEND_PORT
  exit 1
fi

if check_port_in_use $FRONTEND_PORT; then
  echo "错误: 前端端口 ${FRONTEND_PORT} 已被占用，请先释放该端口"
  show_port_occupants $FRONTEND_PORT
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

# go run 是包装器，直接 kill 它不会终止真实服务进程（golang/go#40467），
# 需要先用 exec 让 shell 进程本身成为后端，或以进程组为单位终止。
# 这里改用 go build + 直接运行二进制：BACKEND_PID 就是真实服务进程。
FRONTEND_PID=""
BACKEND_PID=""
cleanup() {
  local pid
  for pid in "$FRONTEND_PID" "$BACKEND_PID"; do
    if [ -n "$pid" ]; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  wait 2>/dev/null || true
}
# 在任何后台进程启动前注册，中途某步失败时也能清掉已启动的进程，避免孤儿
trap cleanup EXIT

echo "==> 安装前端依赖（npm install，首次或依赖变更时较慢）"
cd "$ROOT_DIR/frontend"
if ! npm install > "$FRONTEND_LOG" 2>&1; then
  echo "错误: npm install 失败，详见 $FRONTEND_LOG"
  tail -n 20 "$FRONTEND_LOG"
  exit 1
fi

echo "==> 启动前端开发服务器"
npm run dev -- --host 0.0.0.0 > "$FRONTEND_LOG" 2>&1 &
FRONTEND_PID=$!

cd "$ROOT_DIR/backend"

echo "==> 下载 Go 依赖（首次或 go.mod 变更时较慢）"
if ! go mod download 2>>"$BACKEND_LOG"; then
  echo "错误: Go 依赖下载失败，详见 $BACKEND_LOG"
  tail -n 20 "$BACKEND_LOG"
  exit 1
fi

echo "==> 编译后端"
# 确保 embed 所需的 dist/ 目录存在（至少包含 .gitkeep）
mkdir -p "$ROOT_DIR/backend/web/dist"
touch "$ROOT_DIR/backend/web/dist/.gitkeep"
# go build 产出真实二进制再运行：kill BACKEND_PID 即终止服务本身，
# 不会像 go run 那样留下孤儿服务进程占用 :8080
if ! go build -o "$LOCAL_DATA_DIR/openshare-dev" ./cmd/server 2>>"$BACKEND_LOG"; then
  echo "错误: 后端编译失败，详见 $BACKEND_LOG"
  tail -n 20 "$BACKEND_LOG"
  exit 1
fi

echo "==> 启动后端服务"
# 追加写入（>>）：超管密码只在数据库首次创建时打印一次，
# 截断日志会让重启后再也查不到初始凭据
"$LOCAL_DATA_DIR/openshare-dev" >> "$BACKEND_LOG" 2>&1 &
BACKEND_PID=$!

echo
echo "OpenShare 已启动"
echo "Public: http://localhost:5173/"
echo "Admin : http://localhost:5173/admin"
echo "Health: http://127.0.0.1:8080/healthz"
echo "Logs  : $LOG_DIR"
echo

attempts=30
echo "==> 等待后端就绪（最长 ${attempts} 秒）"
for ((i = 1; i <= attempts; i++)); do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "错误: 后端进程意外退出，详见 $BACKEND_LOG"
    tail -n 20 "$BACKEND_LOG"
    exit 1
  fi
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
  # 非首次启动不会打印凭据，用 healthz 确认就绪，避免白等
  if command -v curl >/dev/null 2>&1 && curl -sf "http://127.0.0.1:${BACKEND_PORT}/healthz" >/dev/null 2>&1; then
    echo "==> 后端已就绪"
    break
  fi
  sleep 1
done

if [[ -z "${line:-}" ]]; then
  echo "提示: 未找到超管初始凭据。凭据只在数据库首次创建时生成一次，"
  echo "      可搜索历史日志: grep '\\[bootstrap\\] super admin' $BACKEND_LOG"
  echo "      或删除 $LOCAL_DATA_DIR/openshare.db* 后重启以重新初始化（会清空数据）"
  echo
fi

echo "按 Ctrl+C 停止服务"

wait