package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"scheduler-platform/internal/executor"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/app/config"
	"scheduler-platform/pkg/ilog"
	"scheduler-platform/pkg/metric"
	"scheduler-platform/pkg/tracex"
)

// Scheduler 基于 robfig/cron 的任务调度器。
type Scheduler struct {
	cron     *cron.Cron
	store    *store.Store
	executor *executor.Executor
	mu       sync.Mutex
	// entryID -> taskID，便于启停
	entryTask map[cron.EntryID]int64
	taskEntry map[int64]cron.EntryID
	// 节点指标采集
	nodeScrapeStop chan struct{}
	nodeScrapeDone chan struct{}
}

// New 创建调度器。
func New(s *store.Store, ex *executor.Executor) *Scheduler {
	return &Scheduler{
		// 标准 5 字段 cron（分 时 日 月 周），与种子数据及任务配置保持一致
		cron:      cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow))),
		store:     s,
		executor:  ex,
		entryTask: make(map[cron.EntryID]int64),
		taskEntry: make(map[int64]cron.EntryID),
	}
}

// Start 启动并加载所有启用任务。
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.store.ListTasks(context.Background()) {
		if t.Status == model.TaskStatusEnabled && t.CronExpr != "" {
			s.addTaskLocked(t)
		}
	}
	s.cron.Start()
	ilog.Infof("scheduler started with %d jobs", len(s.entryTask))

	// 启动数据保留策略的每日定时清理（每天 03:07 执行，非业务任务）
	if _, err := s.cron.AddFunc("7 3 * * *", func() { s.runRetentionCleanup() }); err != nil {
		ilog.Warnf("register retention cleanup job failed: %v", err)
	}

	// 启动节点指标定时采集
	s.nodeScrapeStop = make(chan struct{})
	s.nodeScrapeDone = make(chan struct{})
	go s.nodeScrapeLoop()
}

// Stop 停止调度器。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 停止节点指标采集
	if s.nodeScrapeStop != nil {
		close(s.nodeScrapeStop)
		<-s.nodeScrapeDone
	}

	ctx := s.cron.Stop()
	<-ctx.Done()
	ilog.Info("scheduler stopped")
}

// nodeScrapeLoop 定时采集节点指标（每 30 秒）。
// 仅采集配置了 metricsUrl 的节点，失败不中断循环。
func (s *Scheduler) nodeScrapeLoop() {
	defer close(s.nodeScrapeDone)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	ilog.Info("node metrics scraper started (interval=30s)")

	// 启动后先采一次
	s.scrapeAllNodes()

	for {
		select {
		case <-ticker.C:
			s.scrapeAllNodes()
		case <-s.nodeScrapeStop:
			ilog.Info("node metrics scraper stopped")
			return
		}
	}
}

// scrapeAllNodes 采集所有配置了 metricsUrl 的节点指标。
func (s *Scheduler) scrapeAllNodes() {
	ctx := tracex.NewContext(context.Background())
	nodes := s.store.ListNodes(ctx)
	// 全局出站代理（若启用则经代理抓取）
	proxyClient := s.store.ProxyConfig(ctx).Client(10 * time.Second)
	success := 0
	for _, n := range nodes {
		if n.MetricsURL == "" {
			continue
		}
		result := metric.ScrapePrometheusWith(n.MetricsURL, 10*time.Second, proxyClient)
		if result.Error != "" {
			ilog.Trace(ctx).Warnf("scrape node %s (%s) failed: %s", n.Name, n.MetricsURL, result.Error)
			continue
		}
		s.store.UpdateNodeMetrics(ctx, n.NodeID,
			result.CPUUsage, result.MemoryUsage, result.DiskUsage, result.LoadAverage, n.RunningTasks)
		success++
	}
	if success > 0 {
		ilog.Trace(ctx).Debugf("scraped %d/%d nodes metrics", success, len(nodes))
	}
}

// SyncTask 任务变更后同步调度（新增/更新/启停）。
func (s *Scheduler) SyncTask(t *model.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeTaskLocked(t.ID)
	if t.Status == model.TaskStatusEnabled && t.CronExpr != "" {
		s.addTaskLocked(t)
	}
}

// RemoveTask 删除任务时移除调度。
func (s *Scheduler) RemoveTask(taskID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeTaskLocked(taskID)
}

func (s *Scheduler) addTaskLocked(t *model.Task) {
	entryID, err := s.cron.AddFunc(t.CronExpr, func() { s.execute(t.ID) })
	if err != nil {
		ilog.Warnf("register cron for task %d failed: %v", t.ID, err)
		return
	}
	s.entryTask[entryID] = t.ID
	s.taskEntry[t.ID] = entryID
	// 计算并回写下一次执行时间，供 Dashboard「即将执行」使用
	s.computeNextRunLocked(t)
}

// computeNextRunLocked 计算任务下一次执行时间并回写数据库，供 Dashboard「即将执行」使用。
func (s *Scheduler) computeNextRunLocked(t *model.Task) {
	if t.CronExpr == "" {
		return
	}
	next, err := s.nextRun(t.CronExpr, time.Now())
	if err != nil {
		ilog.Warnf("compute next run for task %d failed: %v", t.ID, err)
		return
	}
	t.NextRunTime = &next
	s.store.UpdateTaskNextRunTime(context.Background(), t.ID, next)
}

// nextRun 计算 cron 表达式自 from 之后的下一次执行时间。
// 使用独立解析器，避免依赖已启动的调度器实例。
func (s *Scheduler) nextRun(expr string, from time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}

// removeTaskLocked 移除任务的调度并清理映射。
func (s *Scheduler) removeTaskLocked(taskID int64) {
	if id, ok := s.taskEntry[taskID]; ok {
		s.cron.Remove(id)
		delete(s.taskEntry, taskID)
		delete(s.entryTask, id)
	}
}

// execute 调度触发：交给执行器真正执行并回写结果。
// cron 回调无 HTTP 请求，使用 tracex.NewContext 生成独立根链路，
// 贯穿到 executor 执行与通知日志，使每次调度可独立追踪。
func (s *Scheduler) execute(taskID int64) {
	ctx := tracex.NewContext(context.Background())
	t, ok := s.store.GetTask(ctx, taskID)
	if !ok || t.Status != model.TaskStatusEnabled {
		return
	}
	ilog.Trace(ctx).Infof("cron triggered task %d (%s)", t.ID, t.Name)
	if s.executor != nil {
		s.executor.Trigger(ctx, t)
	}
}

// runRetentionCleanup 按配置执行数据保留清理：删除超期的执行记录（保留最近 N 天）、
// 每个任务超量的旧记录，以及超过保留天数的节点指标采样。
func (s *Scheduler) runRetentionCleanup() {
	ctx := tracex.NewContext(context.Background())
	cfg := config.GetCurrentConfig()

	execDel, logDel := s.store.CleanupExecutions(ctx,
		cfg.Retention.ExecutionRetainDays, cfg.Retention.ExecutionMaxPerTask)
	if execDel > 0 || logDel > 0 {
		ilog.Trace(ctx).Infof("retention cleanup: deleted %d executions, %d logs", execDel, logDel)
	}

	nodeDel := s.store.CleanupNodeMetricsSamples(ctx, cfg.Retention.NodeMetricsRetainDays)
	if nodeDel > 0 {
		ilog.Trace(ctx).Infof("retention cleanup: deleted %d node metric samples", nodeDel)
	}
}
