# ============================================================
# 调度平台目标机运维 Makefile（离线部署包内）
# 常用：make up / make down / make logs / make ps / make restart
# ============================================================

COMPOSE := $(shell if docker compose version >/dev/null 2>&1; then echo "docker compose"; else echo "docker-compose"; fi)

# 适配旧版 docker daemon 的 API 版本上限
DOCKER_API_VER := $(shell docker version -f '{{.Server.APIVersion}}' 2>/dev/null)
ifneq ($(DOCKER_API_VER),)
export DOCKER_API_VERSION := $(DOCKER_API_VER)
endif

.PHONY: help
help:
	@echo "调度平台运维命令："
	@echo "  make up        启动 / 升级（后台）"
	@echo "  make down      停止并移除容器（数据保留）"
	@echo "  make restart   重启"
	@echo "  make ps        查看容器状态"
	@echo "  make logs      跟踪日志"
	@echo "  make reload    重新加载随包镜像并启动"
	@echo "  make clean     停止并删除容器（不删宿主机数据）"

.PHONY: up
up:
	$(COMPOSE) up -d
	$(COMPOSE) ps

.PHONY: down
down:
	$(COMPOSE) down

.PHONY: restart
restart:
	$(COMPOSE) restart

.PHONY: ps
ps:
	$(COMPOSE) ps

.PHONY: logs
logs:
	$(COMPOSE) logs -f --tail=200

.PHONY: reload
reload:
	@bash ./start.sh

.PHONY: clean
clean:
	$(COMPOSE) down
