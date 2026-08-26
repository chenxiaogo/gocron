package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"scheduler-platform/internal/executor"
	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/scheduler"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/ilog"
	"scheduler-platform/pkg/tracex"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

// TaskHandler 任务管理接口。
type TaskHandler struct {
	store    *store.Store
	sched    *scheduler.Scheduler
	executor *executor.Executor
}

func NewTaskHandler(s *store.Store, sched *scheduler.Scheduler, ex *executor.Executor) *TaskHandler {
	return &TaskHandler{store: s, sched: sched, executor: ex}
}

// syncScheduler 任务变更后同步调度器。
func (h *TaskHandler) syncScheduler(t *model.Task) {
	if h.sched != nil {
		h.sched.SyncTask(t)
	}
}

// RegisterRoute 注册任务路由。
func (h *TaskHandler) RegisterRoute(g *gin.RouterGroup) {
	group := g.Group("/tasks")
	{
		group.GET("", middleware.RequirePerm(model.PermTaskView), h.List)
		group.GET("/:id", middleware.RequirePerm(model.PermTaskView), h.Get)
		group.POST("", middleware.RequirePerm(model.PermTaskCreate), h.Create)
		group.PUT("/:id", middleware.RequirePerm(model.PermTaskUpdate), h.Update)
		group.DELETE("/:id", middleware.RequirePerm(model.PermTaskDelete), h.Delete)
		group.POST("/:id/run", middleware.RequirePerm(model.PermTaskRun), h.Run)
		group.POST("/:id/enable", middleware.RequirePerm(model.PermTaskRun), h.Enable)
		group.POST("/:id/disable", middleware.RequirePerm(model.PermTaskRun), h.Disable)
	}
}

// List 任务列表（分页 + SQL 筛选）。
// @Summary 任务列表
// @Tags task
// @Param keyword query string false "关键字"
// @Param type query string false "任务类型"
// @Param status query int false "状态"
// @Param page query int false "页码"
// @Param pageSize query int false "每页数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/tasks [get]
func (h *TaskHandler) List(c *gin.Context) {
	keyword := c.Query("keyword")
	ttype := c.Query("type")
	statusStr := c.DefaultQuery("status", "-1")
	status, err := strconv.Atoi(statusStr)
	if err != nil || statusStr == "" {
		status = -1 // 空字符串视为不按状态过滤
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	tasks, total := h.store.PageTasks(tracex.FromRequest(c), keyword, ttype, status, page, pageSize)
	c.JSON(http.StatusOK, gin.H{
		"list":     tasks,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// Get 任务详情。
// @Summary 任务详情
// @Tags task
// @Param id path int true "任务ID"
// @Success 200 {object} model.Task
// @Router /api/tasks/:id [get]
func (h *TaskHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	t, ok := h.store.GetTask(tracex.FromRequest(c), id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "任务不存在"})
		return
	}
	c.JSON(http.StatusOK, t)
}

// Create 创建任务。
func (h *TaskHandler) Create(c *gin.Context) {
	var t model.Task
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": err.Error()})
		return
	}
	if t.Code == "" {
		t.Code = "T" + time.Now().Format("20060102150405")
	}
	if t.Status != model.TaskStatusEnabled {
		t.Status = model.TaskStatusDisabled
	}
	// 启用状态 + cron 类型时，校验 cron 表达式，避免无效表达式静默不调度
	if t.Status == model.TaskStatusEnabled && t.CronExpr != "" {
		if err := validateCronExpr(t.CronExpr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "cron 表达式无效: " + err.Error()})
			return
		}
	}
	if err := validateTaskBasics(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": err.Error()})
		return
	}
	_, username, _ := middleware.CurrentUser(c)
	t.CreatedBy = username
	created := h.store.CreateTask(tracex.FromRequest(c), &t)
	if created != nil {
		h.syncScheduler(created)
		ilog.FromGin(c).Infof("用户 %s 创建任务 #%d (%s, type=%s)", username, created.ID, created.Name, created.Type)
	} else {
		ilog.FromGin(c).Errorf("用户 %s 创建任务失败 (%s)", username, t.Name)
	}
	c.JSON(http.StatusOK, created)
}

