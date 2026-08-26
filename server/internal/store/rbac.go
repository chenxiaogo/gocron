package store

import (
	"context"
	"time"

	"gorm.io/gorm"

	"scheduler-platform/internal/model"
	"scheduler-platform/pkg/rbac"
)

// ---------- 角色初始化（RBAC） ----------

// InitRBAC 初始化权限点表与内置角色（admin / viewer / operator）。
// 仅在对应表为空时写入，保证幂等。admin 角色拥有全部权限。
func (s *Store) InitRBAC(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 权限点（幂等：缺失的权限点自动补齐，兼容既有库升级）
		if err := s.ensurePermissions(tx); err != nil {
			return err
		}

		// 2. 内置角色
		var roleCount int64
		if err := tx.Model(&model.Role{}).Count(&roleCount).Error; err != nil {
			return err
		}
		if roleCount > 0 {
			return nil
		}
		return s.seedRoles(tx)
	})
}

// ensurePermissions 确保权限点表中包含 rbac.AllPermissions 定义的全部权限点。
// 对已存在数据库也做补齐（幂等 upsert），保证新增权限点能生效。
func (s *Store) ensurePermissions(tx *gorm.DB) error {
	var existing []model.Permission
	if err := tx.Find(&existing).Error; err != nil {
		return err
	}
	have := make(map[string]bool, len(existing))
	for _, p := range existing {
		have[p.Code] = true
	}
	now := time.Now()
	for _, p := range rbac.AllPermissions {
		if have[p.Code] {
			continue
		}
		if err := tx.Create(&model.Permission{
			Code: p.Code, Name: p.Name, Type: p.Type, Description: p.Description, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// seedRoles 写入内置角色并绑定权限点。
func (s *Store) seedRoles(tx *gorm.DB) error {
	now := time.Now()

	// admin：全部权限
	admin := &model.Role{Code: "admin", Name: "管理员", Description: "系统管理员，拥有全部权限", BuiltIn: true, CreatedAt: now}
	if err := tx.Create(admin).Error; err != nil {
		return err
	}
	if err := s.bindAllPerms(tx, admin.ID); err != nil {
		return err
	}

	// operator：任务/执行/节点查看与操作
	operator := &model.Role{Code: "operator", Name: "运维人员", Description: "可管理任务、查看执行与节点", BuiltIn: true, CreatedAt: now}
	if err := tx.Create(operator).Error; err != nil {
		return err
	}
	opPerms := []string{
		model.PermTaskView, model.PermTaskCreate, model.PermTaskUpdate,
		model.PermTaskRun, model.PermExecView, model.PermLogView,
		model.PermNodeView,
	}
	if err := s.bindPermsByCodes(tx, operator.ID, opPerms); err != nil {
		return err
	}

	// viewer：只读
	viewer := &model.Role{Code: "viewer", Name: "访客", Description: "只读权限", BuiltIn: true, CreatedAt: now}
	if err := tx.Create(viewer).Error; err != nil {
		return err
	}
	viewerPerms := []string{model.PermTaskView, model.PermExecView, model.PermLogView, model.PermNodeView}
	return s.bindPermsByCodes(tx, viewer.ID, viewerPerms)
}

func (s *Store) bindAllPerms(tx *gorm.DB, roleID int64) error {
	var perms []model.Permission
	if err := tx.Find(&perms).Error; err != nil {
		return err
	}
	now := time.Now()
	for _, p := range perms {
		if err := tx.Create(&model.RolePermission{RoleID: roleID, PermissionID: p.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
	}
	return nil
}

// bindPermsByCodes 按权限码绑定角色权限。
func (s *Store) bindPermsByCodes(tx *gorm.DB, roleID int64, codes []string) error {
	var perms []model.Permission
	if err := tx.Where("code IN ?", codes).Find(&perms).Error; err != nil {
		return err
	}
	now := time.Now()
	for _, p := range perms {
		if err := tx.Create(&model.RolePermission{RoleID: roleID, PermissionID: p.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ---------- 角色查询 ----------

// ListRoles 角色列表（含权限点）。
func (s *Store) ListRoles(ctx context.Context) []*model.Role {
	var roles []*model.Role
	s.db.WithContext(ctx).Order("id ASC").Preload("Permissions").Find(&roles)
	for _, r := range roles {
		for _, p := range r.Permissions {
			r.PermIDs = append(r.PermIDs, p.ID)
		}
	}
	return roles
}

// GetRole 角色详情。
func (s *Store) GetRole(ctx context.Context, id int64) (*model.Role, bool) {
	var r model.Role
	if err := s.db.WithContext(ctx).First(&r, id).Error; err != nil {
		return nil, false
	}
	var perms []model.Permission
	s.db.WithContext(ctx).Model(&r).Association("Permissions").Find(&perms)
	r.Permissions = perms
	for _, p := range perms {
		r.PermIDs = append(r.PermIDs, p.ID)
	}
	return &r, true
}

// GetRoleByCode 按编码查角色。
func (s *Store) GetRoleByCode(ctx context.Context, code string) (*model.Role, bool) {
	var r model.Role
	if err := s.db.WithContext(ctx).Where("code = ?", code).First(&r).Error; err != nil {
		return nil, false
	}
	return &r, true
}

// ListPermissions 权限点列表。
func (s *Store) ListPermissions(ctx context.Context) []*model.Permission {
	var perms []*model.Permission
	s.db.WithContext(ctx).Order("id ASC").Find(&perms)
	return perms
}

// ---------- 角色权限管理 ----------

// SaveRolePerms 保存角色权限点（先删后插）。
func (s *Store) SaveRolePerms(ctx context.Context, roleID int64, permIDs []int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, pid := range permIDs {
			if err := tx.Create(&model.RolePermission{RoleID: roleID, PermissionID: pid, CreatedAt: now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetRolePermCodes 获取角色权限码列表。
func (s *Store) GetRolePermCodes(ctx context.Context, roleID int64) []string {
	var perms []model.Permission
	s.db.WithContext(ctx).Model(&model.Role{ID: roleID}).Association("Permissions").Find(&perms)
	codes := make([]string, 0, len(perms))
	for _, p := range perms {
		codes = append(codes, p.Code)
	}
	return codes
}

// ---------- 用户角色 ----------

// GetUserRole 获取用户角色（admin 特殊处理）。
func (s *Store) GetUserRole(ctx context.Context, userID int64) (*model.Role, []string, error) {
	var u model.User
	if err := s.db.WithContext(ctx).First(&u, userID).Error; err != nil {
		return nil, nil, err
	}
	if u.IsAdmin {
		// admin 拥有全部权限
		all := make([]string, 0, len(rbac.AllPermissions))
		for _, p := range rbac.AllPermissions {
			all = append(all, p.Code)
		}
		return &model.Role{Code: "admin", Name: "管理员", ID: u.RoleID}, all, nil
	}
	if u.RoleID == 0 {
		return nil, nil, nil
	}
	role, ok := s.GetRole(ctx, u.RoleID)
	if !ok {
		return nil, nil, nil
	}
	return role, s.GetRolePermCodes(ctx, role.ID), nil
}

// ---------- 任务-节点绑定 ----------

// SaveTaskNodes 保存任务绑定的节点（先删后插）。
func (s *Store) SaveTaskNodes(ctx context.Context, taskID int64, nodeIDs []int64) {
	if err := s.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&model.TaskNode{}).Error; err != nil {
		return
	}
	for _, nid := range nodeIDs {
		if nid <= 0 {
			continue
		}
		s.db.WithContext(ctx).Create(&model.TaskNode{TaskID: taskID, NodeID: nid})
	}
}

// GetTaskNodeIDs 获取任务绑定的节点ID列表。
func (s *Store) GetTaskNodeIDs(ctx context.Context, taskID int64) []int64 {
	var tns []model.TaskNode
	s.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&tns)
	ids := make([]int64, 0, len(tns))
	for _, tn := range tns {
		ids = append(ids, tn.NodeID)
	}
	return ids
}

// GetTaskNodes 获取任务绑定的节点列表。
func (s *Store) GetTaskNodes(ctx context.Context, taskID int64) []*model.Node {
	ids := s.GetTaskNodeIDs(ctx, taskID)
	if len(ids) == 0 {
		return nil
	}
	var nodes []*model.Node
	s.db.WithContext(ctx).Where("id IN ?", ids).Find(&nodes)
	return nodes
}

// DeleteTaskNodes 删除任务时清理绑定。
func (s *Store) DeleteTaskNodes(ctx context.Context, taskID int64) {
	s.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&model.TaskNode{})
}
