# 调度平台离线部署包

- 应用：`@@PKG_NAME@@`
- 镜像：`@@IMAGE_NAME@@:@@IMAGE_TAG@@`（随包文件 `@@IMAGE_TAR@@`）
- 环境：`@@ENV_NAME@@`
- 生效配置：`@@ENV_CONFIG_FILE@@`

## 部署（Jenkins / 目标机）

```bash
mkdir -p /var/www/gocron
tar -xzf @@TARBALL@@ -C /var/www/gocron
cd /var/www/gocron
./start.sh
```

解压只覆盖部署文件；数据库与日志位于宿主机独立目录，升级不丢数据。

## 目录说明

| 文件 | 说明 |
| --- | --- |
| `@@IMAGE_TAR@@` | 离线镜像（`docker save` 产物，start.sh 自动 load） |
| `docker-compose.yml` | 容器编排 |
| `.env` | 端口 / 环境 / 数据目录等变量 |
| `conf.d/config.*.yaml` | local / test / prod 三套配置 |
| `scripts/import.*.yaml` | 各环境初始化口径清单 |
| `start.sh` / `stop.sh` | 启动 / 停止 |

## 常用运维

```bash
./start.sh          # 加载镜像并启动/升级
./stop.sh           # 停止并移除容器（数据保留）
make logs           # 跟踪日志
make ps             # 查看容器状态
```

## 默认账号

`admin / admin123`，请尽快修改密码，并在生效配置中更换 `security.jwtSecret`。

## 数据与日志

- 数据：宿主机 `/data/gocron/data`（容器内 `/data`，SQLite 文件 `scheduler.db`）
- 日志：宿主机 `/data/gocron/logs`（容器内 `/app/server/logs`）

可在 `.env` 中通过 `HOST_DATA_DIR` / `HOST_LOG_DIR` / `SCHEDULER_HTTP_PORT` 调整。
