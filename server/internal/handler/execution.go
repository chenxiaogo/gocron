package handler

import (
	"net/http"
	"strconv"

	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/store"
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
}

// List 执行记录列表（分页 + 筛选）。
// @Summary 执行记录
// @Tags execution
// @Param taskId query int false "任务ID"
// @Param status query string false "状态"
// @Param trigger query string false "触发方式"
// @Param page query int false "页码"
// @Param pageSize query int false "每页数量"
// @Success 200 {object} map[string]interface{}
// @Router /api/executions [get]
func (h *ExecutionHandler) List(c *gin.Context) {
	taskID, _ := strconv.ParseInt(c.Query("taskId"), 10, 64)
	status := c.Query("status")
	trigger := c.Query("trigger")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	execs, total := h.store.PageExecutions(tracex.FromRequest(c), taskID, status, trigger, page, pageSize)
	c.JSON(http.StatusOK, gin.H{
		"list":  execs,
		"total": total,
		"page":  page,
		"pageSize": pageSize,
	})
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
