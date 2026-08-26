# 企业级增强功能参考

本文件规范调度平台在 goscheduler 基础上新增的企业级增强能力：**执行解耦、重试、超时、DAG 依赖、告警中心、RBAC、操作审计、任务版本、通知渠道、节点监控**。

> 所有 Go 后端代码统一放 `server/` 目录下；所有业务数据走 GORM 持久化（默认 SQLite，可切 MySQL，见 `gorm-mysql-persistence.md`）。落地实例：`/home/chenguohong/go/src/gocron`。
>
> 当前落地项目已实现：**Task/Execution 解耦、JWT 鉴权、bcrypt、robfig/cron 真实调度（5 字段）、Dashboard 聚合、真实执行器（HTTP/Shell/Cron/GRPC）、仅初始化 admin（无演示数据）**。DAG/告警/RBAC/审计/任务版本等为可选增强蓝图（未在落地代码中），按需实现。

## 1. Task 与 Execution 解耦（核心）

- `Task` = 任务配置（可修改，可启停）；`Execution` = 一次执行记录（不可变，追加）。
- **禁止**用 Task 状态字段保存历史执行信息。
- 执行生命周期：`pending -> running -> success | failed | timeout | cancelled | retrying`。

```go
// server/internal/model/task_execution.go
type TaskExecution struct {
	ID           uint      `gorm:"primaryKey"`
	TaskID       uint      `gorm:"index"`
	TaskName     string    `gorm:"size:64"`
	Status       string    `gorm:"size:16;index"` // pending/running/success/failed/timeout/cancelled/retrying
	Trigger      string    `gorm:"size:16;index"` // cron/manual/api/dependency
	WorkerID     uint      `gorm:"index"`
	NodeName     string    `gorm:"size:64"`
	Attempt      int       `gorm:"default:1"`
	DurationMS   int64
	ErrorMessage string    `gorm:"type:text"`
	StartedAt    time.Time `gorm:"index"`
	FinishedAt   time.Time
	CreatedAt    time.Time
}
```

### Executor 统一接口
```go
// server/internal/executor/executor.go
type Executor interface {
	Validate(ctx context.Context, cfg TaskConfig) error
	Execute(ctx context.Context, exec *TaskExecution) Result
	Cancel(ctx context.Context, executionID uint) error
}
```

实现类型：`cron`（shell 命令按 Cron 调度）、`http`（GET/POST，含请求体/头/超时）、`shell`（系统命令）、`grpc`。

### 调度器（robfig/cron）
- 用 `github.com/robfig/cron/v3` 注册启用任务。
- 每次触发生成 `Execution`，交给执行逻辑处理。
- **必须用标准 5 字段 parser**（分 时 日 月 周），**不要用 `cron.WithSeconds()`**（6 字段与 5 字段 cron 表达式冲突，会导致 job 注册失败，见落地 `scheduler.go`）。

```go
c := cron.New(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow))
c.AddFunc(task.CronExpr, func() { /* 创建 execution 并派发 */ })
c.Start()
```

### 并发策略（增强）
| 策略 | 行为 |
|---|---|
| `allow` | 允许多实例并发执行 |
| `forbid` | 上一实例未结束则跳过本次（默认） |
| `queue` | 排队，上一实例结束再执行 |
| `overwrite` | 取消并覆盖上一次 |
| `max` | 限制最大并发数（`MaxConcurrent`） |

## 2. 超时控制

- 每个任务可配 `Timeout`（秒）。执行开始后启动定时器。
- 超时后：标记 execution `timeout`、调用 `Cancel` 终止执行、按重试策略处理、触发告警。

```go
ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
defer cancel()
// 执行器内部监听 ctx.Done() 实现超时中断
```

## 3. 重试策略

### 配置
- 是否启用、最大重试次数 `RetryMax`、重试间隔 `RetryInterval`、策略 `fixed`（固定）/`backoff`（指数退避）、最大退避、可重试错误类型。

### 可重试规则
- HTTP：5xx、408、429 可重试；4xx 默认不重试。
- gRPC：Unavailable / DeadlineExceeded / ResourceExhausted 可重试。
- Shell：非零退出码按 `RetryMax` 重试。

