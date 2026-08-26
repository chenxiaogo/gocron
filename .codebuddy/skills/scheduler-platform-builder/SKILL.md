---
name: scheduler-platform-builder
description: 指导开发企业级任务调度与执行平台的技能。当用户需要构建/重构任务调度平台的首页 Dashboard、任务管理、执行管理、节点监控、登录鉴权等页面时使用。本技能裁剪了 scheduler-platform-spec 的核心规范，聚焦"首页需求 + 美观页面 + 功能完整增强"，项目结构参考 go-proxy（前后端分离：server/ + web/），功能与交互参考 goscheduler（并做企业级增强：GORM 持久化、SQLite 默认/MySQL 可选、bcrypt+JWT 鉴权、robfig/cron 调度、重试/超时/并发）。适用于 Vue 3 + Element Plus 前端与 Gin + GORM 后端的调度平台开发场景。
---

# 企业级调度平台构建技能

## Overview

本技能用于构建"企业级任务调度与执行平台"，核心聚焦首页 Dashboard 需求，同时覆盖任务、执行、节点、登录鉴权等完整模块。页面必须美观、现代、专业。项目结构严格参考 `go-proxy`（前后端分离、分层清晰），功能交互参考 `goscheduler`（任务/节点/执行/用户），并做**企业级增强**：数据持久化采用 **GORM**（默认 SQLite 纯 Go 驱动，可切换 MySQL），密码 bcrypt + JWT 鉴权，robfig/cron 真实调度，Task 与 Execution 解耦。**先读本文件再动手，代码生成前务必阅读 `references/` 目录下的参考文档。**

> **当前项目落地状态（本次目录调整后）**：`/home/chenguohong/go/src/gocron` 即为该技能构建出的调度平台实例。模块名 `scheduler-platform`，后端全部代码在 `server/` 下，前端在 `web/` 下。若更新项目或本技能，请以实际代码为准。

## 核心原则

1. **首页优先**：首页 Dashboard 是核心交付物，必须"一屏可见核心指标、信息密度高、视觉精致"。
2. **页面要好看**：遵循现代 Dashboard 设计规范——卡片化、渐变指标卡、趋势图、状态 Tag、空态/加载态/错误态完整。
3. **结构参考 go-proxy**：后端严格分层（Router → Middleware → Handler → Service → Store → GORM），前端目录规范统一。**前后端分离：Go 代码统一放 `server/`，Vue 代码放 `web/`。**
4. **功能参考 goscheduler 并增强**：保留核心能力（任务 CRUD/启停/手动执行、执行记录、节点管理、登录鉴权），并增强重试/超时/并发/真实调度。
5. **数据持久化必须用 GORM**：所有业务数据落库（禁止用内存 store 代替真实数据库），GORM AutoMigrate 建表，Store 层封装查询，默认 SQLite（纯 Go 驱动 `glebarez/sqlite`，`CGO_ENABLED=0`），可通过配置切换 MySQL。
6. **Task 与 Execution 必须解耦**：Task 是配置，Execution 是一次执行，禁止用 Task 状态代替历史执行记录。
7. **增强但不重复造轮子**：Cron 调度用 robfig/cron（标准 5 字段 parser），密码用 bcrypt，token 用 JWT，日志用 zap。

## 裁剪后的首页需求（P0 范围）

首页 Dashboard 必须包含以下模块，按优先级排列：

### 1. 顶部指标卡（Stats Cards）
- 任务总数 / 启用任务 / 暂停任务
- 今日执行次数 / 今日成功 / 今日失败
- 今日成功率 / 执行中任务 / 异常任务
- 在线节点 / 离线节点

设计：**渐变背景指标卡**（不同颜色区分业务语义），大数字 + 描述 + 环比/趋势小箭头，鼠标悬浮有轻微上浮阴影动画。

### 2. 执行趋势图（Trend Chart）
- 支持时间范围切换：今日 / 近 7 天 / 近 30 天
- 折线图展示：执行次数、成功次数、失败次数、超时次数
- 使用 ECharts，配色与主题统一

### 3. 节点资源概览（Node Resources）
- CPU / Memory / Disk / Load 实时概览
- 每个节点一个卡片，显示资源进度条 + 在线状态 Tag

