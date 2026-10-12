package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/ilog"
	"scheduler-platform/pkg/metric"
	"scheduler-platform/pkg/proxyx"
	"scheduler-platform/pkg/tracex"

	"github.com/gin-gonic/gin"
)

// NodeHandler 节点管理接口。
type NodeHandler struct {
	store *store.Store
}

func NewNodeHandler(s *store.Store) *NodeHandler {
	return &NodeHandler{store: s}
}

// RegisterRoute 注册节点路由。
func (h *NodeHandler) RegisterRoute(g *gin.RouterGroup) {
	group := g.Group("/nodes")
	{
		group.GET("", middleware.RequirePerm(model.PermNodeView), h.List)
		group.GET("/:id", middleware.RequirePerm(model.PermNodeView), h.Get)
		group.GET("/:id/metrics", middleware.RequirePerm(model.PermNodeView), h.MetricsHistory)
		group.POST("", middleware.RequirePerm(model.PermNodeManage), h.Create)
		group.PUT("/:id", middleware.RequirePerm(model.PermNodeManage), h.Update)
		group.DELETE("/:id", middleware.RequirePerm(model.PermNodeManage), h.Delete)
		group.POST("/:id/scrape", middleware.RequirePerm(model.PermNodeManage), h.ScrapeMetrics)
	}
	// 全局出站代理（机场类 HTTP/SOCKS5），独立于 /nodes/:id 路由前缀避免参数冲突
	proxy := g.Group("/node-proxy")
	{
		proxy.GET("", middleware.RequirePerm(model.PermNodeView), h.GetGlobalProxy)
		proxy.PUT("", middleware.RequirePerm(model.PermNodeManage), h.SaveGlobalProxy)
		proxy.POST("/test", middleware.RequirePerm(model.PermNodeManage), h.TestGlobalProxy)
	}
}

// proxyPasswordMask 密码在返回给前端时的掩码，避免明文回显。
const proxyPasswordMask = "******"

// proxyReq 全局代理配置请求体。
type proxyReq struct {
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// GetGlobalProxy 查询全局代理配置（密码以掩码返回）。
func (h *NodeHandler) GetGlobalProxy(c *gin.Context) {
	ctx := tracex.FromRequest(c)
	p := h.store.GetGlobalProxy(ctx)
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"id":       p.ID,
			"enabled":  p.Enabled,
			"type":     p.Type,
			"host":     p.Host,
			"port":     p.Port,
			"username": p.Username,
			"password": maskProxyPassword(p.Password),
		},
	})
}

// SaveGlobalProxy 保存全局代理配置。password 为空或掩码表示不修改原密码。
func (h *NodeHandler) SaveGlobalProxy(c *gin.Context) {
	var req proxyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "参数错误"})
		return
	}
	ctx := tracex.FromRequest(c)
	cur := h.store.GetGlobalProxy(ctx)
	cur.Enabled = req.Enabled
	cur.Type = req.Type
	cur.Host = req.Host
	cur.Port = req.Port
	cur.Username = req.Username
	if req.Password != "" && req.Password != proxyPasswordMask {
		cur.Password = req.Password
	}
	if !req.Enabled {
		// 关闭代理时不做校验
	} else if cur.Host == "" || cur.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "启用代理需填写代理 IP 与端口"})
		return
	}
	if err := h.store.SaveGlobalProxy(ctx, cur); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "保存失败"})
		return
	}
	_, operator, _ := middleware.CurrentUser(c)
	ilog.FromGin(c).Infof("用户 %s 更新全局代理: enabled=%v type=%s host=%s:%d user=%s",
		operator, cur.Enabled, cur.Type, cur.Host, cur.Port, cur.Username)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "保存成功"})
}

