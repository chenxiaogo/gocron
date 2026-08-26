package rbac

import "scheduler-platform/internal/model"

// PermItem 权限点定义。
type PermItem struct {
	Code        string
	Name        string
	Type        string
	Description string
}

// AllPermissions 系统全部权限点清单（用于初始化 permission 表与角色授权）。
var AllPermissions = []PermItem{
	{model.PermDashboardView, "查看首页看板", "menu", "查看首页 Dashboard 统计"},
	{model.PermTaskView, "查看任务", "menu", "查看任务列表与详情"},
	{model.PermTaskCreate, "创建任务", "button", "新增任务"},
	{model.PermTaskUpdate, "更新任务", "button", "编辑任务"},
	{model.PermTaskDelete, "删除任务", "button", "删除任务"},
	{model.PermTaskRun, "执行/启停任务", "button", "手动执行、启用、停用任务"},
	{model.PermExecView, "查看执行记录", "menu", "查看执行记录与详情"},
	{model.PermLogView, "查看执行日志", "button", "查看执行日志"},
	{model.PermNodeView, "查看节点", "menu", "查看节点列表与详情"},
	{model.PermNodeManage, "节点管理", "button", "节点增删改"},
	{model.PermUserManage, "用户管理", "menu", "用户增删改查与分配角色"},
	{model.PermRoleManage, "角色管理", "menu", "角色增删改查与授权"},
	{model.PermNotifyView, "查看通知配置", "menu", "查看全局通知配置"},
	{model.PermNotifyManage, "管理通知配置", "button", "保存通知配置与发送测试"},
	{model.PermSystemAdmin, "系统管理员", "role", "系统管理员，拥有全部权限"},
}
