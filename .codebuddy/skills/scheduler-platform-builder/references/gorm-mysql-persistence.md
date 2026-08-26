# GORM 数据持久化参考（SQLite 默认 / MySQL 可选）

本文件规范调度平台后端如何用 **GORM** 做数据持久化。当前落地项目默认 **SQLite**（纯 Go 驱动 `glebarez/sqlite`，`CGO_ENABLED=0` 跨平台），可切换 **MySQL**；二者通过配置 `database.driver` 切换。

> **重要**：所有 Go 后端代码（含 go.mod/go.sum）统一放在 `server/` 目录下，本文件的包路径均以 `server/` 为根。落地实例：`/home/chenguohong/go/src/gocron`。

## 1. 依赖与连接

依赖：`gorm.io/gorm`、`github.com/glebarez/sqlite`（默认）、`gorm.io/driver/mysql`（可选）。

在 `server/conf.d/config.yaml` 配置：

```yaml
database:
  driver: sqlite            # sqlite（默认）或 mysql
  dsn: scheduler.db
  # driver: mysql
  # dsn: "user:password@tcp(127.0.0.1:3306)/scheduler?charset=utf8mb4&parseTime=True&loc=Local"
  maxIdleConns: 10
  maxOpenConns: 100
  connMaxLifetime: 2h
```

连接封装在 **`server/pkg/database/database.go`**（落地实现），按 driver 选择 dialector：

```go
package database

func New(conf *config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch strings.ToLower(conf.Driver) {
	case "mysql":
		dialector = mysql.Open(conf.DSN)
	default: // sqlite
		dialector = sqlite.Open(conf.DSN)
	}
	db, err := gorm.Open(dialector, &gorm.Config{ Logger: logger.Default.LogMode(logger.Warn) })
	// ...设置连接池
	return db, nil
}
```

在 `server/pkg/app/server.go` 启动时初始化并建表（落地实现）：

```go
db, _ := database.New(&conf.Database)
db.AutoMigrate(&model.Task{}, &model.Execution{}, &model.TaskLog{}, &model.Node{}, &model.User{})
dataStore := store.New(db)   // 仅初始化 admin 账号（无演示数据）
sched := scheduler.New(dataStore, executor.New(dataStore))
```

## 2. 模型规范（GORM tag）

每个模型用 GORM tag 明确：主键、类型、长度、索引、默认值、可空、唯一约束。

```go
package model

import "time"

type Task struct {
	ID              uint      `gorm:"primaryKey"`
	Name            string    `gorm:"size:64;uniqueIndex;not null"`
	Type            string    `gorm:"size:16;index"` // cron/http/shell/grpc
	CronExpr        string    `gorm:"size:64"`
	Spec            string    `gorm:"size:64"`
	Command         string    `gorm:"size:512"`
	RequestBody     string    `gorm:"type:text"`
	HttpMethod      string    `gorm:"size:8"`
	Timeout         int       `gorm:"default:0"`   // 秒，0 不限制
	Concurrency     string    `gorm:"size:16"`     // allow/forbid/queue/overwrite/max
	MaxConcurrent   int       `gorm:"default:1"`
	RetryMax        int       `gorm:"default:0"`
	RetryInterval   int       `gorm:"default:0"`
	RetryPolicy     string    `gorm:"size:16"` // fixed/backoff
	Group           string    `gorm:"size:32;index"`
	Owner           string    `gorm:"size:32"`
	Tag             string    `gorm:"size:32;index"`
	NotifyStatus    int8      `gorm:"default:1"`
	NotifyType      int8      `gorm:"default:0"`
	NotifyReceiver  string    `gorm:"size:256"`
	Status          int8      `gorm:"index;default:1"` // 1 enabled 0 disabled
	Remark          string    `gorm:"size:100"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"` // 软删除
}
```

### 模型编写约束
- 统一使用 `uint` / `int64` 主键，`gorm.Model` 或显式声明 `CreatedAt/UpdatedAt/DeletedAt`。
- 时间字段用 `time.Time`；布尔用 `int8` 或 `bool`（与旧系统 int 兼容用 `int8`）。
- 多对多关系（task<->host、user<->role）用中间表模型显式建模，不滥用 GORM many2many tag。
- 大字段（JSON、结果、请求体）用 `type:text` 或 `type:json`。

## 3. 数据访问层规范（Store，落地实现）

落地项目用 `internal/store`（单 Store 结构）封装所有数据访问，Service 层不得直接操作 `*gorm.DB`（可通过 `Store.DB()` 获取底层句柄用于事务）。

```go
package store

type Store struct{ db *gorm.DB }

func New(db *gorm.DB) *Store {
	s := &Store{db: db}
	s.ensureAdmin() // 仅初始化 admin 账号（bcrypt），无演示数据
	return s
}

func (s *Store) DB() *gorm.DB { return s.db }

// 列表/详情/增删改
func (s *Store) ListTasks() []*model.Task
func (s *Store) GetTask(id int64) (*model.Task, bool)
func (s *Store) CreateTask(t *model.Task) *model.Task   // 返回 nil 表示失败
func (s *Store) UpdateTask(t *model.Task) (*model.Task, bool)
func (s *Store) DeleteTask(id int64) bool