### 4. 异常任务列表（Failed Tasks）
- 字段：任务名称、最近状态、连续失败次数、最后执行时间、最后错误、操作（查看详情/查看日志/立即重试/暂停任务）
- 失败行红色高亮，支持一键跳转执行详情

### 5. 最近执行记录（Recent Executions）
- 字段：任务、类型、执行时间、耗时、节点、状态
- 状态用彩色 Tag 展示（成功/失败/超时/执行中/重试中）

### 6. 即将执行任务（Upcoming Tasks）
- 字段：任务、下次执行时间、倒计时、执行节点

## API 规范（实际路由，见 `server/pkg/app/route/route.go`）

```
# 公开（无需登录）
GET  /api/health              # 健康检查
POST /api/auth/login          # 登录（bcrypt 校验，返回 JWT + user）

# 受保护（需 Authorization: Bearer <token>）
GET  /api/auth/me             # 当前用户（从 JWT 解析）

# Dashboard（受保护）
GET  /api/dashboard/overview     # 指标卡数据
GET  /api/dashboard/trend        # 执行趋势（支持?range=day|7d|30d）
GET  /api/dashboard/failures     # 异常任务
GET  /api/dashboard/recent       # 最近执行
GET  /api/dashboard/upcoming     # 即将执行
GET  /api/dashboard/resources    # 节点资源

# Task（受保护）
GET    /api/tasks               # 列表（?keyword&type&status）
POST   /api/tasks               # 创建
GET    /api/tasks/:id           # 详情
PUT    /api/tasks/:id           # 更新
DELETE /api/tasks/:id           # 删除
POST   /api/tasks/:id/run       # 手动执行（生成 execution）
POST   /api/tasks/:id/enable    # 启用
POST   /api/tasks/:id/disable   # 停用

# Execution（受保护）
GET  /api/executions            # 执行记录列表
GET  /api/executions/:id        # 执行详情
GET  /api/executions/:id/logs   # 执行日志

# Node（受保护）
GET  /api/nodes                 # 节点列表
GET  /api/nodes/:id             # 节点详情
POST /api/nodes                 # 新增节点（需 node:manage）
PUT  /api/nodes/:id             # 更新节点（需 node:manage）
DELETE /api/nodes/:id           # 删除节点（需 node:manage）

# User（受保护，需 user:manage）
GET  /api/users                 # 用户列表
POST /api/users                 # 创建用户
PUT  /api/users/:id             # 更新用户（角色/状态/管理员标记）
PUT  /api/users/:id/password    # 重置密码
DELETE /api/users/:id           # 删除用户

# Role（受保护，需 role:manage）
GET  /api/roles                 # 角色列表（含权限点）
GET  /api/roles/permissions     # 权限点列表（所有登录用户可读，供授权表单）
GET  /api/roles/:id             # 角色详情
POST /api/roles                 # 创建角色
PUT  /api/roles/:id             # 更新角色
PUT  /api/roles/:id/perms       # 设置角色权限点
DELETE /api/roles/:id           # 删除角色（内置角色不可删）
```

### 鉴权（JWT）
- 登录接口 `POST /api/auth/login`（body: `{"username","password"}`），密码 bcrypt 校验，成功返回 `{code:0, token, user:{uid,username,nickname,isAdmin}}`。
- **注意**：`AuthHandler.RegisterRoute` 不要套 `/auth` 子分组（`route.go` 已传 `/api/auth` 分组，否则路径变成 `/api/auth/auth/login`）。
- 受保护分组挂 `middleware.Auth`，解析 `Authorization: Bearer <token>`（兼容 `Token` 头），用户信息写入 gin context。
- 前端 `web/src/api/request.js` 统一附带 Bearer 头。

Dashboard 禁止实时扫描 `task_execution` 大表做全表聚合，用 `CountExecutions`/`ExecutionsByStatus` 等 COUNT/GROUP BY 查询。

## 项目结构（参考 go-proxy，前后端分离）

项目采用**前后端分离目录**：所有 Go 后端代码统一放在 `server/` 目录下，前端放在 `web/` 目录下。**模块名 `scheduler-platform`（import 基于模块名而非目录路径）。**

> 下方为**当前落地实例**的真实目录结构（`/home/chenguohong/go/src/gocron`），后续开发/重构以此为准。

