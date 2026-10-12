package model

import "time"

// 任务状态
const (
	TaskStatusDisabled = 0
	TaskStatusEnabled  = 1
)

// 任务类型
const (
	TaskTypeCron = "cron"
	TaskTypeHTTP = "http"
	TaskTypeShell = "shell"
	TaskTypeGRPC = "grpc"
)

// 权限点常量（RBAC）
const (
	PermDashboardView = "dashboard:view" // 查看首页看板
	PermTaskView      = "task:view"      // 查看任务
	PermTaskCreate    = "task:create"    // 创建任务
	PermTaskUpdate    = "task:update"    // 更新任务
	PermTaskDelete    = "task:delete"    // 删除任务
	PermTaskRun       = "task:run"       // 手动执行/启停
	PermExecView      = "exec:view"      // 查看执行记录
	PermLogView       = "log:view"       // 查看执行日志
	PermNodeView      = "node:view"      // 查看节点
	PermNodeManage    = "node:manage"    // 节点管理（增删改）
	PermUserManage    = "user:manage"    // 用户管理
	PermRoleManage    = "role:manage"    // 角色管理
	PermNotifyView    = "notify:view"    // 查看通知配置
	PermNotifyManage  = "notify:manage"  // 管理通知配置
	PermSystemAdmin   = "system:admin"   // 系统管理员（全部权限）
)

// Task 任务配置（Task 与 Execution 解耦）
type Task struct {
	ID             int64     `json:"id" gorm:"primaryKey"`
	Code           string    `json:"code" gorm:"uniqueIndex;size:64"`
	Name           string    `json:"name" gorm:"size:128;index"`
	Type           string    `json:"type" gorm:"size:16;index"` // cron/http/shell/grpc
	Description    string    `json:"description" gorm:"size:512"`
	Group          string    `json:"group" gorm:"size:64;index"`
	Tag            string    `json:"tag" gorm:"size:64"`
	Environment    string    `json:"environment" gorm:"size:32"`
	Status         int       `json:"status" gorm:"index"` // 0 停用 1 启用
	CronExpr       string    `json:"cronExpr" gorm:"size:64"`
	Timeout        int       `json:"timeout"` // 秒
	Concurrency    int       `json:"concurrency"`
	RetryCount     int       `json:"retryCount"`
	RetryInterval  int       `json:"retryInterval"`
	OwnerID        int64     `json:"ownerId" gorm:"index"`
	Owner          string    `json:"owner" gorm:"size:64"`
	ExecutorConfig string    `json:"executorConfig" gorm:"type:text"`
	// 通知配置
	NotifyEnabled    bool   `json:"notifyEnabled" gorm:"default:false"`   // 是否启用执行结果通知
	NotifyType       string `json:"notifyType" gorm:"size:32"`            // 通知渠道：dingtalk / webhook / dingtalk,webhook
	NotifyOnSuccess  bool   `json:"notifyOnSuccess" gorm:"default:false"` // 成功时也通知（默认仅失败/超时）
	NotifyDingtalk   string `json:"notifyDingtalk" gorm:"size:512"`       // 钉钉机器人 webhook 地址（空则用全局配置）
	NotifySecret     string `json:"notifySecret" gorm:"size:255"`         // 钉钉加签密钥（可选）
	NotifyWebhook    string `json:"notifyWebhook" gorm:"size:512"`        // 通用 webhook 地址（空则用全局配置）
	NotifyWebhookSec string `json:"notifyWebhookSec" gorm:"size:255"`     // 通用 webhook 签名密钥（可选）
	NextRunTime      *time.Time `json:"nextRunTime"`
	CreatedBy      string    `json:"createdBy" gorm:"size:64"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`

	// NodeIDs 任务绑定的节点ID列表（多对多，非 DB 列）
	NodeIDs []int64 `json:"nodeIds" gorm:"-"`
	// Nodes 任务绑定的节点详情（非 DB 列，用于展示）
	Nodes []*Node `json:"nodes" gorm:"-"`
}

