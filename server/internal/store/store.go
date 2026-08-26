package store

import (
	"context"
	"encoding/json"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"scheduler-platform/internal/model"
	"scheduler-platform/pkg/cache"
	"scheduler-platform/pkg/casbin"
)

// Store 数据仓库，基于 GORM 持久化（默认 SQLite，可配置 MySQL）。
// 所有业务数据落库，重启后数据仍在。
type Store struct {
	db    *gorm.DB
	cache *cache.GlobalCache // 可选缓存（nil 表示不启用）
	// Enforcer Casbin API 策略执行器（角色-API 访问控制）
	Enforcer *casbin.Enforcer
}

// New 创建数据仓库并自动建表；初始化 RBAC、菜单/API、admin 账号。
// 业务数据全部来自真实任务创建与执行；如需批量初始化，见 conf.d/init.sql。
// gc 为可选缓存实例，传 nil 则不启用缓存。
func New(db *gorm.DB, gc *cache.GlobalCache) *Store {
	ctx := context.Background()
	s := &Store{db: db, cache: gc, Enforcer: casbin.New(db)}
	// 初始化 RBAC（权限点 + 内置角色）→ 菜单/受管API → admin 账号 → 内置角色默认权限
	s.InitRBAC(ctx)
	s.InitPermissionData(ctx, s.Enforcer)
	s.ensureAdmin(ctx)
	s.SyncBuiltinRolePermissions(ctx, s.Enforcer)
	return s
}

// DB 返回底层 GORM 句柄（供调度器/事务使用）。
func (s *Store) DB() *gorm.DB { return s.db }

// ensureAdmin 仅当无任何用户时创建默认 admin（密码 bcrypt 加密），保证可登录。
// admin 拥有全部权限（isAdmin=true），RoleID 指向内置 admin 角色。
func (s *Store) ensureAdmin(ctx context.Context) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.User{}).Count(&count).Error; err != nil || count > 0 {
		return
	}
	pw, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	var roleID int64
	if r, ok := s.GetRoleByCode(ctx, "admin"); ok {
		roleID = r.ID
	}
	s.db.WithContext(ctx).Create(&model.User{
		Username: "admin", Password: string(pw),
		Nickname: "管理员", Email: "admin@scheduler.local",
		IsAdmin: true, RoleID: roleID, Status: 1, CreatedAt: time.Now(),
	})
}

// ---------- Task ----------

func (s *Store) ListTasks(ctx context.Context) []*model.Task {
	cacheKey := cache.KeyTaskList("", "", -1, 0, 0)
	if s.cache != nil {
		if tasks, ok := s.getTasksCache(ctx, cacheKey); ok {
			return tasks
		}
	}
	tasks := s.listTasksDB(ctx)
	if s.cache != nil && len(tasks) > 0 {
		s.setTasksCache(ctx, cacheKey, tasks, 3*time.Minute)
	}
	return tasks
}

func (s *Store) listTasksDB(ctx context.Context) []*model.Task {
	var tasks []*model.Task
	s.db.WithContext(ctx).Order("id ASC").Find(&tasks)
	for _, t := range tasks {
		s.loadTaskNodes(ctx, t)
	}
	return tasks
}

// CountTasksCreatedBefore 统计 created_at 早于指定时间的任务数（用于首页周环比等）。
func (s *Store) CountTasksCreatedBefore(ctx context.Context, t time.Time) int64 {
	var c int64
	s.db.WithContext(ctx).Model(&model.Task{}).Where("created_at < ?", t).Count(&c)
	return c
}

// PageTasks 分页查询任务，支持按关键字/类型/状态筛选（下推到 SQL）。
func (s *Store) PageTasks(ctx context.Context, keyword, ttype string, status int, page, pageSize int) ([]*model.Task, int64) {
	query := s.db.WithContext(ctx).Model(&model.Task{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR code LIKE ?", like, like)
	}
	if ttype != "" {
		query = query.Where("type = ?", ttype)
	}
	if status >= 0 {
		query = query.Where("status = ?", status)
	}
	var total int64
	query.Count(&total)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var tasks []*model.Task
	query.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks)
	for _, t := range tasks {
		s.loadTaskNodes(ctx, t)
	}
	return tasks, total
}

