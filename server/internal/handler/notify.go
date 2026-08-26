package handler

import (
	"net/http"

	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/notify"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/ilog"
	"scheduler-platform/pkg/tracex"

	"github.com/gin-gonic/gin"
)

// NotifyHandler 通知配置接口。
type NotifyHandler struct {
	store *store.Store
}

func NewNotifyHandler(s *store.Store) *NotifyHandler {
	return &NotifyHandler{store: s}
}

// RegisterRoute 注册通知配置路由。
func (h *NotifyHandler) RegisterRoute(g *gin.RouterGroup) {
	group := g.Group("/notify")
	{
		group.GET("/settings", middleware.RequirePerm(model.PermNotifyView), h.GetSettings)
		group.PUT("/settings", middleware.RequirePerm(model.PermNotifyManage), h.SaveSettings)
		group.POST("/test", middleware.RequirePerm(model.PermNotifyManage), h.Test)
	}
}

// GetSettings 获取全局通知配置。
// @Summary 获取通知配置
// @Tags notify
// @Success 200 {object} model.NotifySetting
// @Router /api/notify/settings [get]
func (h *NotifyHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GetNotifySetting(tracex.FromRequest(c)))
}

// SaveSettings 保存全局通知配置。
// @Summary 保存通知配置
// @Tags notify
// @Accept json
// @Param body body model.NotifySetting true "通知配置"
// @Success 200 {object} map[string]interface{}
// @Router /api/notify/settings [put]
func (h *NotifyHandler) SaveSettings(c *gin.Context) {
	var st model.NotifySetting
	if err := c.ShouldBindJSON(&st); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": err.Error()})
		return
	}
	_, username, _ := middleware.CurrentUser(c)
	st.UpdatedBy = username
	h.store.SaveNotifySetting(tracex.FromRequest(c), &st)
	ilog.FromGin(c).Infof("用户 %s 保存全局通知配置 (enabled=%v)", username, st.Enabled)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "保存成功"})
}

// Test 发送测试通知（用当前保存的全局配置发一条测试消息）。
// @Summary 发送测试通知
// @Tags notify
// @Success 200 {object} map[string]interface{}
// @Router /api/notify/test [post]
func (h *NotifyHandler) Test(c *gin.Context) {
	st := h.store.GetNotifySetting(tracex.FromRequest(c))
	if !st.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "通知未启用，请先保存并启用配置"})
		return
	}
	if st.DingtalkWebhook == "" && st.WebhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "请先配置钉钉或 Webhook 地址"})
		return
	}
	// 发送测试（钉钉 + webhook 都试），携带链路 ID
	ctx := tracex.FromRequest(c)
	title := "[调度平台] 测试通知"
	content := "#### 测试通知\n这是一条来自调度平台的测试消息。"
	_, username, _ := middleware.CurrentUser(c)
	log := ilog.Trace(ctx)
	if st.DingtalkWebhook != "" {
		if err := notify.SendDingtalk(ctx, st.DingtalkWebhook, st.DingtalkSecret, title, content); err != nil {
			log.Warnf("用户 %s 测试钉钉通知失败: %v", username, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "钉钉发送失败: " + err.Error()})
			return
		}
	}
	if st.WebhookURL != "" {
		if err := notify.SendWebhook(ctx, st.WebhookURL, st.WebhookSecret, title, content, "test"); err != nil {
			log.Warnf("用户 %s 测试 Webhook 通知失败: %v", username, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "Webhook 发送失败: " + err.Error()})
			return
		}
	}
	log.Infof("用户 %s 发送测试通知成功", username)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "测试通知已发送"})
}