// TaskNode 任务-节点多对多关联表。
type TaskNode struct {
	ID     int64 `json:"id" gorm:"primaryKey"`
	TaskID int64 `json:"taskId" gorm:"uniqueIndex:idx_task_node;index"`
	NodeID int64 `json:"nodeId" gorm:"uniqueIndex:idx_task_node;index"`
}

// Execution 一次执行
type Execution struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	TaskID       int64     `json:"taskId" gorm:"index"`
	TaskName     string    `json:"taskName" gorm:"size:128"`
	TaskType     string    `json:"taskType" gorm:"size:16"`
	TriggerType  string    `json:"triggerType" gorm:"size:16;index"` // cron / manual / retry
	WorkerID     int64     `json:"workerId" gorm:"index"`
	WorkerName   string    `json:"workerName" gorm:"size:64"`
	Status       string    `json:"status" gorm:"size:16;index"` // pending/running/success/failed/timeout/canceled/retrying
	Attempt      int       `json:"attempt"`
	StartedAt    time.Time `json:"startedAt" gorm:"index"`
	FinishedAt   time.Time `json:"finishedAt"`
	DurationMs   int64     `json:"durationMs"`
	ErrorCode    string    `json:"errorCode" gorm:"size:16"`
	ErrorMessage string    `json:"errorMessage" gorm:"type:text"`
	RetryCount   int       `json:"retryCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// TaskLog 执行日志
type TaskLog struct {
	ID          int64     `json:"id" gorm:"primaryKey"`
	ExecutionID int64     `json:"executionId" gorm:"index"`
	TaskID      int64     `json:"taskId" gorm:"index"`
	Stream      string    `json:"stream" gorm:"size:16"` // stdout/stderr
	Level       string    `json:"level" gorm:"size:16"`
	Content     string    `json:"content" gorm:"type:text"`
	Sequence    int       `json:"sequence"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Node 调度节点
