# Multi-stage build: single static binary + embedded Vue UI. Runtime: Debian slim (glibc, CGO sqlite).
# Build multi-arch (Mac Docker + Linux): from repo root:
#   docker buildx build -f docker/Dockerfile -t openshare:latest --platform linux/amd64,linux/arm64 --push .

FROM node:22-bookworm AS frontend
WORKDIR /app
COPY frontend/package.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-bookworm AS backend
RUN apt-get update \
  && apt-get install -y --no-install-recommends gcc libc6-dev \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY backend/ ./backend/
COPY --from=frontend /app/dist ./backend/web/dist
WORKDIR /src/backend
ENV CGO_ENABLED=1
RUN go build -trimpath -ldflags="-s -w" -o /openshare ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --system --no-create-home --shell /usr/sbin/nologin openshare
WORKDIR /app
COPY --from=backend /openshare /app/openshare
COPY backend/configs/config.default.json /app/configs/config.default.json
COPY docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/openshare /app/entrypoint.sh \
  && mkdir -p /data/openshare \
  && chown openshare:openshare /data/openshare
USER openshare
EXPOSE 8080
VOLUME ["/data/openshare"]
ENV OPENSHARE_STORAGE_ROOT=/data/openshare \
    OPENSHARE_DATABASE_PATH=/data/openshare/openshare.db
ENTRYPOINT ["/app/entrypoint.sh"]