func (s *Store) GetTask(ctx context.Context, id int64) (*model.Task, bool) {
	var t model.Task
	if err := s.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, false
	}
	s.loadTaskNodes(ctx, &t)
	return &t, true
}

func (s *Store) CreateTask(ctx context.Context, t *model.Task) *model.Task {
	now := time.Now()
	t.ID = 0
	t.CreatedAt = now
	t.UpdatedAt = now
	if err := s.db.WithContext(ctx).Create(t).Error; err != nil {
		return nil
	}
	s.SaveTaskNodes(ctx, t.ID, t.NodeIDs)
	s.loadTaskNodes(ctx, t)
	s.invalidateTasksCache(ctx)
	return t
}

// UpdateTaskNextRunTime 更新任务下一次执行时间。
func (s *Store) UpdateTaskNextRunTime(ctx context.Context, id int64, next time.Time) bool {
	return s.db.WithContext(ctx).Model(&model.Task{}).Where("id = ?", id).Update("next_run_time", next).Error == nil
}

func (s *Store) UpdateTask(ctx context.Context, t *model.Task) (*model.Task, bool) {
	db := s.db.WithContext(ctx)
	var old model.Task
	if err := db.First(&old, t.ID).Error; err != nil {
		return nil, false
	}
	t.CreatedAt = old.CreatedAt
	t.UpdatedAt = time.Now()
	if err := db.Save(t).Error; err != nil {
		return nil, false
	}
	s.SaveTaskNodes(ctx, t.ID, t.NodeIDs)
	s.loadTaskNodes(ctx, t)
	s.invalidateTasksCache(ctx)
	return t, true
}

func (s *Store) DeleteTask(ctx context.Context, id int64) bool {
	res := s.db.WithContext(ctx).Delete(&model.Task{}, id)
	if res.Error != nil {
		return false
	}
	// 先保存本次删除影响的行数，避免后续 DeleteTaskNodes 覆盖 RowsAffected。
	affected := res.RowsAffected
	if affected > 0 {
		s.DeleteTaskNodes(ctx, id)
		s.invalidateTasksCache(ctx)
	}
	return affected > 0
}

// loadTaskNodes 填充任务的节点绑定信息。
func (s *Store) loadTaskNodes(ctx context.Context, t *model.Task) {
	t.NodeIDs = s.GetTaskNodeIDs(ctx, t.ID)
	t.Nodes = s.GetTaskNodes(ctx, t.ID)
}

// ---------- Execution ----------

func (s *Store) ListExecutions(ctx context.Context, limit int) []*model.Execution {
	var execs []*model.Execution
	db := s.db.WithContext(ctx).Order("started_at DESC")
	if limit > 0 {
		db = db.Limit(limit)
	}
	db.Find(&execs)
	return execs
}