```
scheduler-platform/
├── Makefile            # 构建后端/前端/启动（CGO_ENABLED=0）
├── README.md           # 项目说明（登录账号 admin/admin123）
├── server/             # 后端（全部 Go 代码 + go.mod + conf.d + logs）
│   ├── cmd/
│   │   └── main.go     # 入口，shutdown.Hook + app.NewServer
│   ├── conf.d/
│   │   └── config.yaml # 配置（sqlite 默认，可切 mysql）
│   ├── logs/           # 日志目录（运行时 CWD 为 server/）
│   ├── internal/
│   │   ├── handler/    # Handler 层（enter.go 聚合 + auth/dashboard/execution/node/task/user/role）
│   │   ├── middleware/ # 中间件（JWT 鉴权 + RequirePerm 权限校验 auth.go）
│   │   ├── service/    # 业务逻辑层（Dashboard 聚合等）
│   │   ├── store/      # 数据仓库层（GORM，初始化 admin + RBAC；含 rbac.go 角色/权限/节点绑定）
│   │   ├── scheduler/  # 调度器（robfig/cron，标准 5 字段 parser，触发后交执行器）
│   │   ├── executor/   # 执行器（按 HTTP/Shell/Cron/GRPC 类型真正执行并回写结果/日志）
│   │   └── model/      # 数据模型（Task/Execution/TaskLog/Node/User/TaskNode/Role/Permission，GORM tag）
│   ├── pkg/
│   │   ├── app/        # 应用装配
│   │   │   ├── server.go  # 启动/优雅停止/AutoMigrate
│   │   │   ├── static.go  # 前端静态 SPA 托管（../web/dist）
│   │   │   ├── config/    # 配置加载（viper + yaml + 热更新 + EnsureConfigFile）
│   │   │   └── route/     # 路由注册（Registrar 接口 + JWT 保护分组）
│   │   ├── auth/       # JWT 签发/解析（HS256，golang-jwt/v5，Claims 含 roleId/roleName/perms）
│   │   ├── rbac/       # RBAC 权限点清单（AllPermissions，供初始化与 admin 全权限）
│   │   ├── database/   # GORM 连接（SQLite 默认 / MySQL 可选）
│   │   ├── ecode/      # 错误码（中英文多语言，Code 接口）
│   │   ├── ilog/       # 结构化日志（zap + lumberjack）
│   │   └── shutdown/   # 优雅退出
│   ├── go.mod          # 模块名 scheduler-platform
│   └── go.sum
└── web/                # 前端（Vue3 + Element Plus + ECharts + Pinia）
    └── src/
        ├── api/           # 接口封装（request.js 带 Bearer 头 + auth/dashboard/execution/node/task/user/role）
        ├── components/    # StatCard / StatusTag
        ├── directives/    # perm.js（v-perm 按钮权限指令）
        ├── layouts/       # MainLayout（侧边栏 + 顶栏 + 主内容区，菜单按权限过滤）
        ├── router/        # hash 路由 + 登录/权限守卫
        ├── stores/        # Pinia user.js（含 hasPerm/perms/roleName）
        ├── styles/        # index.css
        └── views/
            ├── login/      # 登录页
            ├── dashboard/  # 首页 Dashboard
            ├── task/       # 任务管理（表单含节点绑定 + 按类型动态展示执行参数）
            ├── execution/  # 执行管理
            ├── node/       # 节点管理（管理员可增删改）
            ├── user/       # 用户管理（UserList.vue，分配角色/重置密码）
            └── role/       # 角色管理（RoleList.vue，授权权限点）
```

后端全部 Go 代码（含 `go.mod`/`go.sum`）都必须在 `server/` 目录下，项目根目录不放置任何 Go 源码文件。前端构建产物在 `web/dist/`，由 `server/pkg/app/static.go` 以 `../web/dist` 相对路径托管（运行时 CWD 为 `server/`）。

## 前端设计规范（页面要好看）

### 视觉基调
- 主色：现代科技蓝 `#409EFF`（Element Plus 默认），成功绿 `#67C23A`、警告橙 `#E6A23C`、危险红 `#F56C6C`
- 背景：浅灰 `#f0f2f5`，卡片纯白，圆角 `8px`，卡片阴影 `0 2px 12px rgba(0,0,0,0.06)`
- 深色侧边栏：`#1f2d3d` 或 `#001529`，菜单激活态高亮蓝条