// TestGlobalProxy 连通性测试：保存后经代理请求指定地址，返回是否可达。
func (h *NodeHandler) TestGlobalProxy(c *gin.Context) {
	var req struct {
		Target string `json:"target"`
	}
	_ = c.ShouldBindJSON(&req)
	ctx := tracex.FromRequest(c)
	cfg := h.store.ProxyConfig(ctx)
	target := req.Target
	if target == "" {
		target = "https://www.google.com"
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}
	start := time.Now()
	status := "未走代理"
	var code int
	var errMsg string
	client := cfg.Client(15 * time.Second)
	resp, err := client.Get(target)
	cost := time.Since(start).Milliseconds()
	if err != nil {
		errMsg = err.Error()
	} else {
		code = resp.StatusCode
		resp.Body.Close()
	}
	if errMsg != "" {
		ilog.FromGin(c).Warnf("全局代理连通性测试失败 target=%s: %s", target, errMsg)
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": "连接失败: " + errMsg, "data": gin.H{
			"proxy": cfg.Addr(), "target": target, "costMs": cost, "error": errMsg,
		}})
		return
	}
	if cfg.Valid() {
		status = "已走代理 " + cfg.Type + "://" + cfg.Addr()
	}
	ilog.FromGin(c).Infof("全局代理连通性测试成功 target=%s status=%d cost=%dms", target, code, cost)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "连接成功", "data": gin.H{
		"proxy": cfg.Addr(), "target": target, "statusCode": code, "costMs": cost, "via": status,
	}})
}

// maskProxyPassword 密码掩码。
func maskProxyPassword(p string) string {
	if p == "" {
		return ""
	}
	return proxyPasswordMask
}

// RegisterPublicRoute 注册公开路由（无需 JWT 鉴权，供节点 agent 上报指标）。
func (h *NodeHandler) RegisterPublicRoute(g *gin.RouterGroup) {
	g.POST("/nodes/report", h.ReportMetrics)
}

type nodeReq struct {
	Name           string  `json:"name" binding:"required"`
	Address        string  `json:"address"`
	MetricsURL     string  `json:"metricsUrl"`
	Proxy          string  `json:"proxy"`
	OS             string  `json:"os"`
	MaxConcurrency int     `json:"maxConcurrency"`
	CPUUsage       float64 `json:"cpuUsage"`
	MemoryUsage    float64 `json:"memoryUsage"`
	DiskUsage      float64 `json:"diskUsage"`
}

// Create 新增节点。
func (h *NodeHandler) Create(c *gin.Context) {
	var req nodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "参数错误"})
		return
	}
	node := &model.Node{
		NodeID: "node-" + time.Now().Format("20060102150405"),
		Name: req.Name, Address: req.Address, MetricsURL: req.MetricsURL, Proxy: req.Proxy, OS: req.OS,
		Status: "online", MaxConcurrency: req.MaxConcurrency,
		CPUUsage: req.CPUUsage, MemoryUsage: req.MemoryUsage, DiskUsage: req.DiskUsage,
		LastHeartbeat: time.Now(), CreatedAt: time.Now(),
	}
	if err := h.store.DB().Create(node).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "创建节点失败"})
		return
	}
	_, operator, _ := middleware.CurrentUser(c)
	ilog.FromGin(c).Infof("用户 %s 创建节点 #%d (%s, address=%s)", operator, node.ID, node.Name, node.Address)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "创建成功", "data": node.ID})
}

// Update 更新节点。
func (h *NodeHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, ok := h.store.GetNode(tracex.FromRequest(c), id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "节点不存在"})
		return
	}
	var req nodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "参数错误"})
		return
	}
	node.Name = req.Name
	node.Address = req.Address
	node.MetricsURL = req.MetricsURL
	node.Proxy = req.Proxy
	node.OS = req.OS
	node.MaxConcurrency = req.MaxConcurrency
	node.CPUUsage = req.CPUUsage
	node.MemoryUsage = req.MemoryUsage
	node.DiskUsage = req.DiskUsage
	node.UpdatedAt = time.Now()
	if err := h.store.DB().Save(node).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "更新失败"})
		return
	}
	_, operator, _ := middleware.CurrentUser(c)
	ilog.FromGin(c).Infof("用户 %s 更新节点 #%d (%s)", operator, id, node.Name)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "更新成功"})
}

// Delete 删除节点。
func (h *NodeHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.store.DB().Delete(&model.Node{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "删除失败"})
		return
	}
	_, operator, _ := middleware.CurrentUser(c)
	ilog.FromGin(c).Infof("用户 %s 删除节点 #%d", operator, id)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "删除成功"})
}

