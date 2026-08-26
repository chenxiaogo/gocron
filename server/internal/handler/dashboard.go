package handler

import (
	"net/http"

	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/service"
	"scheduler-platform/pkg/tracex"

	"github.com/gin-gonic/gin"
)

// DashboardHandler 首页 Dashboard 接口。
type DashboardHandler struct {
	svc *service.Service
}

func NewDashboardHandler(svc *service.Service) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// RegisterRoute 注册 Dashboard 路由。
// 全部使用 dashboard:view 权限点（viewer 等只读角色可访问，保证权限体系一致）。
func (h *DashboardHandler) RegisterRoute(g *gin.RouterGroup) {
	group := g.Group("/dashboard")
	{
		group.GET("/overview", middleware.RequirePerm(model.PermDashboardView), h.Overview)
		group.GET("/trend", middleware.RequirePerm(model.PermDashboardView), h.Trend)
		group.GET("/failures", middleware.RequirePerm(model.PermDashboardView), h.Failures)
		group.GET("/recent", middleware.RequirePerm(model.PermDashboardView), h.Recent)
		group.GET("/upcoming", middleware.RequirePerm(model.PermDashboardView), h.Upcoming)
		group.GET("/resources", middleware.RequirePerm(model.PermDashboardView), h.Resources)
		group.GET("/cluster", middleware.RequirePerm(model.PermDashboardView), h.Cluster)
		group.GET("/charts", middleware.RequirePerm(model.PermDashboardView), h.Charts)
	}
}

// Overview 指标卡数据。
func (h *DashboardHandler) Overview(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Overview(tracex.FromRequest(c)))
}

// Trend 执行趋势。
func (h *DashboardHandler) Trend(c *gin.Context) {
	rng := c.DefaultQuery("range", "7d")
	c.JSON(http.StatusOK, h.svc.Trend(tracex.FromRequest(c), rng))
}

// Failures 异常任务。
func (h *DashboardHandler) Failures(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Failures(tracex.FromRequest(c)))
}

// Recent 最近执行记录。
func (h *DashboardHandler) Recent(c *gin.Context) {
	execs := h.svc.RecentExecutions(tracex.FromRequest(c), 10)
	c.JSON(http.StatusOK, execs)
}

// Upcoming 即将执行。
func (h *DashboardHandler) Upcoming(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Upcoming(tracex.FromRequest(c)))
}

// Resources 节点资源概览。
func (h *DashboardHandler) Resources(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.NodeResources(tracex.FromRequest(c)))
}

// Cluster 集群聚合资源统计。
func (h *DashboardHandler) Cluster(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.ClusterStatsAgg(tracex.FromRequest(c)))
}

// Charts 图表分布统计。
func (h *DashboardHandler) Charts(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Charts(tracex.FromRequest(c)))
}