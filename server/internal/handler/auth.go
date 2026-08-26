package handler

import (
	"net/http"

	"scheduler-platform/internal/middleware"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/auth"
	"scheduler-platform/pkg/ilog"
	"scheduler-platform/pkg/tracex"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler 认证接口。
type AuthHandler struct {
	store *store.Store
	jwt   *auth.JWT
}

func NewAuthHandler(s *store.Store, secret string, expireHours int) *AuthHandler {
	return &AuthHandler{store: s, jwt: auth.New(secret, expireHours)}
}

// RegisterRoute 注册认证路由。
// 入参 g 为 /api/auth 分组（公开，无需登录）。仅注册公开接口。
func (h *AuthHandler) RegisterRoute(g *gin.RouterGroup) {
	g.POST("/login", h.Login)
}

// RegisterProtectedRoute 注册需要登录的认证路由（如 /auth/me）。
func (h *AuthHandler) RegisterProtectedRoute(g *gin.RouterGroup) {
	g.GET("/me", h.Me)
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 登录，密码 bcrypt 校验，签发 JWT。
// @Summary 用户登录
// @Tags auth
// @Success 200 {object} map[string]interface{}
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 2, "message": "参数错误"})
		return
	}
	log := ilog.FromGin(c)
	user, ok := h.store.GetUserByUsername(tracex.FromRequest(c), req.Username)
	if !ok {
		log.Warnf("login failed: 用户 %q 不存在", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 4, "message": "用户名或密码错误"})
		return
	}
	if user.Status == 0 {
		log.Warnf("login failed: 用户 %q 已被禁用", req.Username)
		c.JSON(http.StatusForbidden, gin.H{"code": 4, "message": "账号已被禁用"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		log.Warnf("login failed: 用户 %q 密码错误", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 4, "message": "用户名或密码错误"})
		return
	}
	role, perms, _ := h.store.GetUserRole(tracex.FromRequest(c), user.ID)
	var roleID int64
	roleNameStr := ""
	if role != nil {
		roleID = role.ID
		roleNameStr = role.Name
	}
	token, err := h.jwt.Generate(user.ID, user.Username, user.IsAdmin, roleID, roleNameStr, perms)
	if err != nil {
		log.Errorf("签发 token 失败 for user %q: %v", req.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5, "message": "签发 token 失败"})
		return
	}
	log.Infof("用户 %q 登录成功 (uid=%d, isAdmin=%v)", user.Username, user.ID, user.IsAdmin)
	c.JSON(http.StatusOK, gin.H{
		"code": 0, "token": token,
		"user": gin.H{
			"uid": user.ID, "username": user.Username,
			"nickname": user.Nickname, "isAdmin": user.IsAdmin,
			"roleId": roleID, "roleName": roleNameStr, "perms": perms,
		},
	})
}

// Me 当前用户信息（从 JWT 解析，而非硬编码）。
func (h *AuthHandler) Me(c *gin.Context) {
	uid, username, isAdmin := middleware.CurrentUser(c)
	var nickname string
	var roleID int64
	var roleName string
	if user, ok := h.store.GetUserByUsername(tracex.FromRequest(c), username); ok {
		nickname = user.Nickname
		roleID = user.RoleID
		if r, ok2 := h.store.GetRole(tracex.FromRequest(c), roleID); ok2 {
			roleName = r.Name
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"uid": uid, "username": username, "nickname": nickname, "isAdmin": isAdmin,
		"roleId": roleID, "roleName": roleName, "perms": middleware.CurrentPerms(c),
	})
}