### 指标卡设计
- 左侧渐变图标块 + 右侧数据文本
- 示例配色：任务总数 `#409EFF`，成功 `#67C23A`，失败 `#F56C6C`，在线节点 `#13ce66`
- 大数字 `font-size: 28px; font-weight: 700`，标签 `color: #909399`
- 悬浮效果：`transform: translateY(-4px)` + 阴影加深过渡

### 组件规范
- Loading / Empty / Error 三态必须完整
- 表格统一 StatusTag 组件展示状态
- 高风险操作（删除任务/停止任务/执行 Shell）必须二次确认
- 分页、搜索、筛选、批量操作齐全

## 后端分层规范（go-proxy 风格）

```
Router
  -> Middleware
    -> Handler
      -> Service
        -> Store (Repository)
          -> GORM
            -> SQLite / MySQL
```

所有 Go 源码（含 `go.mod`）统一放在 `server/` 目录下（`server/cmd`、`server/pkg`、`server/internal`），项目根目录不放置 Go 代码。

- **Handler** 只做：参数解析、参数校验、调用 Service/Store、返回统一响应
- **Service** 负责：业务规则、状态转换、Dashboard 聚合、排序/计数
- **Store**（数据仓库层，替代 Repository）负责：所有 GORM 数据访问、建表后初始化 admin 账号、聚合查询
- **禁止**为了快速实现功能直接绕过 Service/Store 层操作 `*gorm.DB`

## 数据持久化（GORM，SQLite 默认 / MySQL 可选）

**必须使用 GORM 做持久化**，禁止用内存 store 代替真实数据库。默认 **SQLite**（纯 Go 驱动 `glebarez/sqlite`，`CGO_ENABLED=0` 可跨平台编译），可通过配置切换 **MySQL**。

### 连接与建表
- `server/conf.d/config.yaml` 中配置 `database.driver`（`sqlite`/`mysql`）与 `database.dsn`。
- `server/pkg/database/database.go` 按 driver 选择 dialector：`sqlite.Open(dsn)` 或 `mysql.Open(dsn)`，并设置连接池。
- 启动时 `server/pkg/app/server.go` 调 `database.New(&conf.Database)` + `db.AutoMigrate(&model.Task{}, &model.Execution{}, &model.TaskLog{}, &model.Node{}, &model.User{}, &model.TaskNode{}, &model.Role{}, &model.Permission{}, &model.RolePermission{}, &model.UserRole{})` 建表。
- 依赖：`gorm.io/gorm` + `github.com/glebarez/sqlite`（纯 Go）+ `gorm.io/driver/mysql`（可选）。

