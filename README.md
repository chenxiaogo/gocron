# 企业级任务调度与执行平台

基于 `scheduler-platform-builder` 技能构建的任务调度平台。后端参考 `go-proxy` 分层结构，功能交互参考 `goscheduler`，聚焦美观的首页 Dashboard。项目采用**前后端分离**结构，Go 后端代码统一放在 `server/` 目录，前端放在 `web/` 目录。

## 技术栈

- **后端**：Go 1.25 + Gin + Viper + Zap + GORM，分层结构（Router → Middleware → Handler → Service → Store → GORM），位于 `server/`
- **前端**：Vue 3 + Vite + Element Plus + Pinia + Vue Router + ECharts，位于 `web/`
- **数据**：GORM 持久化，默认 **SQLite**（纯 Go 驱动，无需安装），可通过配置切换 **MySQL**
- **鉴权**：bcrypt 密码哈希 + JWT（HS256）
- **调度**：robfig/cron 真实调度（标准 5 字段）

## 项目结构

```
scheduler-platform/
├── Makefile                     # 构建后端/前端/启动
├── README.md
├── server/                      # 后端（全部 Go 代码 + go.mod）
│   ├── cmd/
│   │   └── main.go              # 入口（shutdown.Hook 生命周期管理）
│   ├── conf.d/
│   │   └── config.yaml          # 配置（sqlite 默认 / mysql 可选）
│   ├── logs/                    # 日志目录
│   ├── internal/
│   │   ├── handler/             # Handler 层（enter.go 聚合入口）
│   │   ├── middleware/          # JWT 鉴权中间件
│   │   ├── service/             # 业务逻辑层（Dashboard 聚合）
│   │   ├── store/               # 数据仓库层（GORM + 种子数据）
│   │   ├── scheduler/           # 调度器（robfig/cron）
│   │   └── model/               # 数据模型
│   ├── pkg/
│   │   ├── app/                 # 应用装配（server.go / static.go / config / route）
│   │   ├── auth/                # JWT 签发/解析
│   │   ├── database/            # GORM 连接（SQLite/MySQL 切换）
│   │   ├── ecode/               # 错误码
│   │   ├── ilog/                # 结构化日志（zap）
│   │   └── shutdown/            # 优雅退出
│   └── go.mod                   # 模块名 scheduler-platform
└── web/                         # 前端 Vue3 工程
    └── src/
        ├── api/                 # 接口封装（带 Bearer 头）
        ├── components/          # StatCard / StatusTag
        ├── layouts/             # MainLayout
        ├── router/              # hash 路由 + 登录守卫
        ├── stores/              # Pinia user
        ├── styles/
        └── views/
            ├── login/           # 登录页
            ├── dashboard/       # 首页 Dashboard
            ├── task/            # 任务管理
            ├── execution/       # 执行管理
            └── node/            # 节点管理
```

## 快速开始

### 1. 构建后端（在 server/ 下）

```bash
make build
```

### 2. 构建前端

```bash
make web-install
make web-build
```

### 3. 一键构建前后端

```bash
make all
```

### 4. 启动服务

```bash
make run
# 或
cd server && ../scheduler-server
```

访问 http://localhost:8080

**登录账号**：`admin` / `admin123`

> 前端构建产物托管于 `web/dist/`，后端以 `../web/dist` 相对路径提供（运行时 CWD 为 `server/`）。默认使用 SQLite（`server/scheduler.db`），如需 MySQL 请修改 `server/conf.d/config.yaml`。

## 首页 Dashboard 功能

- 渐变指标卡：任务总数、今日执行、今日成功/失败、执行中、在线节点
- 执行趋势图：今日/近7天/近30天切换，展示执行/成功/失败/超时
- 节点资源概览：CPU / 内存 / 磁盘进度条 + 节点状态
- 异常任务列表：连续失败次数、最后错误、快速重试/暂停
- 最近执行记录：彩色状态 Tag、耗时展示
- 即将执行任务：倒计时展示

## 核心 API

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/auth/login | 登录（返回 JWT） |
| GET | /api/auth/me | 当前用户 |
| GET | /api/dashboard/overview | 首页指标卡 |
| GET | /api/dashboard/trend?range=7d | 执行趋势 |
| GET | /api/dashboard/failures | 异常任务 |
| GET | /api/dashboard/recent | 最近执行 |
| GET | /api/dashboard/upcoming | 即将执行 |
| GET | /api/dashboard/resources | 节点资源 |
| GET/POST | /api/tasks | 任务列表/创建 |
| PUT/DELETE | /api/tasks/:id | 更新/删除任务 |
| POST | /api/tasks/:id/run | 手动执行 |
| POST | /api/tasks/:id/enable /disable | 启停任务 |
| GET | /api/executions | 执行记录 |
| GET | /api/executions/:id/logs | 执行日志 |
| GET | /api/nodes | 节点列表 |

> 除登录外，其余接口需携带 `Authorization: Bearer <token>`。
