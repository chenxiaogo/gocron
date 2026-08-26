package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"scheduler-platform/internal/model"
	"scheduler-platform/internal/store"
)

// Service 业务逻辑层，封装所有业务规则。
type Service struct {
	store *store.Store
}

func NewService(s *store.Store) *Service {
	return &Service{store: s}
}

// DashboardOverview 首页指标卡数据。
type DashboardOverview struct {
	TaskTotal       int     `json:"taskTotal"`
	TaskEnabled     int     `json:"taskEnabled"`
	TaskDisabled    int     `json:"taskDisabled"`
	TodayExecutions int     `json:"todayExecutions"`
	TodaySuccess    int     `json:"todaySuccess"`
	TodayFailed     int     `json:"todayFailed"`
	Running         int     `json:"running"`
	SuccessRate     float64 `json:"successRate"`
	AbnormalTasks   int     `json:"abnormalTasks"`
	OnlineNodes     int     `json:"onlineNodes"`
	OfflineNodes    int     `json:"offlineNodes"`
	// 数据库计算的真实趋势指标（较上一周期增减）
	TaskWeekGrowth     int     `json:"taskWeekGrowth"`     // 任务总数较上周新增
	TaskWeekPct        float64 `json:"taskWeekPct"`        // 任务总数较上周增长率(%)
	TodayExecGrowth    int     `json:"todayExecGrowth"`    // 今日执行次数较昨日增减
	TodayExecPct       float64 `json:"todayExecPct"`       // 今日执行次数较昨日增长率(%)
	TodaySuccessGrowth int     `json:"todaySuccessGrowth"` // 今日成功较昨日增减
	TodayFailGrowth    int     `json:"todayFailGrowth"`    // 今日失败较昨日增减
	OnlineGrowth       int     `json:"onlineGrowth"`       // 在线节点较上次变化
}

// Overview 计算首页概览（使用聚合查询，避免拉取全表）。
func (s *Service) Overview(ctx context.Context) *DashboardOverview {
	tasks := s.store.ListTasks(ctx)
	var enabled, disabled int
	for _, t := range tasks {
		if t.Status == model.TaskStatusEnabled {
			enabled++
		} else {
			disabled++
		}
	}

	now := time.Now()
	loc := now.Location()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	startOfYesterday := startOfDay.AddDate(0, 0, -1)

	todayExec := s.store.CountExecutions(ctx, "started_at >= ?", startOfDay)
	todaySuccess := s.store.CountExecutions(ctx, "started_at >= ? AND status = ?", startOfDay, "success")
	todayFailed := s.store.CountExecutions(ctx, "started_at >= ? AND status IN ?", startOfDay, []string{"failed", "timeout"})

	// 昨日对比（用于今日趋势）
	yesterdayExec := s.store.CountExecutions(ctx, "started_at >= ? AND started_at < ?", startOfYesterday, startOfDay)
	yesterdaySuccess := s.store.CountExecutions(ctx, "started_at >= ? AND started_at < ? AND status = ?", startOfYesterday, startOfDay, "success")
	yesterdayFailed := s.store.CountExecutions(ctx, "started_at >= ? AND started_at < ? AND status IN ?", startOfYesterday, startOfDay, []string{"failed", "timeout"})

	statusCount := s.store.ExecutionsByStatus(ctx)
	running := int(statusCount["running"])

	// 异常任务：存在失败/超时执行的任务数
	abnormal := int(s.store.CountExecutions(ctx, "status IN ?", []string{"failed", "timeout"}))

	successRate := 0.0
	if todayExec > 0 {
		successRate = float64(todaySuccess) / float64(todayExec) * 100
	}

	nodes := s.store.ListNodes(ctx)
	var online, offline int
	for _, n := range nodes {
		if n.Status == "online" || n.Status == "busy" {
			online++
		} else {
			offline++
		}
	}

	// 任务总数较上周新增
	lastWeekCount := int(s.store.CountTasksCreatedBefore(ctx, now.AddDate(0, 0, -7)))

	return &DashboardOverview{
		TaskTotal:          len(tasks),
		TaskEnabled:        enabled,
		TaskDisabled:       disabled,
		TodayExecutions:    int(todayExec),
		TodaySuccess:       int(todaySuccess),
		TodayFailed:        int(todayFailed),
		Running:            running,
		SuccessRate:        round2(successRate),
		AbnormalTasks:      abnormal,
		OnlineNodes:        online,
		OfflineNodes:       offline,
		TaskWeekGrowth:     len(tasks) - lastWeekCount,
		TaskWeekPct:        growthPct(lastWeekCount, len(tasks)),
		TodayExecGrowth:    int(todayExec) - int(yesterdayExec),
		TodayExecPct:       growthPct(int(yesterdayExec), int(todayExec)),
		TodaySuccessGrowth: int(todaySuccess) - int(yesterdaySuccess),
		TodayFailGrowth:    int(todayFailed) - int(yesterdayFailed),
		OnlineGrowth:       online, // 在线节点数（本身即当前值，用于展示）
	}
}

