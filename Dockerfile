# ============================================================
# 调度平台 Dockerfile — 多阶段构建
# 阶段 1: 构建前端 (Vue 3 + Vite)
# 阶段 2: 构建后端 (Go 1.25, CGO_ENABLED=0)
# 阶段 3: 运行时 (Alpine, 最小镜像)
# ============================================================

# ---------- 阶段 1: 前端构建 ----------
FROM node:22-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm install --registry=https://registry.npmmirror.com
COPY web/ ./
RUN npm run build

# ---------- 阶段 2: 后端构建 ----------
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /scheduler-server ./cmd

# ---------- 阶段 3: 运行时 ----------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates

# 创建工作目录（CWD 需在 server/ 下，与本地运行一致）
WORKDIR /app/server

# 后端二进制
COPY --from=go-builder /scheduler-server ./

# 配置文件
COPY server/conf.d/ ./conf.d/

# 前端静态文件（相对路径 ../web/dist）
COPY --from=web-builder /src/web/dist /app/web/dist

# 日志目录
RUN mkdir -p /app/server/logs

EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/health || exit 1

ENTRYPOINT ["./scheduler-server"]