// Update 更新任务。
func (h *TaskHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var t model.Task
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": err.Error()})
		return
	}
	t.ID = id
	// 启用状态 + cron 类型时校验 cron 表达式
	if t.Status == model.TaskStatusEnabled && t.CronExpr != "" {
		if err := validateCronExpr(t.CronExpr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "cron 表达式无效: " + err.Error()})
			return
		}
	}
	if err := validateTaskBasics(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": err.Error()})
		return
	}
	updated, ok := h.store.UpdateTask(tracex.FromRequest(c), &t)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "任务不存在"})
		return
	}
	h.syncScheduler(updated)
	_, username, _ := middleware.CurrentUser(c)
	ilog.FromGin(c).Infof("用户 %s 更新任务 #%d (%s)", username, updated.ID, updated.Name)
	c.JSON(http.StatusOK, updated)
}

// Delete 删除任务（高危操作）。
func (h *TaskHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if !h.store.DeleteTask(tracex.FromRequest(c), id) {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "任务不存在"})
		return
	}
	if h.sched != nil {
		h.sched.RemoveTask(id)
	}
	_, username, _ := middleware.CurrentUser(c)
	ilog.FromGin(c).Infof("用户 %s 删除任务 #%d", username, id)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "删除成功"})
}

// Run 手动执行任务（同步执行，等待完成再返回结果）。
func (h *TaskHandler) Run(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	t, ok := h.store.GetTask(tracex.FromRequest(c), id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "任务不存在"})
		return
	}
	if h.executor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "执行器未初始化"})
		return
	}
	_, username, _ := middleware.CurrentUser(c)
	ctx := tracex.NewContext(context.Background())
	execID, status, msg, duration := h.executor.RunSync(ctx, t)
	ilog.Trace(ctx).Infof("用户 %s 手动执行任务 #%d (%s) status=%s duration=%dms", username, t.ID, t.Name, status, duration)
	c.JSON(http.StatusOK, gin.H{
		"code":        0,
		"message":     "执行完成",
		"executionId": execID,
		"status":      status,
		"errorMsg":    msg,
		"durationMs":  duration,
	})
}

// Enable 启用任务。
func (h *TaskHandler) Enable(c *gin.Context) {
	h.changeStatus(c, model.TaskStatusEnabled)
}

// Disable 停用任务。
func (h *TaskHandler) Disable(c *gin.Context) {
	h.changeStatus(c, model.TaskStatusDisabled)
}

func (h *TaskHandler) changeStatus(c *gin.Context, status int) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	t, ok := h.store.GetTask(tracex.FromRequest(c), id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "任务不存在"})
		return
	}
	t.Status = status
	h.store.UpdateTask(tracex.FromRequest(c), t)
	h.syncScheduler(t)
	_, username, _ := middleware.CurrentUser(c)
	action := "启用"
	if status == model.TaskStatusDisabled {
		action = "停用"
	}
	ilog.FromGin(c).Infof("用户 %s %s任务 #%d (%s)", username, action, t.ID, t.Name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "操作成功"})
}

// validateCronExpr 校验标准 5 字段 cron 表达式。
func validateCronExpr(expr string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err
}

// validateTaskBasics 校验任务基本字段的合理性。
func validateTaskBasics(t *model.Task) error {
	if t.Name == "" {
		return fmt.Errorf("任务名称不能为空")
	}
	switch t.Type {
	case model.TaskTypeCron:
		if t.CronExpr == "" {
			return fmt.Errorf("cron 类型任务必须填写 cron 表达式")
		}
	case model.TaskTypeHTTP, model.TaskTypeGRPC:
		if !strings.Contains(t.ExecutorConfig, `"url"`) || !strings.Contains(t.ExecutorConfig, `"url":"`) {
			// 宽松：仅提示，url 允许运行时校验
		}
	case model.TaskTypeShell:
		if !strings.Contains(t.ExecutorConfig, "command") {
			return fmt.Errorf("shell 类型任务必须配置执行命令")
		}
	default:
		return fmt.Errorf("不支持的任务类型: %s", t.Type)
	}
	if t.RetryCount < 0 {
		return fmt.Errorf("重试次数不能为负数")
	}
	if t.RetryInterval < 0 {
		return fmt.Errorf("重试间隔不能为负数")
	}
	return nil
}


