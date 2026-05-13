#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$ROOT_DIR/build"

BACKEND_PORT="${BACKEND_PORT:-8080}"
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

if [[ -f "$DEV_DB" ]]; then
  if [[ -f "$PROD_DB" ]]; then
    DEV_TS=$(stat -f %m "$DEV_DB" 2>/dev/null || stat -c %Y "$DEV_DB" 2>/dev/null || echo 0)
    PROD_TS=$(stat -f %m "$PROD_DB" 2>/dev/null || stat -c %Y "$PROD_DB" 2>/dev/null || echo 0)
    if [[ "$DEV_TS" -gt "$PROD_TS" ]]; then
      echo "==> 开发库较新，同步到生产环境"
      cp "$DEV_DB" "$PROD_DB"
    else
      echo "==> 生产库已是最新，跳过同步"
    fi
  else
    echo "==> 生产库不存在，从开发环境同步"
    mkdir -p "$DATA_DIR"
    cp "$DEV_DB" "$PROD_DB"
  fi
elif [[ -f "$PROD_DB" ]]; then
  echo "==> 开发库不存在，使用现有生产库"
else
  echo "==> 未检测到已有数据库，首次运行将自动初始化"
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
mkdir -p "$DATA_DIR"

cp "$ROOT_DIR/backend/configs/config.default.json" "$BUILD_DIR/configs/"

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
    "secret": "$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p)"
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
  echo "  或加 --daemon 后台运行"
  echo "============================================"
  echo
  cd "$BUILD_DIR"
  exec ./openshare
fi