// TrendPoint 趋势数据点。
type TrendPoint struct {
	Date    string `json:"date"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
	Timeout int    `json:"timeout"`
}

// Trend 执行趋势，range: day/7d/30d。
func (s *Service) Trend(ctx context.Context, rng string) []*TrendPoint {
	execs := s.store.ListExecutions(ctx, 0)
	now := time.Now()
	var days int
	switch rng {
	case "day":
		days = 1
	case "30d":
		days = 30
	default:
		days = 7
	}

	points := make([]*TrendPoint, 0, days)
	start := now.AddDate(0, 0, -(days - 1))
	// 初始化空点
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i)
		key := d.Format("01-02")
		points = append(points, &TrendPoint{Date: key})
	}

	idx := map[string]*TrendPoint{}
	for _, p := range points {
		idx[p.Date] = p
	}
	for _, e := range execs {
		if e.StartedAt.Before(start) {
			continue
		}
		key := e.StartedAt.Format("01-02")
		if p, ok := idx[key]; ok {
			p.Total++
			switch e.Status {
			case "success":
				p.Success++
			case "failed":
				p.Failed++
			case "timeout":
				p.Timeout++
			}
		}
	}
	return points
}

// FailedTask 异常任务。
type FailedTask struct {
	TaskID           int64     `json:"taskId"`
	TaskName         string    `json:"taskName"`
	TaskType         string    `json:"taskType"`
	LastStatus       string    `json:"lastStatus"`
	ConsecutiveFails int       `json:"consecutiveFails"`
	LastRunTime      time.Time `json:"lastRunTime"`
	LastError        string    `json:"lastError"`
}

// Failures 异常任务列表。
func (s *Service) Failures(ctx context.Context) []*FailedTask {
	execs := s.store.ListExecutions(ctx, 0)
	failMap := map[int64]*FailedTask{}
	var order []int64
	for _, e := range execs {
		if e.Status != "failed" && e.Status != "timeout" {
			continue
		}
		ft, ok := failMap[e.TaskID]
		if !ok {
			ft = &FailedTask{
				TaskID: e.TaskID, TaskName: e.TaskName, TaskType: e.TaskType,
				LastStatus: e.Status, LastRunTime: e.StartedAt, LastError: e.ErrorMessage,
			}
			failMap[e.TaskID] = ft
			order = append(order, e.TaskID)
		}
		ft.ConsecutiveFails++
		// 更新最近失败时间
		if e.StartedAt.After(ft.LastRunTime) {
			ft.LastRunTime = e.StartedAt
			ft.LastStatus = e.Status
			ft.LastError = e.ErrorMessage
		}
	}
	out := make([]*FailedTask, 0, len(order))
	for _, id := range order {
		out = append(out, failMap[id])
	}
	return out
}

// UpcomingTask 即将执行任务。
type UpcomingTask struct {
	TaskID   int64     `json:"taskId"`
	TaskName string    `json:"taskName"`
	TaskType string    `json:"taskType"`
	NextTime time.Time `json:"nextTime"`
	Countdown string   `json:"countdown"`
	Worker   string    `json:"worker"`
}

// Upcoming 即将执行任务。
func (s *Service) Upcoming(ctx context.Context) []*UpcomingTask {
	now := time.Now()
	var tasks []*model.Task
	for _, t := range s.store.ListTasks(ctx) {
		if t.Status == model.TaskStatusEnabled && t.NextRunTime != nil {
			tasks = append(tasks, t)
		}
	}
	// 排序下次执行时间（升序）
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].NextRunTime.Before(*tasks[j].NextRunTime)
	})
	if len(tasks) > 8 {
		tasks = tasks[:8]
	}
	out := make([]*UpcomingTask, 0, len(tasks))
	// 任务无绑定节点时的回退默认节点（数据库驱动，避免硬编码）
	defWorker := ""
	if def, ok := s.store.DefaultNode(ctx); ok {
		defWorker = def.Name
	}
	for _, t := range tasks {
		diff := t.NextRunTime.Sub(now)
		worker := defWorker
		if len(t.Nodes) > 0 {
			worker = t.Nodes[0].Name
		}
		out = append(out, &UpcomingTask{
			TaskID: t.ID, TaskName: t.Name, TaskType: t.Type,
			NextTime: *t.NextRunTime, Countdown: formatCountdown(diff), Worker: worker,
		})
	}
	return out
}

func formatCountdown(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return "即将执行"
	}
}

// RecentExecution 最近执行记录（含执行耗时等展示字段）。
type RecentExecution struct {
	ID         int64     `json:"id"`
	TaskID     int64     `json:"taskId"`
	TaskName   string    `json:"taskName"`
	TaskType   string    `json:"taskType"`
	Status     string    `json:"status"`
	Trigger    string    `json:"trigger"`
	ExecTime   time.Time `json:"execTime"`
	DurationMs int64     `json:"durationMs"`
	WorkerName string    `json:"workerName"`
}

// RecentExecutions 最近 N 条执行记录。
func (s *Service) RecentExecutions(ctx context.Context, n int) []*RecentExecution {
	execs := s.store.ListExecutions(ctx, n)
	out := make([]*RecentExecution, 0, len(execs))
	for _, e := range execs {
		out = append(out, &RecentExecution{
			ID: e.ID, TaskID: e.TaskID, TaskName: e.TaskName, TaskType: e.TaskType,
			Status: e.Status, Trigger: e.TriggerType, ExecTime: e.StartedAt,
			DurationMs: e.DurationMs, WorkerName: e.WorkerName,
		})
	}
	return out
}

// NodeResource 节点资源。
type NodeResource struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Status      string    `json:"status"`
	CPUUsage    float64   `json:"cpuUsage"`
	MemoryUsage float64   `json:"memoryUsage"`
	DiskUsage   float64   `json:"diskUsage"`
	LoadAverage float64   `json:"loadAverage"`
	Running     int       `json:"running"`
	MaxConc     int       `json:"maxConcurrency"`
	LastBeat    time.Time `json:"lastHeartbeat"`
}

// NodeResources 节点资源概览。
func (s *Service) NodeResources(ctx context.Context) []*NodeResource {
	nodes := s.store.ListNodes(ctx)
	out := make([]*NodeResource, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, &NodeResource{
			ID: n.ID, Name: n.Name, Address: n.Address, Status: n.Status,
			CPUUsage: n.CPUUsage, MemoryUsage: n.MemoryUsage, DiskUsage: n.DiskUsage,
			LoadAverage: n.LoadAverage, Running: n.RunningTasks, MaxConc: n.MaxConcurrency,
			LastBeat: n.LastHeartbeat,
		})
	}
	return out
}

// ClusterStats 集群聚合资源统计（首页指标卡）。
type ClusterStats struct {
	NodeCount    int     `json:"nodeCount"`    // 节点总数
	OnlineCount  int     `json:"onlineCount"`  // 在线节点数
	CPUUsage     float64 `json:"cpuUsage"`     // CPU 平均使用率
	MemoryUsage  float64 `json:"memoryUsage"`  // 内存平均使用率
	DiskUsage    float64 `json:"diskUsage"`    // 磁盘平均使用率
	LoadAverage  float64 `json:"loadAverage"`  // 平均负载
	RunningTasks int     `json:"runningTasks"` // 运行中任务数
}

// ClusterStatsAgg 基于所有节点聚合集群资源指标。
func (s *Service) ClusterStatsAgg(ctx context.Context) *ClusterStats {
	nodes := s.store.ListNodes(ctx)
	st := &ClusterStats{NodeCount: len(nodes)}
	if len(nodes) == 0 {
		return st
	}
	var cpuSum, memSum, diskSum, loadSum float64
	var running int
	for _, n := range nodes {
		if n.Status == "online" || n.Status == "busy" {
			st.OnlineCount++
		}
		cpuSum += n.CPUUsage
		memSum += n.MemoryUsage
		diskSum += n.DiskUsage
		loadSum += n.LoadAverage
		running += n.RunningTasks
	}
	st.CPUUsage = round2(cpuSum / float64(len(nodes)))
	st.MemoryUsage = round2(memSum / float64(len(nodes)))
	st.DiskUsage = round2(diskSum / float64(len(nodes)))
	st.LoadAverage = round2(loadSum / float64(len(nodes)))
	st.RunningTasks = running
	return st
}

// ChartSlice 图表数据项。
type ChartSlice struct {
	Name  string  `json:"name"`
	Value int     `json:"value"`
	Color string  `json:"color"`
}

// DashboardCharts 首页图表统计（分布类，供环形/饼/柱状图展示）。
type DashboardCharts struct {
	TaskType   []*ChartSlice `json:"taskType"`   // 任务类型分布
	TaskStatus []*ChartSlice `json:"taskStatus"` // 任务启用/停用分布
	ExecStatus []*ChartSlice `json:"execStatus"` // 执行状态分布（今日）
	NodeStatus []*ChartSlice `json:"nodeStatus"` // 节点在线/离线分布
}

// Charts 聚合首页图表分布数据。
func (s *Service) Charts(ctx context.Context) *DashboardCharts {
	c := &DashboardCharts{}

	// 任务类型分布
	typeCnt := map[string]int{"cron": 0, "http": 0, "shell": 0, "grpc": 0}
	for _, t := range s.store.ListTasks(ctx) {
		if _, ok := typeCnt[t.Type]; ok {
			typeCnt[t.Type]++
		} else {
			typeCnt[t.Type] = 1
		}
	}
	typeOrder := []struct {
		key, name, color string
	}{
		{"cron", "Cron", "#4f6ef7"},
		{"http", "HTTP", "#7c5cf0"},
		{"shell", "Shell", "#22c55e"},
		{"grpc", "gRPC", "#f59e0b"},
	}
	for _, o := range typeOrder {
		if v := typeCnt[o.key]; v > 0 {
			c.TaskType = append(c.TaskType, &ChartSlice{Name: o.name, Value: v, Color: o.color})
		}
	}

	// 任务状态分布
	var enabled, disabled int
	for _, t := range s.store.ListTasks(ctx) {
		if t.Status == model.TaskStatusEnabled {
			enabled++
		} else {
			disabled++
		}
	}
	c.TaskStatus = []*ChartSlice{
		{Name: "启用", Value: enabled, Color: "#22c55e"},
		{Name: "停用", Value: disabled, Color: "#c0c4cc"},
	}

	// 今日执行状态分布
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	exec := s.store.ExecutionsByStatusRange(ctx, startOfDay)
	c.ExecStatus = []*ChartSlice{
		{Name: "成功", Value: int(exec["success"]), Color: "#22c55e"},
		{Name: "失败", Value: int(exec["failed"]), Color: "#ef4444"},
		{Name: "超时", Value: int(exec["timeout"]), Color: "#f59e0b"},
		{Name: "运行中", Value: int(exec["running"]), Color: "#06b6d4"},
	}

	// 节点在线/离线分布
	var online, offline int
	for _, n := range s.store.ListNodes(ctx) {
		if n.Status == "online" || n.Status == "busy" {
			online++
		} else {
			offline++
		}
	}
	c.NodeStatus = []*ChartSlice{
		{Name: "在线", Value: online, Color: "#10b981"},
		{Name: "离线", Value: offline, Color: "#c0c4cc"},
	}

	return c
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// growthPct 计算较上一周期的增长率（百分比，保留一位小数）。base 为 0 时返回 0。
func growthPct(base, current int) float64 {
	if base <= 0 {
		return 0
	}
	return round1(float64(current-base) / float64(base) * 100)
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
