#!/usr/bin/env bash
#
# 调度平台脚手架生成脚本
#
# 用法:
#   create-project-scaffold.sh <项目名> [目标目录]
#
# 说明:
#   - 生成后端目录结构（参考 go-proxy 分层，Gin + GORM，SQLite 默认 / MySQL 可选）
#   - 生成前端 Vue3 目录结构（参考 goscheduler 页面划分）
#   - 生成 SQLite/MySQL 连接与建表基础代码（见 references/gorm-mysql-persistence.md）
#
# 与落地实例对齐：/home/chenguohong/go/src/gocron
#   - 后端统一 server/（模块名 <MODULE>，import 基于模块名而非目录路径）
#   - 数据仓库层用 store/（替代 repository/）
#   - 数据库默认 SQLite（glebarez/sqlite，CGO_ENABLED=0），可切 MySQL
#
set -euo pipefail

PROJECT_NAME="${1:?用法: create-project-scaffold.sh <项目名> [目标目录]}"
TARGET_DIR="${2:-$PROJECT_NAME}"
MODULE="${3:-$PROJECT_NAME}"

mkdir -p "$TARGET_DIR"
cd "$TARGET_DIR"

# Go 后端代码统一放在 server/ 目录
echo "创建后端目录结构（参考 go-proxy，置于 server/ 下）..."
mkdir -p server/cmd
mkdir -p server/conf.d
mkdir -p server/logs
mkdir -p server/pkg/app/config
mkdir -p server/pkg/app/route
mkdir -p server/pkg/auth
mkdir -p server/pkg/database
mkdir -p server/pkg/ecode
mkdir -p server/pkg/ilog
mkdir -p server/pkg/shutdown
mkdir -p server/internal/handler
mkdir -p server/internal/middleware
mkdir -p server/internal/service
mkdir -p server/internal/store
mkdir -p server/internal/model
mkdir -p server/internal/scheduler

echo "创建前端目录结构（参考 goscheduler + Vue3/Vite）..."
mkdir -p web/src/api
mkdir -p web/src/components
mkdir -p web/src/layouts
mkdir -p web/src/router
mkdir -p web/src/stores
mkdir -p web/src/styles
mkdir -p web/src/views/{login,dashboard,task,execution,node}

echo "创建后端占位文件（置于 server/ 下）..."

# go.mod
cat > server/go.mod <<EOF
module $MODULE

go 1.25.0

require (
	github.com/gin-gonic/gin v1.11.0
	github.com/glebarez/sqlite v1.11.0
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/viper v1.18.2
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.55.0
	gorm.io/driver/mysql v1.5.7
	gorm.io/gorm v1.25.12
)
EOF

# cmd/main.go
cat > server/cmd/main.go <<'EOF'
package main

import (
	"context"
	"log"
	"syscall"
	"time"

	"<MODULE>/pkg/app"
	"<MODULE>/pkg/shutdown"
)

func main() {
	hook := shutdown.NewHook().
		WithSignals(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP).
		WithTimeout(60 * time.Second)

	hook.AddCloseFunc(func(ctx context.Context) {
		log.Println("cleaning up scheduler platform resources...")
	})

	if err := hook.Run(app.NewServer); err != nil {
		log.Fatalf("application failed: %v", err)
	}
	log.Println("application exited successfully")
}
EOF

# conf.d/config.yaml
cat > server/conf.d/config.yaml <<'EOF'
app:
  name: scheduler-platform
  env: dev

server:
  host: 0.0.0.0
  port: 8080

database:
  # sqlite（默认，纯 Go 驱动，无需安装）或 mysql
  driver: sqlite
  dsn: scheduler.db
  # MySQL 示例:
  # driver: mysql
  # dsn: "root:root@tcp(127.0.0.1:3306)/scheduler?charset=utf8mb4&parseTime=True&loc=Local"
  maxIdleConns: 10
  maxOpenConns: 100
  connMaxLifetime: 2h

security:
  jwtSecret: scheduler-platform-secret
  tokenExpireHours: 24

