package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 自定义 JWT 声明。
type Claims struct {
	UserID   int64    `json:"uid"`
	Username string   `json:"username"`
	IsAdmin  bool     `json:"isAdmin"`
	RoleID   int64    `json:"roleId"`
	RoleName string   `json:"roleName"`
	Perms    []string `json:"perms"`
	jwt.RegisteredClaims
}

// HasPerm 判断是否拥有某权限点（admin 恒有全部权限）。
func (c *Claims) HasPerm(perm string) bool {
	if c.IsAdmin {
		return true
	}
	for _, p := range c.Perms {
		if p == perm {
			return true
		}
	}
	return false
}

// JWT 签发与解析封装。
type JWT struct {
	secret []byte
	expire time.Duration
}

// New 创建 JWT 工具。
func New(secret string, expireHours int) *JWT {
	if expireHours <= 0 {
		expireHours = 24
	}
	return &JWT{secret: []byte(secret), expire: time.Duration(expireHours) * time.Hour}
}

// Generate 为用户签发 token。
func (j *JWT) Generate(userID int64, username string, isAdmin bool, roleID int64, roleName string, perms []string) (string, error) {
	claims := Claims{
		UserID: userID, Username: username, IsAdmin: isAdmin,
		RoleID: roleID, RoleName: roleName, Perms: perms,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "scheduler-platform",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

// GenerateWithClaims 按给定 Claims 签发（供内部复用）。
func (j *JWT) GenerateWithClaims(c Claims) (string, error) {
	c.Issuer = "scheduler-platform"
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(j.expire))
	c.IssuedAt = jwt.NewNumericDate(time.Now())
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(j.secret)
}

// Parse 解析并校验 token，返回声明。
func (j *JWT) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
