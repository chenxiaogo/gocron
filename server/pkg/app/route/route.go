package route

import (
	"context"
	"net/http"

	"scheduler-platform/internal/executor"
	"scheduler-platform/internal/handler"
	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/scheduler"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/app/config"
	"scheduler-platform/pkg/ilog"
	"scheduler-platform/pkg/metric"
	"scheduler-platform/pkg/tracex"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registrar 业务 Handler 实现该接口以注册路由。
type Registrar interface {
	RegisterRoute(*gin.RouterGroup)
}

// Route 封装 Gin 路由。
type Route struct {
	engine *gin.Engine
	logic  *handler.Logic
}

// NewRoute 创建路由并注册所有 Handler。
func NewRoute(ctx context.Context, conf *config.Config, store *store.Store, sched *scheduler.Scheduler, ex *executor.Executor) (*Route, error) {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(tracex.TraceMiddleware())
	engine.Use(ilogMiddleware())
	engine.Use(corsMiddleware())

	// Prometheus 指标端点
	if conf.Metrics.Enabled {
		mm := metric.GetMetricManager()
		mm.InitializeDefaultMetrics()
		mm.SetLogger(ilog.GetLogger())
		engine.GET(conf.Metrics.Path, gin.WrapH(promhttp.Handler()))
		ilog.Infof("metrics endpoint enabled at %s", conf.Metrics.Path)
	}

	r := &Route{engine: engine}
	r.logic = handler.NewLogic(ctx, conf, store, sched, ex)

	api := engine.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 根路径 ping
	engine.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	// 认证路由（无需登录）
	authGroup := api.Group("/auth")
	r.logic.Auth.RegisterRoute(authGroup)

	// 节点指标上报（公开接口，无需登录，供节点 agent 使用）
	r.logic.Node.RegisterPublicRoute(api)

	// 业务路由：挂载 JWT 鉴权 + Casbin API 权限中间件
	authMiddleware := middleware.Auth(r.logic.JWT)
	protected := api.Group("")
	protected.Use(authMiddleware)
	protected.Use(middleware.CasbinAuth(store.Enforcer, store))

	// 需登录的认证路由（当前用户信息）
	r.logic.Auth.RegisterProtectedRoute(protected.Group("/auth"))

	for _, reg := range []Registrar{
		r.logic.Dashboard,
		r.logic.Task,
		r.logic.Execution,
		r.logic.Node,
		r.logic.User,
		r.logic.Role,
		r.logic.Menu,
		r.logic.Api,
		r.logic.Notify,
	} {
		reg.RegisterRoute(protected)
	}

	return r, nil
}

// Handler 返回 HTTP Handler。
func (r *Route) Handler() http.Handler {
	return r.engine
}

// AttachHandler 挂载额外的 HTTP 处理逻辑（如静态文件 SPA）。
func (r *Route) AttachHandler(h func(w http.ResponseWriter, req *http.Request)) {
	r.engine.NoRoute(func(c *gin.Context) {
		h(c.Writer, c.Request)
	})
}

func ilogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		tid := tracex.TraceIDShort(c.Request.Context())
		ilog.Infof("[%s] %s %s %d", tid, c.Request.Method, c.Request.URL.Path, c.Writer.Status())
	}
}

func corsMiddleware() gin.HandlerFunc {
	cfg := cors.DefaultConfig()
	cfg.AllowAllOrigins = true
	cfg.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	cfg.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "Token"}
	cfg.ExposeHeaders = []string{"Content-Length"}
	return cors.New(cfg)
}