// PageExecutions 分页查询执行记录，支持按任务/状态/触发方式筛选。
// 返回记录列表与总数。
func (s *Store) PageExecutions(ctx context.Context, taskID int64, status, trigger string, page, pageSize int) ([]*model.Execution, int64) {
	query := s.db.WithContext(ctx).Model(&model.Execution{})
	if taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if trigger != "" {
		query = query.Where("trigger_type = ?", trigger)
	}
	var total int64
	query.Count(&total)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var execs []*model.Execution
	query.Order("started_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&execs)
	return execs, total
}

func (s *Store) GetExecution(ctx context.Context, id int64) (*model.Execution, bool) {
	var e model.Execution
	if err := s.db.WithContext(ctx).First(&e, id).Error; err != nil {
		return nil, false
	}
	return &e, true
}

func (s *Store) CreateExecution(ctx context.Context, e *model.Execution) *model.Execution {
	now := time.Now()
	e.ID = 0
	e.CreatedAt = now
	if e.StartedAt.IsZero() {
		e.StartedAt = now
	}
	if err := s.db.WithContext(ctx).Create(e).Error; err != nil {
		return nil
	}
	return e
}

func (s *Store) UpdateExecution(ctx context.Context, e *model.Execution) bool {
	if err := s.db.WithContext(ctx).Save(e).Error; err != nil {
		return false
	}
	return true
}

// CountExecutions 统计某状态数量（用于 Dashboard 聚合，避免拉全表）。
func (s *Store) CountExecutions(ctx context.Context, where string, args ...interface{}) int64 {
	var c int64
	s.db.WithContext(ctx).Model(&model.Execution{}).Where(where, args...).Count(&c)
	return c
}

// TaskExecStats 单任务执行统计（用于任务详情/列表展示历史表现）。
type TaskExecStats struct {
	Total       int64   `json:"total"`
	Success     int64   `json:"success"`
	Failed      int64   `json:"failed"`
	Timeout     int64   `json:"timeout"`
	Running     int64   `json:"running"`
	AvgDuration float64 `json:"avgDurationMs"`
	SuccessRate float64 `json:"successRate"`
	LastStatus  string  `json:"lastStatus"`
	LastRunAt   *time.Time `json:"lastRunAt"`
}

// TaskExecStatsByID 统计单个任务的历史执行情况。
func (s *Store) TaskExecStatsByID(ctx context.Context, taskID int64) *TaskExecStats {
	stats := &TaskExecStats{}
	var rows []struct {
		Status string
		Cnt    int64
		AvgDur float64
	}
	s.db.WithContext(ctx).Model(&model.Execution{}).
		Select("status, count(*) as cnt, avg(duration_ms) as avg_dur").
		Where("task_id = ?", taskID).Group("status").Scan(&rows)
	for _, r := range rows {
		switch r.Status {
		case "success":
			stats.Success = r.Cnt
		case "failed":
			stats.Failed = r.Cnt
		case "timeout":
			stats.Timeout = r.Cnt
		case "running", "pending", "retrying":
			stats.Running += r.Cnt
		}
		stats.Total += r.Cnt
		if r.AvgDur > 0 {
			stats.AvgDuration = r.AvgDur
		}
	}
	if stats.Total > 0 {
		stats.SuccessRate = float64(stats.Success) / float64(stats.Total) * 100
	}
	// 最近一次执行状态
	var last model.Execution
	if err := s.db.WithContext(ctx).Where("task_id = ?", taskID).Order("started_at DESC").First(&last).Error; err == nil {
		stats.LastStatus = last.Status
		stats.LastRunAt = &last.StartedAt
	}
	return stats
}

// ListRecentExecsByTask 查询单个任务最近 N 条执行记录（供任务历史查看）。
func (s *Store) ListRecentExecsByTask(ctx context.Context, taskID int64, limit int) []*model.Execution {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var execs []*model.Execution
	s.db.WithContext(ctx).Where("task_id = ?", taskID).Order("started_at DESC").Limit(limit).Find(&execs)
	return execs
}

// ExecutionsByStatus 按状态统计数量。
func (s *Store) ExecutionsByStatus(ctx context.Context) map[string]int64 {
	var rows []struct {
		Status string
		Cnt    int64
	}
	s.db.WithContext(ctx).Model(&model.Execution{}).Select("status, count(*) as cnt").Group("status").Scan(&rows)
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Status] = r.Cnt
	}
	return out
}

// ExecutionsByStatusRange 按状态统计 started_at >= since 的执行数量。
func (s *Store) ExecutionsByStatusRange(ctx context.Context, since time.Time) map[string]int64 {
	var rows []struct {
		Status string
		Cnt    int64
	}
	s.db.WithContext(ctx).Model(&model.Execution{}).
		Select("status, count(*) as cnt").
		Where("started_at >= ?", since).
		Group("status").Scan(&rows)
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Status] = r.Cnt
	}
	return out
}

// ---------- 全局通知配置 ----------

// GetNotifySetting 读取全局通知配置（单行，ID=1；不存在则返回零值）。
func (s *Store) GetNotifySetting(ctx context.Context) *model.NotifySetting {
	var st model.NotifySetting
	if err := s.db.WithContext(ctx).First(&st, 1).Error; err != nil {
		return &model.NotifySetting{ID: 1}
	}
	return &st
}