### 实现（server/internal/retry）
```go
type Policy struct {
	Enabled     bool
	MaxRetries  int
	Interval    time.Duration
	Strategy    string // fixed / backoff
	MaxBackoff  time.Duration
}

func (p *Policy) NextDelay(attempt int) time.Duration {
	if p.Strategy == "backoff" {
		d := p.Interval * time.Duration(1<<(attempt-1)) // 指数退避
		if p.MaxBackoff > 0 && d > p.MaxBackoff {
			return p.MaxBackoff
		}
		return d
	}
	return p.Interval
}

func (p *Policy) ShouldRetry(status string) bool {
	if !p.Enabled || status == "success" || status == "cancelled" {
		return false
	}
	return true
}
```

- 每次重试生成 `task_retry` 记录（execution_id/attempt/status/next_retry_at）。
- 前端执行详情展示 `Attempt 1 -> Failed / Attempt 2 -> Success` 轨迹。
- 后台协程轮询 `next_retry_at` 到期任务进行重试。

## 4. DAG 任务依赖（增强）

升级 goscheduler 的简单父子依赖为 DAG 拓扑：

```go
// server/internal/model/task_dependency.go
type TaskDependency struct {
	ID                 uint   `gorm:"primaryKey"`
	UpstreamTaskID     uint   `gorm:"index"`
	DownstreamTaskID   uint   `gorm:"index"`
	DependencyType     string `gorm:"size:16"` // success/complete/failure
}
```

依赖语义：
- `success`：上游全部成功，下游才执行。
- `complete`：上游执行完成（不论成败）即触发下游。
- `failure`：上游失败时触发下游（补偿/兜底）。

实现：
- 任务完成后，按 `task_dependency` 查下游，满足依赖语义则创建下游 execution。
- 检测环：添加依赖时用拓扑排序/DFS 检测循环依赖，禁止成环。
- 前端用 ECharts graph 或 DAG 画布展示依赖拓扑。

## 5. 告警中心（Alert）

### 告警类型
任务失败、连续失败（阈值）、任务超时、节点离线、CPU/Mem/Disk 高、Scheduler 异常。

### 规则与记录模型
```go
// server/internal/model/alert_rule.go
type AlertRule struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"size:64"`
	Type        string `gorm:"size:32;index"` // task_fail/consecutive_fail/timeout/node_offline/cpu/mem/disk/scheduler
	Target      string `gorm:"size:64"`       // 作用对象（task_id/node_id 或 *）
	Threshold   string `gorm:"size:64"`       // 如 "3"、"80%"
	Channels    string `gorm:"size:256"`      // 逗号分隔渠道
	Enabled     int8   `gorm:"default:1"`
	CreatedAt   time.Time
}

type AlertRecord struct {
	ID         uint      `gorm:"primaryKey"`
	RuleID     uint      `gorm:"index"`
	Level      string    `gorm:"size:16"` // info/warning/critical
	Title      string    `gorm:"size:128"`
	Content    string    `gorm:"type:text"`
	Status     int8      `gorm:"index;default:1"` // 1 未处理 2 已处理
	CreatedAt  time.Time `gorm:"index"`
}
```

### 告警引擎（server/internal/alert）
- 消费执行/节点事件，命中规则 → 生成 `alert_record` → 调用 `Notifier` 通知渠道。

## 6. 通知渠道（Notification）

统一 `Notifier` 接口 + 工厂：

```go
// server/internal/notification/notifier.go
type Notifier interface {
	Send(ctx context.Context, msg Message) error
}