logger:
  level: info
  filePath: ./logs
  maxSize: 100
EOF

# pkg/database/database.go
cat > server/pkg/database/database.go <<'EOF'
package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Driver          string
	DSN             string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

// New 根据配置初始化 GORM，driver 支持 sqlite（默认）/ mysql。
func New(conf *Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch strings.ToLower(conf.Driver) {
	case "mysql":
		dialector = mysql.Open(conf.DSN)
	default: // sqlite
		dialector = sqlite.Open(conf.DSN)
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open database(%s): %w", conf.Driver, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxIdleConns(conf.MaxIdleConns)
	sqlDB.SetMaxOpenConns(conf.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(conf.ConnMaxLifetime)
	return db, nil
}
EOF

# internal/model/model.go
cat > server/internal/model/model.go <<'EOF'
package model

import "time"

// 任务状态
const (
	TaskStatusDisabled = 0
	TaskStatusEnabled  = 1
)

// Task 任务配置（与 Execution 解耦，Task 是可改配置，Execution 是一次执行）
type Task struct {
	ID             int64     `json:"id" gorm:"primaryKey"`
	Code           string    `json:"code" gorm:"uniqueIndex;size:64"`
	Name           string    `json:"name" gorm:"size:128;index"`
	Type           string    `json:"type" gorm:"size:16;index"` // cron/http/shell/grpc
	Status         int       `json:"status" gorm:"index"`       // 0 停用 1 启用
	CronExpr       string    `json:"cronExpr" gorm:"size:64"`
	Timeout        int       `json:"timeout"`
	Concurrency    int       `json:"concurrency"`
	RetryCount     int       `json:"retryCount"`
	NextRunTime    time.Time `json:"nextRunTime"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Execution 一次执行
type Execution struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	TaskID       int64     `json:"taskId" gorm:"index"`
	TaskName     string    `json:"taskName" gorm:"size:128"`
	TaskType     string    `json:"taskType" gorm:"size:16"`
	TriggerType  string    `json:"triggerType" gorm:"size:16;index"` // cron / manual / retry
	WorkerName   string    `json:"workerName" gorm:"size:64"`
	Status       string    `json:"status" gorm:"size:16;index"` // pending/running/success/failed/timeout/canceled/retrying
	Attempt      int       `json:"attempt"`
	StartedAt    time.Time `json:"startedAt" gorm:"index"`
	FinishedAt   time.Time `json:"finishedAt"`
	DurationMs   int64     `json:"durationMs"`
	ErrorMessage string    `json:"errorMessage" gorm:"type:text"`
	CreatedAt    time.Time `json:"createdAt"`
}

// User 用户
type User struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"uniqueIndex;size:64"`
	Password  string    `json:"-" gorm:"size:255"` // bcrypt 哈希
	Nickname  string    `json:"nickname" gorm:"size:64"`
	Email     string    `json:"email" gorm:"size:128"`
	IsAdmin   bool      `json:"isAdmin"`
	Status    int       `json:"status" gorm:"index"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
EOF

echo ""
echo "✅ 脚手架创建完成: $TARGET_DIR"
echo ""
echo "项目结构（前后端分离）:"
echo "  server/   Go 后端代码（含 go.mod，模块名 $MODULE，import 基于模块名）"
echo "  web/      前端 Vue3 + Element Plus"
echo ""
echo "下一步:"
echo "  1. 替换 server/cmd/main.go 与各文件中 <MODULE> 占位符为 $MODULE"
echo "  2. 实现 server/pkg/{config,ecode,ilog,shutdown,auth} 与 internal/{handler,service,store,scheduler,middleware}"
echo "  3. 在 server/pkg/app/server.go 中 AutoMigrate + 启动调度器 + 创建路由"
echo "  4. 构建：cd server && CGO_ENABLED=0 go build -o ../scheduler-server ./cmd"
echo "  5. 实现前端首页 Dashboard 页面"
echo "  6. 详细持久化规范见 references/gorm-mysql-persistence.md，增强功能见 references/enhanced-features.md"
EOF