// SaveNotifySetting 保存全局通知配置（UPSERT 单行 ID=1）。
func (s *Store) SaveNotifySetting(ctx context.Context, st *model.NotifySetting) {
	st.ID = 1
	st.UpdatedAt = time.Now()
	db := s.db.WithContext(ctx)
	var count int64
	db.Model(&model.NotifySetting{}).Count(&count)
	if count == 0 {
		st.CreatedAt = time.Now()
		db.Create(st)
		return
	}
	db.Model(&model.NotifySetting{}).Where("id = 1").Updates(map[string]interface{}{
		"enabled":          st.Enabled,
		"dingtalk_webhook": st.DingtalkWebhook,
		"dingtalk_secret":  st.DingtalkSecret,
		"webhook_url":      st.WebhookURL,
		"webhook_secret":   st.WebhookSecret,
		"notify_on_success": st.NotifyOnSuccess,
		"updated_by":       st.UpdatedBy,
		"updated_at":       st.UpdatedAt,
	})
}

// ---------- Node ----------

func (s *Store) ListNodes(ctx context.Context) []*model.Node {
	cacheKey := cache.KeyNodeList()
	if s.cache != nil {
		if nodes, ok := s.getNodesCache(ctx, cacheKey); ok {
			return nodes
		}
	}
	nodes := s.listNodesDB(ctx)
	if s.cache != nil && len(nodes) > 0 {
		s.setNodesCache(ctx, cacheKey, nodes, 5*time.Minute)
	}
	return nodes
}

func (s *Store) listNodesDB(ctx context.Context) []*model.Node {
	var nodes []*model.Node
	s.db.WithContext(ctx).Order("id ASC").Find(&nodes)
	return nodes
}

func (s *Store) GetNode(ctx context.Context, id int64) (*model.Node, bool) {
	var n model.Node
	if err := s.db.WithContext(ctx).First(&n, id).Error; err != nil {
		return nil, false
	}
	return &n, true
}

// DefaultNode 返回默认调度节点：优先取在线节点（按 ID 升序），否则取第一个节点。
// 用于任务未绑定节点时的回退，避免硬编码 worker 名称。
func (s *Store) DefaultNode(ctx context.Context) (*model.Node, bool) {
	var n model.Node
	// 优先在线节点
	if err := s.db.WithContext(ctx).Where("status IN ?", []string{"online", "busy"}).Order("id ASC").First(&n).Error; err == nil {
		return &n, true
	}
	// 回退任意第一个节点
	if err := s.db.WithContext(ctx).Order("id ASC").First(&n).Error; err == nil {
		return &n, true
	}
	return nil, false
}

// UpdateNodeMetrics 节点指标上报：通过 node_id 更新运行指标与心跳时间。
func (s *Store) UpdateNodeMetrics(ctx context.Context, nodeID string, cpu, mem, disk, load float64, running int) bool {
	result := s.db.WithContext(ctx).Model(&model.Node{}).
		Where("node_id = ?", nodeID).
		Updates(map[string]interface{}{
			"cpu_usage":      cpu,
			"memory_usage":   mem,
			"disk_usage":     disk,
			"load_average":   load,
			"running_tasks":  running,
			"last_heartbeat": time.Now(),
			"updated_at":     time.Now(),
		})
	if result.Error != nil {
		return false
	}
	// 更新后清除节点列表缓存，使 Dashboard 实时看到最新指标
	s.invalidateNodeCache(ctx)
	return result.RowsAffected > 0
}

// invalidateNodeCache 清除节点列表缓存。
func (s *Store) invalidateNodeCache(ctx context.Context) {
	if s.cache == nil {
		return
	}
	ctx, cancel := s.cacheCtx()
	defer cancel()
	_ = s.cache.Delete(ctx, cache.KeyNodeList())
}

// ---------- Logs ----------

func (s *Store) GetLogs(ctx context.Context, execID int64) []*model.TaskLog {
	var logs []*model.TaskLog
	s.db.WithContext(ctx).Where("execution_id = ?", execID).Order("sequence ASC").Find(&logs)
	return logs
}

func (s *Store) AppendLog(ctx context.Context, l *model.TaskLog) {
	l.CreatedAt = time.Now()
	s.db.WithContext(ctx).Create(l)
}

// ---------- Users ----------

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*model.User, bool) {
	var u model.User
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, false
	}
	return &u, true
}

// GetAllUsers 用于管理端用户列表。
func (s *Store) GetAllUsers(ctx context.Context) []*model.User {
	var users []*model.User
	s.db.WithContext(ctx).Order("id ASC").Find(&users)
	return users
}