### 模型规范（GORM tag）
```go
type Task struct {
    ID             int64     `gorm:"primaryKey"`
    Code           string    `gorm:"uniqueIndex;size:64"`
    Name           string    `gorm:"size:128;index"`
    Type           string    `gorm:"size:16;index"` // cron/http/shell/grpc
    Status         int       `gorm:"index"`         // 1 enabled 0 disabled
    CronExpr       string    `gorm:"size:64"`
    Timeout        int       // 秒
    RetryCount     int
    NextRunTime    time.Time
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

### Store 层（数据仓库）
```go
type Store struct{ db *gorm.DB }
func New(db *gorm.DB) *Store { s := &Store{db: db}; s.ensureAdmin(); return s } // 仅初始化 admin 账号
func (s *Store) ListTasks() []*model.Task
func (s *Store) GetTask(id int64) (*model.Task, bool)
func (s *Store) CreateTask(t *model.Task) *model.Task
func (s *Store) UpdateTask(t *model.Task) (*model.Task, bool)
func (s *Store) DeleteTask(id int64) bool
func (s *Store) ListExecutions(limit int) []*model.Execution
func (s *Store) CreateExecution(e *model.Execution) *model.Execution
func (s *Store) CountExecutions(where string, args ...interface{}) int64
func (s *Store) ExecutionsByStatus() map[string]int64
```
- Dashboard 聚合查询用 `COUNT`/`GROUP BY`（`CountExecutions`/`ExecutionsByStatus`），避免实时扫全表。
- 空库时仅 `ensureAdmin()` 初始化 admin 账号（bcrypt，密码 `admin123`）；**不再写演示/假数据**，页面数据全部来自真实任务创建与执行。批量初始化（admin、节点等）见 `server/conf.d/init.sql`（DML，表结构由 GORM AutoMigrate 同步）。

### 事务
- 需要原子性的操作（如执行完成后同时更新 execution + task）用 `db.Transaction` 包裹。
- Store 暴露 `DB()` 返回 `*gorm.DB` 供调度器/事务使用。

## 核心数据模型（当前落地）

所有模型在 `server/internal/model/model.go` 中，均带 GORM tag：

- `Task`：任务配置（id/code/name/type/description/group/tag/environment/status/cronExpr/timeout/concurrency/retryCount/retryInterval/ownerId/owner/executorConfig/nextRunTime/createdBy），Task 与 Execution 解耦；`NodeIDs/Nodes`（gorm:"-"）为任务绑定的节点（多对多 `TaskNode` 表），创建/更新时由 Store 落库关联
- `TaskNode`：任务-节点多对多关联表（taskId+nodeId 唯一索引）；任务执行时 `executor` 解析绑定节点写入 Execution 的 WorkerID/WorkerName（未绑定回退 `worker-01`）
- `Execution`：一次执行（id/taskId/taskName/taskType/triggerType/workerId/workerName/status/attempt/startedAt/finishedAt/durationMs/errorCode/errorMessage/retryCount），生命周期 `pending/running/success/failed/timeout/canceled/retrying`
- `TaskLog`：执行日志（executionId/taskId/stream/level/content/sequence）
- `Node`：调度节点（nodeId/name/address/os/version/status/cpuUsage/memoryUsage/diskUsage/loadAverage/runningTasks/maxConcurrency/lastHeartbeat），状态 `online/offline/busy/warning/error`
- `User`：用户（username/password[bcrypt]/nickname/email/isAdmin/roleId/status）
- `Role`：角色（code/name/description/builtIn/perms）；`Permission`：权限点（code/name/type/description）；`RolePermission`/`UserRole`：关联表

索引建议：`execution`：(task_id, status)、(started_at)、(trigger_type)；`task_log`：(execution_id)；`node`：node_id 唯一索引；`task`：code 唯一索引。

## 任务类型与调度

P0 任务类型：Cron、HTTP、Shell、gRPC（`Task.Type` 区分，`ExecutorConfig` 存执行参数 JSON）。

### 调度（robfig/cron，标准 5 字段 parser）
- 使用 `github.com/robfig/cron/v3`。
- **必须用标准 5 字段 parser**：`cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)`，**不要用 `cron.WithSeconds()`**（6 字段会与 5 字段 cron 表达式冲突，导致 job 注册失败）。
- `Scheduler` 维护 `entryTask`/`taskEntry` 映射，支持任务启停时 `SyncTask` 增删 cron。
- 任务变更（Create/Update/Enable/Disable/Delete）后通过 `TaskHandler.syncScheduler` 同步调度器。
- 调度触发只负责挑选任务并交给执行器，不直接写执行记录。

### 执行器（internal/executor）
- `executor.New(store)` 创建真正按任务类型执行的执行器，由 `Scheduler` 与 `TaskHandler.Run` 共用。
- 执行流程（`Run(task)` / `Trigger(task)`，均异步 goroutine）：
  1. 解析 `Task.ExecutorConfig`（JSON：http/grpc 的 url/method/body/headers，shell 的 command）。
  2. 解析任务绑定节点（`store.GetTaskNodes`），写入 Execution 的 `WorkerID/WorkerName`（未绑定回退 `worker-01`）。
  3. `CreateExecution` 写 `running` 记录（含 `TriggerType=cron/manual`、`Attempt`）。
  4. 按类型执行并回写 `Execution`：`success/failed/timeout` + `FinishedAt`/`DurationMs`/`ErrorMessage`。
  5. `AppendLog` 写 `TaskLog`（http 响应、shell stdout/stderr）。
  6. 失败且 `RetryCount>0` 时按 `RetryInterval` 重试，每次尝试独立 Execution 记录。
- 类型实现：
  - `http` / `grpc`：`http.Client` 发请求，`task.Timeout` 控制超时，`>=400` 判失败。
  - `shell`：`os/exec` 执行 `sh -c command`，捕获 stdout/stderr。
  - `cron`：纯周期触发，无 action，标记成功。
- 手动执行：`TaskHandler.Run` 调 `executor.Run(task)` 返回"任务已开始执行"。

## goscheduler 功能全量搬迁清单

以下 goscheduler 功能**必须全部搬迁**到调度平台（并做增强）：

| 模块 | goscheduler 功能 | 增强点 |
|---|---|---|
| 任务管理 | 列表搜索、创建/编辑、启停开关、手动执行（二次确认）、删除、展开详情、标签 | 增加类型 Cron/gRPC、分组、负责人、批量操作、任务版本回滚 |
| 任务依赖 | 父任务/子任务、强依赖/弱依赖、DependencyTaskId 逗号分隔 | 升级为 DAG 拓扑（success/complete/failure 依赖）、可视化 |
| 通知 | NotifyStatus/NotifyType/NotifyReceiverId/NotifyKeyword，email/slack/webhook 渠道 | 抽成告警中心 + 统一通知渠道，支持钉钉/飞书/企业微信 |
| 节点（主机） | host 增删改查、任务-主机多对多绑定（task_host） | 增加资源监控（CPU/Mem/Disk/Load）、在线心跳、告警；**任务-节点多对多绑定（TaskNode）已落地，执行时调度到绑定节点** |
| 执行日志 | 按任务/节点查询、查看执行历史 | 增加实时滚动、搜索、下载、stdout/stderr 分离、脱敏 |
| 用户 | 登录、用户 CRUD、改密（盐值加密）、isAdmin | 升级 bcrypt + JWT，增强为 RBAC（角色/权限点）；**用户管理/角色管理页面已落地** |
| 登录日志 | login_log 记录登录 | 保留并增强（记录 IP/UA/结果） |
| 系统设置 | email/slack/webhook 通知配置 | 保留并扩展渠道 |
| 安装引导 | 首次安装初始化 install/index.vue | 保留，首启初始化 admin + 建库 |

## 企业级增强功能模块

### 1. 告警中心（Alert）
- 告警类型：任务失败、连续失败、任务超时、节点离线、CPU/Mem/Disk 高、Scheduler 异常。
- 规则可配置阈值（如 CPU>80%、连续失败>3），产生 `alert_record`。
- 通知渠道：Webhook / Email / Slack / 钉钉 / 飞书 / 企业微信，统一 `Notifier` 接口。

### 2. DAG 任务依赖
- 上游/下游任务，支持 success / complete / failure 三种依赖语义。
- 前端使用可视化拓扑（可参考 ECharts graph 或 DAG 画布）。

### 3. RBAC 权限（已落地）
- 权限点（`server/pkg/rbac/rbac.go` 的 `AllPermissions` + `model` 常量）：
  `task:view/create/update/delete/run`、`exec:view`、`log:view`、`node:view/manage`、`user:manage`、`role:manage`、`system:admin`。
- 数据模型：`Role`、`Permission`、`RolePermission`、`UserRole`（关联表），`User` 增加 `RoleID`。
- 内置角色（`store.InitRBAC` 幂等初始化）：`admin`（全部权限）、`operator`（任务管理+查看）、`viewer`（只读）。
- JWT Claims 携带 `roleId/roleName/perms`（`pkg/auth/jwt.go`），登录返回 `user.roleId/roleName/perms`。
- 后端校验：`middleware.RequirePerm(perm)`，admin（isAdmin=true）恒放行；用户路由按权限点挂载。
- 前端控制：`v-perm` 指令控制按钮，菜单按 `hasPerm` 过滤，路由 `meta.perm` 守卫，`stores/user.js` 的 `hasPerm` getter。
- 用户管理 `UserList.vue`、角色管理 `RoleList.vue` 完整 CRUD；至少保留一名管理员。

### 4. 操作审计（Audit）
- 记录：用户、时间、IP、操作、对象、请求参数、结果。
- 敏感参数（密码/token/请求体密钥）脱敏。

### 5. 任务版本（Version）
- 每次任务配置修改生成版本 v1→v2→v3，记录修改人/时间/变更字段/前后内容。
- 支持查看、Diff、回滚。

### 6. 通知渠道（Notification）
- 统一 `Notifier` 接口 + 工厂，按 `setting` 表配置各渠道（email/slack/webhook/钉钉/飞书）。

## 构建/运行（Makefile）

```bash
make build        # 构建后端二进制 scheduler-server（cd server && CGO_ENABLED=0 go build -o ../scheduler-server ./cmd）
make web-install  # 前端安装依赖（cd web && npm install）
make web-build    # 前端构建产物（cd web && npm run build -> web/dist）
make all          # 后端 + 前端
make run          # 启动服务（cd server && ../scheduler-server）
```

- 构建时 **`CGO_ENABLED=0`** 以支持纯 Go SQLite 驱动、跨平台编译。
- 离线构建可用 `GOPROXY=file://<本地 module cache>` 代理。
- 启动后访问 `http://localhost:8080`，登录账号 `admin` / `admin123`。
- 前端静态产物由 `server/pkg/app/static.go` 以 `../web/dist` 托管（运行时 CWD 为 `server/`）。