// List 节点列表。
// @Summary 节点列表
// @Tags node
// @Success 200 {array} model.Node
// @Router /api/nodes [get]
func (h *NodeHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.ListNodes(tracex.FromRequest(c)))
}

// Get 节点详情。
func (h *NodeHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	for _, n := range h.store.ListNodes(tracex.FromRequest(c)) {
		if n.ID == id {
			c.JSON(http.StatusOK, n)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "节点不存在"})
}

// metricsReq 节点指标上报请求体。
type metricsReq struct {
	NodeID       string  `json:"nodeId" binding:"required"`
	CPUUsage     float64 `json:"cpuUsage"`
	MemoryUsage  float64 `json:"memoryUsage"`
	DiskUsage    float64 `json:"diskUsage"`
	LoadAverage  float64 `json:"loadAverage"`
	RunningTasks int     `json:"runningTasks"`
}

// ReportMetrics 节点指标上报（公开接口，无需登录）。
// 节点 agent 定时 POST 上报 CPU/内存/磁盘/负载/运行任务数，同时更新心跳时间。
// @Summary 节点指标上报
// @Tags node
// @Param body body metricsReq true "指标数据"
// @Success 200 {object} gin.H
// @Router /api/nodes/report [post]
func (h *NodeHandler) ReportMetrics(c *gin.Context) {
	var req metricsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "参数错误: " + err.Error()})
		return
	}
	ok := h.store.UpdateNodeMetrics(tracex.FromRequest(c), req.NodeID,
		req.CPUUsage, req.MemoryUsage, req.DiskUsage, req.LoadAverage, req.RunningTasks)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "节点不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

// ScrapeMetrics 主动抓取节点 Prometheus 指标（需登录）。
// 根据节点配置的 metricsUrl 请求 Prometheus 文本格式指标，解析后更新节点状态。
// @Summary 抓取节点指标
// @Tags node
// @Param id path int true "节点ID"
// @Success 200 {object} gin.H
// @Router /api/nodes/:id/scrape [post]
func (h *NodeHandler) ScrapeMetrics(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, ok := h.store.GetNode(tracex.FromRequest(c), id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": 3, "message": "节点不存在"})
		return
	}
	if node.MetricsURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "节点未配置 metricsUrl"})
		return
	}

	// 节点级代理优先（翻墙用），未配置则回落全局代理
	proxyCfg := h.store.ProxyConfig(tracex.FromRequest(c))
	if pc := proxyx.FromURL(node.Proxy); pc.Valid() {
		proxyCfg = pc
	}
	proxyClient := proxyCfg.Client(10 * time.Second)
	result := metric.ScrapePrometheusWith(node.MetricsURL, 10*time.Second, proxyClient)
	if result.Error != "" {
		ilog.FromGin(c).Warnf("scrape node %s (%s) failed: %s", node.Name, node.MetricsURL, result.Error)
		c.JSON(http.StatusOK, gin.H{
			"code":    1,
			"message": result.Error,
			"data":    result,
		})
		return
	}

	// 更新节点指标
	h.store.UpdateNodeMetrics(tracex.FromRequest(c), node.NodeID,
		result.CPUUsage, result.MemoryUsage, result.DiskUsage, result.LoadAverage, node.RunningTasks)

	// 同时更新状态为 online（抓取成功说明节点可达）
	h.store.DB().Model(&model.Node{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": "online",
	})

	ilog.FromGin(c).Infof("scrape node %s (%s) ok: cpu=%.1f%% mem=%.1f%% disk=%.1f%% load=%.2f metrics=%d",
		node.Name, node.MetricsURL, result.CPUUsage, result.MemoryUsage, result.DiskUsage, result.LoadAverage, result.RawMetrics)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    result,
	})
}

// MetricsHistory 查询节点历史指标采样（供趋势图展示）。
// range 参数：1h / 6h / 24h / 7d / 30d，默认 24h。
func (h *NodeHandler) MetricsHistory(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "无效的节点ID"})
		return
	}
	rng := c.DefaultQuery("range", "24h")
	samples := h.store.ListNodeMetricsSamples(tracex.FromRequest(c), id, rng)
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": samples,
	})
}