// Execution
func (s *Store) ListExecutions(limit int) []*model.Execution
func (s *Store) GetExecution(id int64) (*model.Execution, bool)
func (s *Store) CreateExecution(e *model.Execution) *model.Execution // 返回 nil 表示失败
func (s *Store) UpdateExecution(e *model.Execution) bool

// Dashboard 聚合（避免扫全表）
func (s *Store) CountExecutions(where string, args ...interface{}) int64
func (s *Store) ExecutionsByStatus() map[string]int64
```

- 列表/搜索若需分页，可自行在 Handler 中过滤（当前任务列表支持 `?keyword&type&status` 过滤）。
- 初始化：仅 `ensureAdmin()` 创建 admin 用户（bcrypt `admin123`）；**无演示数据**。批量初始化见 `conf.d/init.sql`（DML，表结构由 GORM AutoMigrate 同步）。

## 4. 事务

需要原子性的操作（如执行完成后同时更新 execution + task）必须用事务。落地项目中用 `s.db.Transaction`（或 `Store.DB()` 获取句柄）：

```go
func (s *Store) DoInTx(fn func(tx *gorm.DB) error) error {
	return s.db.Transaction(fn)
}
```

例如执行完成后原子更新 execution + task：

```go
dataStore.DoInTx(func(tx *gorm.DB) error {
	if err := tx.Model(&model.Execution{}).Where("id = ?", execID).
		Updates(map[string]interface{}{"status": "success", "finished_at": now}).Error; err != nil {
		return err
	}
	return tx.Model(&model.Task{}).Where("id = ?", taskID).
		Update("next_run_time", next).Error
})
```

## 5. 索引与查询优化

### 落地模型的推荐索引
| 表（model） | 索引（gorm tag 已声明） |
|---|---|
| `tasks` | `code` 唯一索引、`name` index、`type` index、`status` index、`group` index、`owner_id` index |
| `executions` | `task_id` index、`trigger_type` index、`status` index、`started_at` index |
| `task_logs` | `execution_id` index、`task_id` index |
| `nodes` | `node_id` 唯一索引、`status` index |
| `users` | `username` 唯一索引、`status` index |

### Dashboard 聚合
- 禁止实时扫描 `executions` 全表做全表聚合。
- 用 `COUNT` + `GROUP BY`（落地实现：`Store.CountExecutions`、`Store.ExecutionsByStatus`）。
- 今日指标：`CountExecutions("started_at >= ?", startOfDay)` + `CountExecutions("started_at >= ? AND status = ?", startOfDay, "success")`。

## 6. 迁移与版本升级

- 首启用 `AutoMigrate` 建表；已有表时 GORM 会自动补字段（不删列，注意破坏性变更需手动迁移）。
- 重大 schema 变更走显式迁移（参考 goscheduler `migration.go` 的版本化升级），用 `db.Migrator()` 判断列/索引存在性：

```go
func ensureColumn(db *gorm.DB, table, column string, autoAdd func()) {
	if !db.Migrator().HasColumn(table, column) {
		autoAdd()
	}
}
```

## 7. 初始化与种子数据（落地实现）

- 首启用 `AutoMigrate` 建表；已有表时 GORM 会自动补字段（不删列，注意破坏性变更需手动迁移）。
- 空库时 `store.New` 内部 `ensureAdmin()` 仅创建 admin 用户（bcrypt，密码 `admin123`）；**不再写入节点/任务/执行等演示数据**，页面数据全部来自真实任务创建与执行。批量初始化（admin、默认节点等）用 `conf.d/init.sql`（DML，表结构由 GORM AutoMigrate 同步）。**无需独立安装引导页面。**

## 8. 从 goscheduler（Xorm）迁移映射

| goscheduler Xorm | 调度平台 GORM |
|---|---|
| `xorm:"pk autoincr"` | `gorm:"primaryKey;autoIncrement"` |
| `xorm:"varchar(32) notnull"` | `gorm:"size:32;not null"` |
| `xorm:"index"` | `gorm:"index"` |
| `xorm:"unique"` | `gorm:"uniqueIndex"` |
| `xorm:"tinyint default 0"` | `gorm:"default:0"` |
| `xorm:"text"` | `gorm:"type:text"` |
| `xorm:"datetime created"` | `time.Time` + `CreatedAt` |
| `xorm:"datetime deleted"` | `gorm.DeletedAt`（软删除） |
| `Db.Where/In/Join` | `db.Where/Where("id IN ?")/Joins` |
| `Db.Limit/Desc` | `db.Order("id DESC").Offset().Limit()` |
| `session.Begin/Commit/Rollback` | `db.Transaction` |

## 9. 验收要点
- 重启服务后数据仍在（落库验证，默认 `server/scheduler.db` SQLite）。
- Dashboard 聚合用 COUNT/GROUP BY，接口响应流畅。
- 关键写操作（执行完成同时更新 execution + task）走事务。
- 敏感字段（密码/token）不直接返回，密码用 bcrypt 存储。
- 登录成功签发 JWT，受保护 API 需 `Authorization: Bearer <token>`。
