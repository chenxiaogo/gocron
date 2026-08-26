package middleware

import (
	"net/http"
	"strings"

	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/auth"
	"scheduler-platform/pkg/casbin"

	"github.com/gin-gonic/gin"
)

const (
	CtxUserID   = "auth_user_id"
	CtxUsername = "auth_username"
	CtxIsAdmin  = "auth_is_admin"
	CtxRoleID   = "auth_role_id"
	CtxRoleName = "auth_role_name"
	CtxPerms    = "auth_perms"
)

// Auth 鉴权中间件：校验 Authorization: Bearer <token>。
func Auth(jwt *auth.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			// 兼容旧版前端传 Token 头
			header = c.GetHeader("Token")
		}
		token := strings.TrimPrefix(header, "Bearer ")
		token = strings.TrimSpace(token)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录或 token 缺失"})
			return
		}
		claims, err := jwt.Parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token 无效或已过期"})
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxIsAdmin, claims.IsAdmin)
		c.Set(CtxRoleID, claims.RoleID)
		c.Set(CtxRoleName, claims.RoleName)
		c.Set(CtxPerms, claims.Perms)
		c.Next()
	}
}

// RequirePerm 权限校验中间件：admin 恒放行，普通用户校验权限点。
// 用法：group.POST("", middleware.RequirePerm(model.PermTaskCreate), h.Create)
func RequirePerm(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin := CurrentIsAdmin(c)
		if isAdmin {
			c.Next()
			return
		}
		perms := CurrentPerms(c)
		for _, p := range perms {
			if p == perm {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "无权限执行此操作"})
	}
}

// CasbinAuth Casbin API 权限校验中间件。
// 用法：protected.Use(middleware.CasbinAuth(enforcer, store))。admin 恒放行。
// 仅校验注册到 sys_apis 的受管接口，未纳入管理的接口不做拦截（避免误伤）。
func CasbinAuth(enforcer *casbin.Enforcer, store *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin := CurrentIsAdmin(c)
		roleID := CurrentRoleID(c)
		path := c.Request.URL.Path
		method := c.Request.Method
		// 仅当该接口被声明为受管 API 时才做 casbin 校验
		if !store.IsManagedApi(c.Request.Context(), path, method) {
			c.Next()
			return
		}
		if enforcer.Enforce(roleID, isAdmin, path, method) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "无接口访问权限"})
	}
}

// CurrentRoleID 从上下文取当前用户角色ID。
func CurrentRoleID(c *gin.Context) int64 {
	v, _ := c.Get(CtxRoleID)
	roleID, _ := v.(int64)
	return roleID
}

// CurrentUser 从上下文取当前用户信息。
func CurrentUser(c *gin.Context) (userID int64, username string, isAdmin bool) {
	if v, ok := c.Get(CtxUserID); ok {
		userID, _ = v.(int64)
	}
	if v, ok := c.Get(CtxUsername); ok {
		username, _ = v.(string)
	}
	if v, ok := c.Get(CtxIsAdmin); ok {
		isAdmin, _ = v.(bool)
	}
	return
}

// CurrentIsAdmin 当前用户是否管理员。
func CurrentIsAdmin(c *gin.Context) bool {
	v, _ := c.Get(CtxIsAdmin)
	isAdmin, _ := v.(bool)
	return isAdmin
}

// CurrentPerms 当前用户权限点列表。
func CurrentPerms(c *gin.Context) []string {
	v, _ := c.Get(CtxPerms)
	perms, _ := v.([]string)
	return perms
}
