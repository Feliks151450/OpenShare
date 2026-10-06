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
DAEMON=true
SYNC_DB=false
for arg in "$@"; do
  case "$arg" in
    --separated)        MODE="separated" ;;
    --daemon)           DAEMON=true ;;
    --foreground)       DAEMON=false ;;
    --sync-db-from-dev) SYNC_DB=true ;;
  esac
done

PID_FILE="$BUILD_DIR/openshare.pid"
DEV_DB="$DEV_DATA_DIR/openshare.db"
PROD_DB="$DATA_DIR/openshare.db"

# ---- 先停旧进程：数据库操作与构建都必须在服务停止后进行，避免热拷贝/竞态 ----
stop_running_server() {
  if [[ -f "$PID_FILE" ]]; then
    OLD_PID="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
      echo "==> 停止旧进程 (PID $OLD_PID)..."
      kill "$OLD_PID" 2>/dev/null || true
      for _ in $(seq 1 20); do
        kill -0 "$OLD_PID" 2>/dev/null || break
        sleep 0.5
      done
      if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "  旧进程未响应 SIGTERM，发送 SIGKILL"
        kill -9 "$OLD_PID" 2>/dev/null || true
        sleep 1
      fi
    fi
    rm -f "$PID_FILE"
  fi
}
stop_running_server

mkdir -p "$DATA_DIR" "$BUILD_DIR"

# ---- 数据库同步（仅显式触发，绝不自动覆盖生产库）----
if $SYNC_DB; then
  if [[ ! -f "$DEV_DB" ]]; then
    echo "错误: 未找到开发数据库 $DEV_DB，无法同步" >&2
    exit 1
  fi
  echo "==> 显式同步开发数据库到生产环境"
  # 服务已停止；优先用 sqlite3 .backup（自动合并 WAL），不可用则连同 -wal/-shm 一起拷贝
  rm -f "$PROD_DB" "$PROD_DB-wal" "$PROD_DB-shm"
  if command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "$DEV_DB" ".backup '$PROD_DB'"
  else
    cp "$DEV_DB" "$PROD_DB"
    [[ -f "$DEV_DB-wal" ]] && cp "$DEV_DB-wal" "$PROD_DB-wal"
    [[ -f "$DEV_DB-shm" ]] && cp "$DEV_DB-shm" "$PROD_DB-shm"
  fi
  echo "==> 数据库同步完成"
else
  if [[ -f "$DEV_DB" ]]; then
    echo "==> 提示: 开发数据库不会自动同步到生产环境；如需同步请使用 ./deploy.sh --sync-db-from-dev"
  fi
  if [[ ! -f "$PROD_DB" ]]; then
    echo "==> 未检测到生产数据库，首次启动将自动初始化"
  fi
fi

echo "==> 构建前端"
cd "$ROOT_DIR/frontend"
npm ci --silent
npm run build

if [[ "$MODE" == "separated" ]]; then
  echo "==> 构建后端（前后端分离，不内嵌静态文件）"
  cd "$ROOT_DIR/backend"
  go build -tags noembed -o "$BUILD_DIR/openshare" ./cmd/server

  echo "==> 准备部署目录"
  rm -rf "$BUILD_DIR/static"
  mkdir -p "$BUILD_DIR/static"
  cp -a "$ROOT_DIR/frontend/dist/." "$BUILD_DIR/static/"
else
  echo "==> 构建后端（内嵌前端，单端口运行）"
  # embed.go 期望 dist/ 在同一目录下（backend/web/dist/）
  rm -rf "$ROOT_DIR/backend/web/dist"
  cp -a "$ROOT_DIR/frontend/dist" "$ROOT_DIR/backend/web/dist"
  cd "$ROOT_DIR/backend"
  go build -o "$BUILD_DIR/openshare" ./cmd/server
  # 构建完清理，但保留 .gitkeep 确保 start.sh 的 go run 不会报错
  rm -rf "$ROOT_DIR/backend/web/dist"
  mkdir -p "$ROOT_DIR/backend/web/dist"
  touch "$ROOT_DIR/backend/web/dist/.gitkeep"
fi

mkdir -p "$BUILD_DIR/configs"

cp "$ROOT_DIR/backend/configs/config.default.json" "$BUILD_DIR/configs/"

# ---- 会话密钥：持久化到数据目录，跨部署保持登录态 ----
SESSION_SECRET_FILE="$DATA_DIR/session_secret"
if [[ -s "$SESSION_SECRET_FILE" ]]; then
  SESSION_SECRET="$(tr -d ' \n' < "$SESSION_SECRET_FILE")"
fi
if [[ -z "${SESSION_SECRET:-}" ]]; then
  SESSION_SECRET="$(openssl rand -hex 32 2>/dev/null || od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  if [[ -z "$SESSION_SECRET" ]]; then
    echo "错误: 无法生成会话密钥（openssl 与 od 均不可用）" >&2
    exit 1
  fi
  (umask 077 && printf '%s\n' "$SESSION_SECRET" > "$SESSION_SECRET_FILE")
fi

cat > "$BUILD_DIR/configs/config.local.json" <<EOF
{
  "server": {
    "host": "0.0.0.0",
    "port": ${BACKEND_PORT}
  },
  "database": {
    "path": "$DATA_DIR/openshare.db"
  },
  "storage": {
    "root": "$DATA_DIR"
  },
  "session": {
    "secret": "$SESSION_SECRET"
  }
}
EOF

echo
echo "============================================"
if [[ "$MODE" == "separated" ]]; then
  echo "  模式: 前后端分离"
  echo "  静态文件: $BUILD_DIR/static/"
  echo "  API 后端: $BUILD_DIR/openshare (:$BACKEND_PORT)"
else
  echo "  模式: 内嵌（单端口 :$BACKEND_PORT)"
  echo "  二进制: $BUILD_DIR/openshare"
fi
echo "  数据目录: $DATA_DIR"
echo "============================================"

if $DAEMON; then
  mkdir -p "$LOG_DIR"
  LOG_FILE="$LOG_DIR/server.log"

  cd "$BUILD_DIR"
  nohup ./openshare >> "$LOG_FILE" 2>&1 &
  NEW_PID=$!
  echo $NEW_PID > "$PID_FILE"

  echo
  echo "  后台运行中 (PID $NEW_PID)"
  echo "  日志: $LOG_FILE"
  echo "  停止: kill \$(cat $PID_FILE)"
  echo "============================================"
else
  echo
  echo "  前台运行 (Ctrl+C 停止)"
  echo "============================================"
  echo
  cd "$BUILD_DIR"
  exec ./openshare
fi