func NewNotifier(typ string, cfg map[string]string) Notifier {
	switch typ {
	case "email":
		return &EmailNotifier{}
	case "slack":
		return &SlackNotifier{}
	case "webhook":
		return &WebhookNotifier{}
	case "dingtalk":
		return &DingTalkNotifier{}
	case "feishu":
		return &FeishuNotifier{}
	case "wecom":
		return &WeComNotifier{}
	}
	return nil
}
```

渠道配置存 `setting` 表（扩展 goscheduler 的 email/slack/webhook，新增钉钉/飞书/企业微信）。

## 7. RBAC 权限

### 模型
```go
type User       { ID, Username, Password(bcrypt), Email, Status, CreatedAt }
type Role       { ID, Name, Code, Description }
type Permission { ID, Name, Code }   // Code 如 "task:create"
type UserRole   { UserID, RoleID }   // 多对多
```

### 权限点清单
- `task:view/create/update/delete/run/retry/cancel`
- `execution:view`
- `log:view`
- `node:view/manage`
- `alert:manage`
- `audit:view`
- `system:manage`

### 实现
- admin 拥有全部权限；普通用户按角色授权。
- 后端中间件 `RequirePermission("task:create")` 校验 JWT 中的权限码。
- 前端菜单/按钮按权限点控制显隐（`v-permission` 指令或 `hasPermission` 方法）。

### 登录
- 密码 `bcrypt` 哈希；token 用 `JWT`（`golang-jwt/jwt`）。
- 登录成功写 `login_log`（含 IP/UA/结果）。

## 8. 操作审计（Audit）

```go
// server/internal/model/audit_log.go
type AuditLog struct {
	ID        uint      `gorm:"primaryKey"`
	User      string    `gorm:"size:64;index"`
	Action    string    `gorm:"size:32"`   // create/update/delete/run/retry/cancel/login...
	Object    string    `gorm:"size:128"`  // task:12 / node:3
	IP        string    `gorm:"size:45"`
	Params    string    `gorm:"type:text"` // 脱敏后的请求参数
	Result    string    `gorm:"size:16"`   // success/failure
	CreatedAt time.Time `gorm:"index"`
}
```

- 用 Gin 中间件统一记录写操作审计。
- **敏感参数脱敏**：密码/token/密钥/请求体中的认证头替换为 `***`。

```go
func maskSensitive(key string) string {
	if key == "password" || key == "token" || key == "authorization" || key == "secret" {
		return "***"
	}
	return key
}
```

## 9. 任务版本（Version）

- 每次任务配置修改（Create/Update）生成新版本 `v1 → v2 → v3`。

```go
// server/internal/model/task_version.go
type TaskVersion struct {
	ID        uint   `gorm:"primaryKey"`
	TaskID    uint   `gorm:"index"`
	Version   int    `gorm:"index"`
	ChangeBy  string `gorm:"size:64"`
	Diff      string `gorm:"type:text"` // 变更字段前后差异
	Content   string `gorm:"type:text"` // 该版本完整 JSON
	Remark    string `gorm:"size:255"`
	CreatedAt time.Time
}
```

- 能力：查看历史版本、版本 Diff（JSON diff）、回滚到指定版本。
- 回滚 = 生成新版本并写入旧 Content（不回删，保证可追溯）。

## 10. 节点监控（Monitor）

- 扩展 goscheduler 的 host 为 Worker 节点，Worker 心跳上报资源。

```go
type SchedulerNode struct {
	ID              uint   `gorm:"primaryKey"`
	Name            string `gorm:"size:64"`
	Address         string `gorm:"size:128;uniqueIndex"`
	Status          int8   `gorm:"index;default:0"` // 0 offline 1 online
	CPUUsage        float64
	MemoryUsage     float64
	DiskUsage       float64
	Load            float64
	LastHeartbeatAt time.Time
	CreatedAt       time.Time
}
```

- 心跳超时（如 30s 未上报）标记离线并触发告警。
- 首页节点资源卡用实时数据 + 进度条展示。

## 11. 增强功能与 goscheduler 对应关系

| 增强模块 | goscheduler 基础 | 增强内容 |
|---|---|---|
| Task/Execution 解耦 | task_log | 引入独立 execution + 生命周期 |
| 重试策略 | RetryTimes/RetryInterval | 指数退避、可重试规则、Attempt 轨迹 |
| 超时 | Timeout | 真正中断执行 + 告警 |
| DAG 依赖 | DependencyTaskId 逗号分隔 | success/complete/failure 拓扑 + 环检测 + 可视化 |
| 告警中心 | NotifyStatus | 规则化 + 连续失败 + 节点告警 + 记录 |
| RBAC | IsAdmin | 角色 + 权限点 + 中间件 |
| 审计 | 无 | 全量操作审计 + 脱敏 |
| 任务版本 | 无 | 版本化 + Diff + 回滚 |
| 节点监控 | host | 心跳 + 资源上报 + 离线告警 |

## 12. 验收要点
- 执行有独立 `task_execution` 记录，重启后仍可查历史。
- 超时任务被正确中断、标记并可按策略重试。
- DAG 依赖可正确触发下游、成环被拦截。
- 告警命中生成记录并成功通知渠道。
- 权限点在后端强制校验，前端按钮按权限显隐。
- 所有写操作有审计日志，敏感参数已脱敏。
- 任务修改后有多版本可查看/回滚。