type Node struct {
	ID             int64     `json:"id" gorm:"primaryKey"`
	NodeID         string    `json:"nodeId" gorm:"uniqueIndex;size:64"`
	Name           string    `json:"name" gorm:"size:64"`
	Address        string    `json:"address" gorm:"size:128"`
	MetricsURL     string    `json:"metricsUrl" gorm:"size:512"` // Prometheus metrics 端点地址
	Proxy          string    `json:"proxy" gorm:"size:256"`      // 节点专属出站代理（http://user:pass@host:port 或 socks5://host:port），空则回落全局代理
	OS             string    `json:"os" gorm:"size:64"`
	Version        string    `json:"version" gorm:"size:32"`
	Status         string    `json:"status" gorm:"size:16;index"` // online/offline/busy/warning/error
	CPUUsage       float64   `json:"cpuUsage"`
	MemoryUsage    float64   `json:"memoryUsage"`
	DiskUsage      float64   `json:"diskUsage"`
	LoadAverage    float64   `json:"loadAverage"`
	RunningTasks   int       `json:"runningTasks"`
	MaxConcurrency int       `json:"maxConcurrency"`
	LastHeartbeat  time.Time `json:"lastHeartbeat"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// GlobalProxy 全局出站代理配置（单条记录，ID 固定为 1）。
// 用于调度中心访问节点/外部接口时统一走代理（机场类 HTTP/SOCKS5 代理）。
type GlobalProxy struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Enabled   bool      `json:"enabled"`                 // 是否启用
	Type      string    `json:"type" gorm:"size:16"`     // http / socks5
	Host      string    `json:"host" gorm:"size:128"`    // 代理 IP 或域名
	Port      int       `json:"port"`                    // 代理端口
	Username  string    `json:"username" gorm:"size:128"`
	Password  string    `json:"password" gorm:"size:256"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// User 用户
type User struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"uniqueIndex;size:64"`
	Password  string    `json:"-" gorm:"size:255"`
	Nickname  string    `json:"nickname" gorm:"size:64"`
	Email     string    `json:"email" gorm:"size:128"`
	IsAdmin   bool      `json:"isAdmin"`
	RoleID    int64     `json:"roleId" gorm:"index"`
	Status    int       `json:"status" gorm:"index"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Role 角色。
type Role struct {
	ID          int64        `json:"id" gorm:"primaryKey"`
	Code        string       `json:"code" gorm:"uniqueIndex;size:64"`
	Name        string       `json:"name" gorm:"size:64"`
	Description string       `json:"description" gorm:"size:255"`
	BuiltIn     bool         `json:"builtIn"` // 内置角色不可删除
	PermIDs     []int64      `json:"permIds" gorm:"-"`
	Permissions []Permission `json:"permissions" gorm:"many2many:role_permissions;"`
	// MenuIDs 角色绑定的菜单ID列表（非 DB 列，用于授权展示）
	MenuIDs   []int64  `json:"menuIds" gorm:"-"`
	BtnMap    map[int64][]int64 `json:"btnMap" gorm:"-"` // menuId -> btnIds，角色-菜单-按钮授权
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Permission 权限点。
type Permission struct {
	ID          int64     `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;size:64"`
	Name        string    `json:"name" gorm:"size:64"`
	Type        string    `json:"type" gorm:"size:16;index"` // menu / button / api
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"createdAt"`
}

// UserRole 用户-角色关联（保留，兼容多角色扩展）。
type UserRole struct {
	ID     int64     `json:"id" gorm:"primaryKey"`
	UserID int64     `json:"userId" gorm:"uniqueIndex:idx_user_role;index"`
	RoleID int64     `json:"roleId" gorm:"uniqueIndex:idx_user_role;index"`
	CreatedAt time.Time `json:"createdAt"`
}

// RolePermission 角色-权限点关联。
type RolePermission struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	RoleID       int64     `json:"roleId" gorm:"uniqueIndex:idx_role_perm;index"`
	PermissionID int64     `json:"permissionId" gorm:"uniqueIndex:idx_role_perm;index"`
	CreatedAt    time.Time `json:"createdAt"`
}

// NodeMetricsSample 节点指标历史采样（时序数据，供趋势图展示）。
// 每次节点上报或主动抓取时落一条，保留最近 7 天，定期清理。
type NodeMetricsSample struct {
	ID            int64     `json:"id" gorm:"primaryKey"`
	NodeID        int64     `json:"nodeId" gorm:"index:idx_node_time"`
	NodeKey       string    `json:"nodeKey" gorm:"size:64;index"` // 冗余 node_id 字符串，便于查询
	CPUUsage      float64   `json:"cpuUsage"`
	MemoryUsage   float64   `json:"memoryUsage"`
	DiskUsage     float64   `json:"diskUsage"`
	LoadAverage   float64   `json:"loadAverage"`
	RunningTasks  int       `json:"runningTasks"`
	SampledAt     time.Time `json:"sampledAt" gorm:"index:idx_node_time"`
}

// NotifySetting 全局通知配置（单行：ID 恒为 1）。
// 提供钉钉 / Webhook 全局默认渠道，供「通知配置」设置页编辑。
type NotifySetting struct {
	ID              int64     `json:"id" gorm:"primaryKey"`
	Enabled         bool      `json:"enabled" gorm:"default:false"`       // 是否启用全局通知
	DingtalkWebhook string    `json:"dingtalkWebhook" gorm:"size:512"`    // 钉钉机器人 webhook 地址
	DingtalkSecret  string    `json:"dingtalkSecret" gorm:"size:255"`     // 钉钉加签密钥（可选）
	WebhookURL      string    `json:"webhookUrl" gorm:"size:512"`         // 通用 webhook 地址
	WebhookSecret   string    `json:"webhookSecret" gorm:"size:255"`      // 通用 webhook 签名密钥（可选）
	NotifyOnSuccess bool      `json:"notifyOnSuccess" gorm:"default:false"` // 成功时也通知
	UpdatedBy       string    `json:"updatedBy" gorm:"size:64"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