// GetUserByID 按 ID 查用户。
func (s *Store) GetUserByID(ctx context.Context, id int64) (*model.User, bool) {
	var u model.User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, false
	}
	return &u, true
}

// CreateUser 创建用户（密码需已 bcrypt）。
func (s *Store) CreateUser(ctx context.Context, u *model.User) (*model.User, bool) {
	now := time.Now()
	u.ID = 0
	u.CreatedAt = now
	u.UpdatedAt = now
	if err := s.db.WithContext(ctx).Create(u).Error; err != nil {
		return nil, false
	}
	return u, true
}

// UpdateUser 更新用户（普通字段，不含密码）。
func (s *Store) UpdateUser(ctx context.Context, u *model.User) (*model.User, bool) {
	var old model.User
	if err := s.db.WithContext(ctx).First(&old, u.ID).Error; err != nil {
		return nil, false
	}
	u.CreatedAt = old.CreatedAt
	u.UpdatedAt = time.Now()
	// 保留旧密码，除非显式传入新密码
	if u.Password == "" {
		u.Password = old.Password
	}
	if err := s.db.WithContext(ctx).Save(u).Error; err != nil {
		return nil, false
	}
	return u, true
}

// UpdateUserPassword 更新用户密码。
func (s *Store) UpdateUserPassword(ctx context.Context, id int64, hashed string) bool {
	return s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("password", hashed).Error == nil
}

// DeleteUser 删除用户。
func (s *Store) DeleteUser(ctx context.Context, id int64) bool {
	res := s.db.WithContext(ctx).Delete(&model.User{}, id)
	if res.Error != nil {
		return false
	}
	return res.RowsAffected > 0
}

// ============================================================
// 缓存辅助方法
// 缓存操作使用独立短超时 context，避免 Redis 不可用时阻塞请求 context。
// ============================================================

// cacheCtx 返回缓存操作的独立 context（15s 超时），不共享请求 context。
func (s *Store) cacheCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

func (s *Store) getTasksCache(_ context.Context, key string) ([]*model.Task, bool) {
	ctx, cancel := s.cacheCtx()
	defer cancel()
	data, err := s.cache.Get(ctx, key)
	if err != nil || data == nil {
		return nil, false
	}
	var tasks []*model.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, false
	}
	return tasks, true
}

func (s *Store) setTasksCache(_ context.Context, key string, tasks []*model.Task, ttl time.Duration) {
	ctx, cancel := s.cacheCtx()
	defer cancel()
	data, err := json.Marshal(tasks)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, key, data, ttl)
}

func (s *Store) getNodesCache(_ context.Context, key string) ([]*model.Node, bool) {
	ctx, cancel := s.cacheCtx()
	defer cancel()
	data, err := s.cache.Get(ctx, key)
	if err != nil || data == nil {
		return nil, false
	}
	var nodes []*model.Node
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, false
	}
	return nodes, true
}

func (s *Store) setNodesCache(_ context.Context, key string, nodes []*model.Node, ttl time.Duration) {
	ctx, cancel := s.cacheCtx()
	defer cancel()
	data, err := json.Marshal(nodes)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, key, data, ttl)
}

func (s *Store) invalidateTasksCache(_ context.Context) {
	if s.cache == nil {
		return
	}
	ctx, cancel := s.cacheCtx()
	defer cancel()
	_ = s.cache.Delete(ctx, cache.KeyTaskList("", "", -1, 0, 0))
	_ = s.cache.Delete(ctx, cache.KeyDashboardOverview())
	_ = s.cache.Delete(ctx, cache.KeyDashboardCharts())
}

func (s *Store) invalidateDashboardCache(_ context.Context) {
	if s.cache == nil {
		return
	}
	ctx, cancel := s.cacheCtx()
	defer cancel()
	_ = s.cache.Delete(ctx, cache.KeyDashboardOverview())
	_ = s.cache.Delete(ctx, cache.KeyDashboardCharts())
	for _, rng := range []string{"day", "7d", "30d"} {
		_ = s.cache.Delete(ctx, cache.KeyDashboardTrend(rng))
	}
}

// Cache 返回缓存实例（供外部使用）。
func (s *Store) Cache() *cache.GlobalCache { return s.cache }