## 开发工作流

1. **阅读参考文档**：先读 `references/project-structure.md`（go-proxy 结构）、`references/goscheduler-functional.md`（goscheduler 功能交互）、`references/gorm-mysql-persistence.md`（GORM 持久化）、`references/enhanced-features.md`（企业级增强功能）。
2. **确认首页需求**：明确当前要实现的首页模块（指标卡/趋势图/资源/异常任务等）。
3. **后端先行**：按 go-proxy 分层 + GORM（SQLite 默认/MySQL 可选）持久化实现 API，注意 Dashboard 聚合。
4. **构建验证**：`make build` + 运行时验证（登录、受保护 API、Dashboard 各端点、任务启停、cron 触发）。
5. **前端实现**：按首页设计规范构建美观页面，三态完整。
6. **验收检查**：核心指标一屏可见、失败任务可从首页直达执行详情、图表加载流畅、数据落库（重启后仍在）。

### 已运行时验证的关键修复（务必遵守，避免踩坑）
1. **cron 用标准 5 字段 parser**：`cron.NewParser(cron.Minute|Hour|Dom|Month|Dow)`，**不要用 `cron.WithSeconds()`**（6 字段与 5 字段 cron 冲突，导致 0 个 job 注册）。
2. **AuthHandler 不要套 `/auth` 子分组**：`route.go` 已传 `/api/auth` 分组，否则登录路径变成 `/api/auth/auth/login`。
3. **Run 处理器对 `CreateExecution` 返回值加 nil 判空**：避免空指针 panic 返回 500。
4. **默认存储用 SQLite**（纯 Go 驱动 `glebarez/sqlite`，`CGO_ENABLED=0`），MySQL 通过配置切换。

