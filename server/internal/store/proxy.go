package store

import (
	"context"
	"strings"
	"time"

	"scheduler-platform/internal/model"
	"scheduler-platform/pkg/proxyx"
)

// globalProxyID 全局代理配置固定主键，系统只保留一条记录。
const globalProxyID = 1

// GetGlobalProxy 读取全局代理配置，不存在时返回禁用态空配置。
func (s *Store) GetGlobalProxy(ctx context.Context) *model.GlobalProxy {
	var p model.GlobalProxy
	if err := s.db.WithContext(ctx).First(&p, globalProxyID).Error; err != nil {
		return &model.GlobalProxy{ID: globalProxyID, Type: proxyx.TypeHTTP}
	}
	if p.Type == "" {
		p.Type = proxyx.TypeHTTP
	}
	return &p
}

// SaveGlobalProxy 保存（upsert）全局代理配置，ID 固定为 1。
func (s *Store) SaveGlobalProxy(ctx context.Context, p *model.GlobalProxy) error {
	p.ID = globalProxyID
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	if p.Type == "" {
		p.Type = proxyx.TypeHTTP
	}
	now := time.Now()
	p.UpdatedAt = now
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	return s.db.WithContext(ctx).Save(p).Error
}

// ProxyConfig 返回可直接用于构造 HTTP 客户端的代理配置。
func (s *Store) ProxyConfig(ctx context.Context) proxyx.Config {
	p := s.GetGlobalProxy(ctx)
	return proxyx.Config{
		Enabled:  p.Enabled,
		Type:     p.Type,
		Host:     p.Host,
		Port:     p.Port,
		Username: p.Username,
		Password: p.Password,
	}
}
