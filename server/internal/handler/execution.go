package handler

import (
	"net/http"
	"strconv"
	"time"

	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/app/config"
	"scheduler-platform/pkg/tracex"

	"github.com/gin-gonic/gin"
)

// ExecutionHandler 执行记录接口。
type ExecutionHandler struct {
	store *store.Store
}

func NewExecutionHandler(s *store.Store) *ExecutionHandler {
	return &ExecutionHandler{store: s}
}

// RegisterRoute 注册执行记录路由。
func (h *ExecutionHandler) RegisterRoute(g *gin.RouterGroup) {
	group := g.Group("/executions")
	{
		group.GET("", middleware.RequirePerm(model.PermExecView), h.List)
		group.GET("/:id", middleware.RequirePerm(model.PermExecView), h.Get)
		group.GET("/:id/logs", middleware.RequirePerm(model.PermLogView), h.Logs)
	}
	// 单任务执行统计与历史
	group.GET("/task/:taskId/stats", middleware.RequirePerm(model.PermExecView), h.TaskStats)
	group.GET("/task/:taskId/history", middleware.RequirePerm(model.PermExecView), h.TaskHistory)
	// 保留策略与手动清理（每日 03:07 由后台定时任务自动执行）
	group.GET("/retention", middleware.RequirePerm(model.PermExecView), h.Retention)
	group.POST("/cleanup", middleware.RequirePerm(model.PermSystemAdmin), h.Cleanup)
}

// List 执行记录列表（分页 + 筛选）。
// @Summary 执行记录
// @Tags execution
// @Param taskId query int false "任务ID"
// @Param keyword query string false "任务名称关键词"
// @Param worker query string false "节点名称"
// @Param status query string false "状态"
// @Param trigger query string false "触发方式"
// @Param start query string false "开始时间下限（RFC3339 或 2006-01-02T15:04:05）"
// @Param end query string false "开始时间上限（RFC3339 或 2006-01-02T15:04:05）"
// @Param page query int false "页码"
// @Param pageSize query int false "每页数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/executions [get]
func (h *ExecutionHandler) List(c *gin.Context) {
	taskID, _ := strconv.ParseInt(c.Query("taskId"), 10, 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	q := store.ExecutionQuery{
		TaskID:   taskID,
		Keyword:  c.Query("keyword"),
		Worker:   c.Query("worker"),
		Status:   c.Query("status"),
		Trigger:  c.Query("trigger"),
		StartAt:  parseQueryTime(c.Query("start")),
		EndAt:    parseQueryTime(c.Query("end")),
		Page:     page,
		PageSize: pageSize,
	}

	execs, total := h.store.PageExecutions(tracex.FromRequest(c), q)
	c.JSON(http.StatusOK, gin.H{
		"list":     execs,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// parseQueryTime 解析时间范围查询参数，兼容 RFC3339 与本地时间格式。
func parseQueryTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Get 执行详情。
func (h *ExecutionHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	ex, ok := h.store.GetExecution(tracex.FromRequest(c), id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "执行记录不存在"})
		return
	}
	c.JSON(http.StatusOK, ex)
}

// Logs 执行日志。
func (h *ExecutionHandler) Logs(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	c.JSON(http.StatusOK, h.store.GetLogs(tracex.FromRequest(c), id))
}

// TaskStats 单任务执行统计。
func (h *ExecutionHandler) TaskStats(c *gin.Context) {
	taskID, _ := strconv.ParseInt(c.Param("taskId"), 10, 64)
	if taskID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "taskId 无效"})
		return
	}
	c.JSON(http.StatusOK, h.store.TaskExecStatsByID(tracex.FromRequest(c), taskID))
}

// TaskHistory 单任务最近执行记录。
func (h *ExecutionHandler) TaskHistory(c *gin.Context) {
	taskID, _ := strconv.ParseInt(c.Param("taskId"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if taskID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "taskId 无效"})
		return
	}
	c.JSON(http.StatusOK, h.store.ListRecentExecsByTask(tracex.FromRequest(c), taskID, limit))
}

// Retention 返回当前执行记录与指标采样的数据保留策略。
func (h *ExecutionHandler) Retention(c *gin.Context) {
	cfg := config.GetCurrentConfig().Retention
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": cfg})
}

// Cleanup 手动触发执行记录清理（保留最近 N 天 + 每任务超量删除）。
// 不传参数时沿用配置文件中的保留策略。
func (h *ExecutionHandler) Cleanup(c *gin.Context) {
	var body struct {
		RetainDays int `json:"retainDays"`
		MaxPerTask int `json:"maxPerTask"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.RetainDays <= 0 {
		body.RetainDays = config.GetCurrentConfig().Retention.ExecutionRetainDays
	}
	if body.MaxPerTask < 0 {
		body.MaxPerTask = 0
	}
	execDel, logDel := h.store.CleanupExecutions(tracex.FromRequest(c), body.RetainDays, body.MaxPerTask)
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"deletedExecutions": execDel, "deletedLogs": logDel},
	})
}