## 验收标准（完成定义）

一个页面/功能只有在同时满足以下才算完成：
- 产品行为明确（页面展示什么、交互是什么）
- API 明确（请求/响应结构清晰）
- 数据模型明确
- 前端交互明确（Loading/Empty/Error/权限完整）
- 异常场景明确
- 视觉美观（卡片化、渐变、Tag、动画一致）

## Resources

本技能包含以下参考文档，动手编码前必须阅读：

### references/
- `project-structure.md`：go-proxy 项目结构详解（后端分层、配置加载、优雅退出、错误码、路由注册）。
- `goscheduler-functional.md`：goscheduler 功能与交互参考（数据模型、任务管理、执行日志、节点管理、用户权限、登录流程、全量搬迁清单）。
- `gorm-mysql-persistence.md`：GORM 数据持久化规范（连接、模型 tag、Repository/Store、事务、索引、迁移）。
- `enhanced-features.md`：企业级增强功能参考（重试、超时、DAG、告警、RBAC、审计、任务版本、通知渠道、节点监控）。

### scripts/
- `create-project-scaffold.sh`：一键生成调度平台脚手架脚本（后端 `server/cmd/pkg/internal` 目录 + 前端 `web/` Vue3 目录，前后端分离，含 SQLite/MySQL 配置与 GORM 示例）。

### assets/
本技能当前不含静态资产；如需品牌化视觉资源，按需补充至 `assets/` 目录。
