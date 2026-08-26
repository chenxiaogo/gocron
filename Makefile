# 调度平台 Makefile
# 前后端分离：server/ 为 Go 后端，web/ 为 Vue3 前端
APP_NAME := scheduler-server
SERVER_DIR := server
WEB_DIR := web
GO := go
DOCKER_COMPOSE := docker compose

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make build         构建后端二进制（server/ 下）"
	@echo "  make web-install   安装前端依赖"
	@echo "  make web-build     构建前端产物"
	@echo "  make run           启动服务"
	@echo "  make clean         清理产物"
	@echo ""
	@echo "  Docker 部署:"
	@echo "  make docker-build  构建 Docker 镜像"
	@echo "  make docker-up     启动容器（后台运行）"
	@echo "  make docker-down   停止并删除容器"
	@echo "  make docker-logs   查看容器日志"
	@echo "  make docker-clean  清理 Docker 资源"

.PHONY: build
# CGO_ENABLED=0 支持纯 Go SQLite 驱动，跨平台编译
build:
	cd $(SERVER_DIR) && CGO_ENABLED=0 $(GO) build -o ../$(APP_NAME) ./cmd

.PHONY: web-install
web-install:
	cd $(WEB_DIR) && npm install

.PHONY: web-build
web-build:
	cd $(WEB_DIR) && npm run build

.PHONY: all
all: build web-build

.PHONY: run
run:
	cd $(SERVER_DIR) && ../$(APP_NAME)

.PHONY: clean
clean:
	rm -f $(APP_NAME)
	rm -rf $(WEB_DIR)/dist

# ============================================================
# Docker 部署
# ============================================================

.PHONY: docker-build
docker-build:
	$(DOCKER_COMPOSE) build --no-cache

.PHONY: docker-up
docker-up:
	$(DOCKER_COMPOSE) up -d
	@echo "调度平台已启动: http://localhost:8080"
	@echo "默认账号: admin / admin123"

.PHONY: docker-down
docker-down:
	$(DOCKER_COMPOSE) down

.PHONY: docker-logs
docker-logs:
	$(DOCKER_COMPOSE) logs -f --tail=100

.PHONY: docker-restart
docker-restart: docker-down docker-up

.PHONY: docker-clean
docker-clean:
	$(DOCKER_COMPOSE) down -v --rmi local
	rm -f $(APP_NAME)
	rm -rf $(WEB_DIR)/dist
