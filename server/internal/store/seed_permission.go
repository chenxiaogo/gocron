package store

import (
	"context"
	"strconv"
	"strings"
	"time"

	"scheduler-platform/internal/model"
	"scheduler-platform/pkg/casbin"

	"gorm.io/gorm"
)

// InitPermissionData 初始化菜单树、菜单按钮、受管 API。
// 幂等：既有数据仅做补齐（菜单 component 迁移、缺失的 API 补录），保证升级安全。
// 需要在 InitRBAC（角色创建）之后调用。
func (s *Store) InitPermissionData(ctx context.Context, enforcer *casbin.Enforcer) error {
	added := false
	menuMigrated := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 0. 修正既有菜单的 component 路径（幂等迁移，兼容早期 `view/task/index` 等旧写法）
		s.fixMenuComponents(tx)
		// 1. 菜单结构迁移：将多级结构（任务管理/设置目录）或历史结构统一重排为目标扁平结构
		//    （概览/节点/任务/执行记录/通知配置 顶级 + 系统管理目录）。
		menuMigrated = s.migrateMenuStructure(tx)

		// 2. 菜单 + 按钮（仅新库写入，旧库由迁移对齐）
		var menuCount int64
		if err := tx.Model(&model.Menu{}).Count(&menuCount).Error; err != nil {
			return err
		}
		if menuCount == 0 {
			if err := s.seedMenus(tx); err != nil {
				return err
			}
		}

		// 3. 受管 API（幂等：缺失的 API 自动补齐，兼容既有库升级）
		var err error
		added, err = s.ensureApis(tx)
		return err
	})
	if err != nil {
		return err
	}
	// 菜单结构迁移后，重建内置角色的菜单/按钮绑定（需在事务提交后基于最新数据执行）
	if menuMigrated {
		s.RebuildBuiltinRoleMenus(ctx)
	}
	// 若补齐了新的受管 API，则为内置角色重新同步 casbin API 策略（含新增接口）
	if added {
		s.SyncBuiltinRolePermissions(ctx, enforcer)
	}
	return nil
}

// fixMenuComponents 将旧版 component 路径（view/task/index 等）迁移到当前前端实际文件名，保证动态路由正确加载视图。
// 幂等：仅更新匹配到旧路径的菜单。
func (s *Store) fixMenuComponents(tx *gorm.DB) {
	// 旧 component -> 当前前端真实文件路径
	legacy := map[string]string{
		"view/dashboard/index": "view/dashboard/Dashboard",
		"view/task/index":      "view/task/TaskList",
		"view/execution/index": "view/execution/ExecutionList",
		"view/node/index":      "view/node/NodeList",
		"view/user/index":      "view/user/UserList",
		"view/role/index":      "view/role/RoleList",
		"view/system/menu":     "view/system/MenuManage",
		"view/system/api":      "view/system/ApiManage",
		"view/task/list":       "view/task/TaskList",
		"view/execution/list":  "view/execution/ExecutionList",
		"view/node/list":       "view/node/NodeList",
		"view/user/list":       "view/user/UserList",
		"view/role/list":       "view/role/RoleList",
		"view/system/menus":    "view/system/MenuManage",
		"view/system/apis":     "view/system/ApiManage",
	}
	for old, current := range legacy {
		tx.Model(&model.Menu{}).Where("component = ?", old).Update("component", current)
	}
}

// migrateMenuStructure 将既有菜单结构（多级「任务管理/设置」目录，或历史扁平结构）统一重排为
// 目标扁平结构：概览 / 节点 / 任务 / 执行记录 / 通知配置（顶级叶子）+ 系统管理（目录）。
// 兼容当前多级库：Tasks 为目录、含 TaskList/ExecutionList/NodeList 子菜单；Settings 为目录、含 Users/Roles/Menus/Apis/Notify。
// 兼容历史库：任务/执行/节点为独立顶级菜单、用户/角色/菜单/接口为独立顶级菜单（归并到系统管理目录）。
// 返回是否发生了迁移（调用方据此在事务提交后重建内置角色菜单绑定）。
// 幂等：检测到已是目标扁平结构（存在顶级 Nodes 菜单与系统管理目录）则直接返回 false。
func (s *Store) migrateMenuStructure(tx *gorm.DB) bool {
	getByName := func(name string) *model.Menu {
		var m model.Menu
		if err := tx.Where("name = ?", name).First(&m).Error; err != nil {
			return nil
		}
		return &m
	}

	// 幂等：已是目标扁平结构则跳过
	if nodes := getByName("Nodes"); nodes != nil && nodes.Type == "menu" && nodes.ParentID == 0 {
		if sys := getByName("System"); sys != nil && sys.Type == "dir" {
			return false
		}
	}

	now := time.Now()

	// 1. 系统管理目录：优先复用旧 Settings 目录，否则新建
	system := getByName("System")
	if system == nil {
		system = getByName("Settings")
	}
	if system == nil {
		system = &model.Menu{Name: "System", Path: "/system", Title: "系统管理", Icon: "Setting",
			Type: "dir", PermCode: model.PermRoleManage, ParentID: 0, Sort: 6, CreatedAt: now}
		tx.Create(system)
	} else {
		tx.Model(&model.Menu{}).Where("id = ?", system.ID).Updates(map[string]interface{}{
			"name": "System", "title": "系统管理", "icon": "Setting", "path": "/system",
			"type": "dir", "component": "", "perm_code": model.PermRoleManage, "parent_id": 0,
		})
	}

	// 2. 顶级叶子菜单：存在则提升/改名，不存在则新建（兼容历史命名 ExecutionList/NodeList）
	ensureTop := func(name, title, path, component, icon, perm string, sort int) *model.Menu {
		m := getByName(name)
		if m == nil {
			switch name {
			case "Executions":
				m = getByName("ExecutionList")
			case "Nodes":
				m = getByName("NodeList")
			}
		}
		if m == nil {
			m = &model.Menu{Name: name, Path: path, Component: component, Title: title, Icon: icon,
				Type: "menu", PermCode: perm, ParentID: 0, Sort: sort, CreatedAt: now}
			tx.Create(m)
			return m
		}
		tx.Model(&model.Menu{}).Where("id = ?", m.ID).Updates(map[string]interface{}{
			"name": name, "parent_id": 0, "path": path, "component": component, "title": title,
			"icon": icon, "type": "menu", "perm_code": perm, "hidden": false, "sort": sort,
		})
		return m
	}

	ensureTop("Dashboard", "首页", "/dashboard", "view/dashboard/Dashboard", "Odometer", "", 1)
	tasks := ensureTop("Tasks", "任务", "/tasks", "view/task/TaskList", "List", model.PermTaskView, 3)
	ensureTop("Nodes", "节点管理", "/nodes", "view/node/NodeList", "Monitor", model.PermNodeView, 2)
	ensureTop("Executions", "执行记录", "/executions", "view/execution/ExecutionList", "Timer", model.PermExecView, 4)
	ensureTop("Notify", "通知配置", "/notify", "view/settings/NotifyManage", "Bell", model.PermNotifyManage, 5)

	// 3. Tasks 原按钮（挂在历史 TaskList 上）迁移到新的 Tasks 菜单，并删除冗余 TaskList
	if oldTask := getByName("TaskList"); oldTask != nil {
		tx.Model(&model.MenuBtn{}).Where("menu_id = ?", oldTask.ID).Update("menu_id", tasks.ID)
		tx.Where("menu_id = ?", oldTask.ID).Delete(&model.RoleMenu{})
		tx.Where("menu_id = ?", oldTask.ID).Delete(&model.RoleMenuBtn{})
		tx.Where("menu_id = ?", oldTask.ID).Delete(&model.MenuBtn{})
		tx.Delete(&model.Menu{ID: oldTask.ID})
	}

	// 4. 系统管理目录下的子菜单：用户 / 角色 / 菜单 / 接口
	children := []struct {
		name, path, title, component, icon, perm string
		sort                                     int
	}{
		{"Users", "/users", "用户管理", "view/user/UserList", "User", model.PermUserManage, 1},
		{"Roles", "/roles", "角色管理", "view/role/RoleList", "Key", model.PermRoleManage, 2},
		{"Menus", "/menus", "菜单管理", "view/system/MenuManage", "Grid", model.PermRoleManage, 3},
		{"Apis", "/apis", "接口管理", "view/system/ApiManage", "Link", model.PermRoleManage, 4},
	}
	for _, c := range children {
		m := getByName(c.name)
		if m == nil {
			m = &model.Menu{Name: c.name, Path: c.path, Component: c.component, Title: c.title, Icon: c.icon,
				Type: "menu", PermCode: c.perm, ParentID: system.ID, Sort: c.sort, CreatedAt: now}
			tx.Create(m)
		} else {
			tx.Model(&model.Menu{}).Where("id = ?", m.ID).Updates(map[string]interface{}{
				"parent_id": system.ID, "path": c.path, "component": c.component, "title": c.title,
				"icon": c.icon, "type": "menu", "perm_code": c.perm, "hidden": false, "sort": c.sort,
			})
		}
	}

	return true
}

// RebuildBuiltinRoleMenus 清空并重建内置角色（admin/operator/viewer）的菜单与按钮绑定。
// 与 SyncBuiltinRolePermissions 的授权口径一致，基于提交后的最新菜单/按钮数据，先清后插保证幂等无重复。
// 需在菜单结构迁移的事务提交后调用。
func (s *Store) RebuildBuiltinRoleMenus(ctx context.Context) {
	db := s.db.WithContext(ctx)
	var roles []model.Role
	if err := db.Where("built_in = ?", true).Find(&roles).Error; err != nil || len(roles) == 0 {
		return
	}
	roleIDs := make([]int64, 0, len(roles))
	for _, r := range roles {
		roleIDs = append(roleIDs, r.ID)
	}
	db.Where("role_id IN ?", roleIDs).Delete(&model.RoleMenu{})
	db.Where("role_id IN ?", roleIDs).Delete(&model.RoleMenuBtn{})

	var menus []model.Menu
	db.Find(&menus)
	now := time.Now()

	bindRole := func(roleID int64, permSet map[string]bool) {
		for _, m := range menus {
			// 无 permCode 的菜单（如首页）所有人可见；否则需在角色授权集合内
			if m.PermCode != "" && !permSet[m.PermCode] {
				continue
			}
			db.Create(&model.RoleMenu{RoleID: roleID, MenuID: m.ID})
			for _, b := range s.GetMenuBtns(ctx, m.ID) {
				if b.PermCode == "" || permSet[b.PermCode] {
					db.Create(&model.RoleMenuBtn{RoleID: roleID, MenuID: m.ID, BtnID: b.ID, CreatedAt: now})
				}
			}
		}
	}

	for _, role := range roles {
		switch role.Code {
		case "admin":
			for _, m := range menus {
				db.Create(&model.RoleMenu{RoleID: role.ID, MenuID: m.ID})
				for _, b := range s.GetMenuBtns(ctx, m.ID) {
					db.Create(&model.RoleMenuBtn{RoleID: role.ID, MenuID: m.ID, BtnID: b.ID, CreatedAt: now})
				}
			}
		case "operator":
			bindRole(role.ID, map[string]bool{
				model.PermTaskView: true, model.PermTaskCreate: true, model.PermTaskUpdate: true,
				model.PermTaskRun: true, model.PermExecView: true, model.PermLogView: true, model.PermNodeView: true,
			})
		case "viewer":
			bindRole(role.ID, map[string]bool{
				model.PermTaskView: true, model.PermExecView: true, model.PermLogView: true, model.PermNodeView: true,
			})
		}
	}
}

// seedMenus 写入默认菜单树与按钮（目标扁平结构）。
// 结构：概览 / 节点 / 任务 / 执行记录 / 通知配置（顶级叶子）+ 系统管理（目录：用户 / 角色 / 菜单 / 接口）。
func (s *Store) seedMenus(tx *gorm.DB) error {
	now := time.Now()

	// 顶级叶子菜单
	dashboard := &model.Menu{Name: "Dashboard", Path: "/dashboard", Component: "view/dashboard/Dashboard", Title: "首页", Icon: "Odometer", Sort: 1, Type: "menu", CreatedAt: now}
	nodeMenu := &model.Menu{Name: "Nodes", Path: "/nodes", Component: "view/node/NodeList", Title: "节点管理", Icon: "Monitor", Sort: 2, Type: "menu", PermCode: model.PermNodeView, CreatedAt: now}
	taskMenu := &model.Menu{Name: "Tasks", Path: "/tasks", Component: "view/task/TaskList", Title: "任务", Icon: "List", Sort: 3, Type: "menu", PermCode: model.PermTaskView, CreatedAt: now}
	execMenu := &model.Menu{Name: "Executions", Path: "/executions", Component: "view/execution/ExecutionList", Title: "执行记录", Icon: "Timer", Sort: 4, Type: "menu", PermCode: model.PermExecView, CreatedAt: now}
	notifyMenu := &model.Menu{Name: "Notify", Path: "/notify", Component: "view/settings/NotifyManage", Title: "通知配置", Icon: "Bell", Sort: 5, Type: "menu", PermCode: model.PermNotifyManage, CreatedAt: now}
	for _, m := range []*model.Menu{dashboard, nodeMenu, taskMenu, execMenu, notifyMenu} {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
	}

	// 系统管理：目录（仅具备角色管理权限的角色可见）
	systemDir := &model.Menu{Name: "System", Path: "/system", Title: "系统管理", Icon: "Setting", Sort: 6, Type: "dir", PermCode: model.PermRoleManage, CreatedAt: now}
	if err := tx.Create(systemDir).Error; err != nil {
		return err
	}
	userMenu := &model.Menu{Name: "Users", Path: "/users", Component: "view/user/UserList", Title: "用户管理", Icon: "User", Sort: 1, Type: "menu", PermCode: model.PermUserManage, CreatedAt: now}
	roleMenu := &model.Menu{Name: "Roles", Path: "/roles", Component: "view/role/RoleList", Title: "角色管理", Icon: "Key", Sort: 2, Type: "menu", PermCode: model.PermRoleManage, CreatedAt: now}
	menuMgmt := &model.Menu{Name: "Menus", Path: "/menus", Component: "view/system/MenuManage", Title: "菜单管理", Icon: "Grid", Sort: 3, Type: "menu", PermCode: model.PermRoleManage, CreatedAt: now}
	apiMgmt := &model.Menu{Name: "Apis", Path: "/apis", Component: "view/system/ApiManage", Title: "接口管理", Icon: "Link", Sort: 4, Type: "menu", PermCode: model.PermRoleManage, CreatedAt: now}
	for _, c := range []*model.Menu{userMenu, roleMenu, menuMgmt, apiMgmt} {
		c.ParentID = systemDir.ID
		if err := tx.Create(c).Error; err != nil {
			return err
		}
	}

	// 菜单按钮
	taskBtns := []*model.MenuBtn{
		{Name: "add", Desc: "新增任务", PermCode: model.PermTaskCreate},
		{Name: "edit", Desc: "编辑任务", PermCode: model.PermTaskUpdate},
		{Name: "delete", Desc: "删除任务", PermCode: model.PermTaskDelete},
		{Name: "run", Desc: "执行任务", PermCode: model.PermTaskRun},
	}
	execBtns := []*model.MenuBtn{
		{Name: "log", Desc: "查看日志", PermCode: model.PermLogView},
	}
	nodeBtns := []*model.MenuBtn{
		{Name: "add", Desc: "新增节点", PermCode: model.PermNodeManage},
		{Name: "edit", Desc: "编辑节点", PermCode: model.PermNodeManage},
		{Name: "delete", Desc: "删除节点", PermCode: model.PermNodeManage},
	}
	btnGroups := []struct {
		menuID int64
		btns   []*model.MenuBtn
	}{
		{taskMenu.ID, taskBtns},
		{execMenu.ID, execBtns},
		{nodeMenu.ID, nodeBtns},
	}
	for _, g := range btnGroups {
		for _, b := range g.btns {
			b.MenuID = g.menuID
			b.CreatedAt = now
			if err := tx.Create(b).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// defaultApis 返回系统默认受管 API（path+method）清单。
func defaultApis() []model.Api {
	return []model.Api{
		{Path: "/api/tasks", Method: "GET", Group: "任务管理", Description: "任务列表"},
		{Path: "/api/tasks", Method: "POST", Group: "任务管理", Description: "新增任务"},
		{Path: "/api/tasks/:id", Method: "PUT", Group: "任务管理", Description: "更新任务"},
		{Path: "/api/tasks/:id", Method: "DELETE", Group: "任务管理", Description: "删除任务"},
		{Path: "/api/tasks/:id/run", Method: "POST", Group: "任务管理", Description: "执行任务"},
		{Path: "/api/tasks/:id/enable", Method: "PUT", Group: "任务管理", Description: "启用任务"},
		{Path: "/api/tasks/:id/disable", Method: "PUT", Group: "任务管理", Description: "停用任务"},
		{Path: "/api/executions", Method: "GET", Group: "执行管理", Description: "执行记录"},
		{Path: "/api/executions/:id", Method: "GET", Group: "执行管理", Description: "执行详情"},
		{Path: "/api/executions/:id/logs", Method: "GET", Group: "执行管理", Description: "执行日志"},
		{Path: "/api/executions/task/:taskId/stats", Method: "GET", Group: "执行管理", Description: "任务执行统计"},
		{Path: "/api/executions/task/:taskId/history", Method: "GET", Group: "执行管理", Description: "任务执行历史"},
		{Path: "/api/nodes", Method: "GET", Group: "节点管理", Description: "节点列表"},
		{Path: "/api/nodes", Method: "POST", Group: "节点管理", Description: "新增节点"},
		{Path: "/api/nodes/:id", Method: "PUT", Group: "节点管理", Description: "更新节点"},
		{Path: "/api/nodes/:id", Method: "DELETE", Group: "节点管理", Description: "删除节点"},
		{Path: "/api/users", Method: "GET", Group: "用户管理", Description: "用户列表"},
		{Path: "/api/users", Method: "POST", Group: "用户管理", Description: "新增用户"},
		{Path: "/api/users/:id", Method: "PUT", Group: "用户管理", Description: "更新用户"},
		{Path: "/api/users/:id/password", Method: "PUT", Group: "用户管理", Description: "重置密码"},
		{Path: "/api/users/:id", Method: "DELETE", Group: "用户管理", Description: "删除用户"},
		{Path: "/api/roles", Method: "GET", Group: "角色管理", Description: "角色列表"},
		{Path: "/api/roles", Method: "POST", Group: "角色管理", Description: "新增角色"},
		{Path: "/api/roles/:id", Method: "PUT", Group: "角色管理", Description: "更新角色"},
		{Path: "/api/roles/:id", Method: "DELETE", Group: "角色管理", Description: "删除角色"},
		{Path: "/api/roles/:id/perms", Method: "PUT", Group: "角色管理", Description: "设置角色权限"},
		{Path: "/api/menus", Method: "GET", Group: "菜单管理", Description: "菜单列表"},
		{Path: "/api/menus", Method: "POST", Group: "菜单管理", Description: "新增菜单"},
		{Path: "/api/menus/:id", Method: "PUT", Group: "菜单管理", Description: "更新菜单"},
		{Path: "/api/menus/:id", Method: "DELETE", Group: "菜单管理", Description: "删除菜单"},
		{Path: "/api/menus/:id/btns", Method: "PUT", Group: "菜单管理", Description: "设置菜单按钮"},
		{Path: "/api/apis", Method: "GET", Group: "接口管理", Description: "接口列表"},
		{Path: "/api/apis", Method: "POST", Group: "接口管理", Description: "新增接口"},
		{Path: "/api/apis/:id", Method: "PUT", Group: "接口管理", Description: "更新接口"},
		{Path: "/api/apis/:id", Method: "DELETE", Group: "接口管理", Description: "删除接口"},
		{Path: "/api/roles/:id/menus", Method: "PUT", Group: "角色管理", Description: "设置角色菜单"},
		{Path: "/api/roles/:id/menus/:menuId/btns", Method: "PUT", Group: "角色管理", Description: "设置角色菜单按钮"},
		{Path: "/api/roles/:id/apis", Method: "PUT", Group: "角色管理", Description: "设置角色接口"},
		{Path: "/api/menus/my", Method: "GET", Group: "菜单管理", Description: "当前用户菜单"},
		{Path: "/api/dashboard/overview", Method: "GET", Group: "首页看板", Description: "首页概览指标"},
		{Path: "/api/dashboard/trend", Method: "GET", Group: "首页看板", Description: "执行趋势"},
		{Path: "/api/dashboard/failures", Method: "GET", Group: "首页看板", Description: "异常任务"},
		{Path: "/api/dashboard/recent", Method: "GET", Group: "首页看板", Description: "最近执行"},
		{Path: "/api/dashboard/upcoming", Method: "GET", Group: "首页看板", Description: "即将执行"},
		{Path: "/api/dashboard/resources", Method: "GET", Group: "首页看板", Description: "节点资源"},
		{Path: "/api/notify/settings", Method: "GET", Group: "通知管理", Description: "通知配置查询"},
		{Path: "/api/notify/settings", Method: "PUT", Group: "通知管理", Description: "保存通知配置"},
		{Path: "/api/notify/test", Method: "POST", Group: "通知管理", Description: "发送测试通知"},
	}
}

// ensureApis 确保受管 API 表中包含 defaultApis 定义的全部接口（幂等补齐，兼容既有库升级）。
// 返回是否新增了接口（供调用方决定是否需要重同步内置角色策略）。
func (s *Store) ensureApis(tx *gorm.DB) (bool, error) {
	var existing []model.Api
	if err := tx.Find(&existing).Error; err != nil {
		return false, err
	}
	have := make(map[string]bool, len(existing))
	for _, a := range existing {
		have[a.Path+"|"+strings.ToUpper(a.Method)] = true
	}
	now := time.Now()
	added := false
	for _, a := range defaultApis() {
		if have[a.Path+"|"+a.Method] {
			continue
		}
		if err := tx.Create(&model.Api{
			Path: a.Path, Method: a.Method, Group: a.Group,
			Description: a.Description, CreatedAt: now,
		}).Error; err != nil {
			return false, err
		}
		added = true
	}
	return added, nil
}

// SyncBuiltinRolePermissions 为内置角色绑定默认菜单/按钮/权限点并写入默认 casbin API 策略。
// 幂等：仅补齐缺失的授权，不覆盖管理员的手动调整。
func (s *Store) SyncBuiltinRolePermissions(ctx context.Context, enforcer *casbin.Enforcer) {
	admin, ok := s.GetRoleByCode(ctx, "admin")
	if ok {
		// admin：全部菜单 + 全部 API
		s.bindAllMenus(ctx, admin.ID)
		s.bindAllApis(ctx, enforcer)
	}
	operator, ok := s.GetRoleByCode(ctx, "operator")
	if ok {
		s.ensureRolePerms(ctx, operator.ID, []string{model.PermTaskView, model.PermTaskCreate, model.PermTaskUpdate, model.PermTaskRun, model.PermExecView, model.PermLogView, model.PermNodeView, model.PermDashboardView})
		s.bindRoleMenus(ctx, operator.ID, []string{model.PermTaskView, model.PermTaskCreate, model.PermTaskUpdate, model.PermTaskRun, model.PermExecView, model.PermLogView, model.PermNodeView})
		s.bindRoleApis(ctx, enforcer, operator.ID, []string{
			"GET /api/dashboard/overview", "GET /api/dashboard/trend", "GET /api/dashboard/failures",
			"GET /api/dashboard/recent", "GET /api/dashboard/upcoming", "GET /api/dashboard/resources",
			"GET /api/tasks", "POST /api/tasks", "PUT /api/tasks/:id",
			"POST /api/tasks/:id/run", "PUT /api/tasks/:id/enable", "PUT /api/tasks/:id/disable",
			"GET /api/executions", "GET /api/executions/:id/logs",
			"GET /api/executions/task/:taskId/stats", "GET /api/executions/task/:taskId/history",
			"GET /api/nodes",
		})
	}
	viewer, ok := s.GetRoleByCode(ctx, "viewer")
	if ok {
		s.ensureRolePerms(ctx, viewer.ID, []string{model.PermTaskView, model.PermExecView, model.PermLogView, model.PermNodeView, model.PermDashboardView})
		s.bindRoleMenus(ctx, viewer.ID, []string{model.PermTaskView, model.PermExecView, model.PermLogView, model.PermNodeView})
		s.bindRoleApis(ctx, enforcer, viewer.ID, []string{
			"GET /api/dashboard/overview", "GET /api/dashboard/trend", "GET /api/dashboard/failures",
			"GET /api/dashboard/recent", "GET /api/dashboard/upcoming", "GET /api/dashboard/resources",
			"GET /api/tasks", "GET /api/executions", "GET /api/executions/:id/logs",
			"GET /api/executions/task/:taskId/stats", "GET /api/executions/task/:taskId/history",
			"GET /api/nodes",
		})
	}
}

// ensureRolePerms 幂等补齐角色缺失的权限点（不删除已有权限）。
func (s *Store) ensureRolePerms(ctx context.Context, roleID int64, codes []string) {
	role, ok := s.GetRole(ctx, roleID)
	if !ok {
		return
	}
	have := make(map[int64]bool, len(role.PermIDs))
	for _, pid := range role.PermIDs {
		have[pid] = true
	}
	var perms []model.Permission
	if err := s.db.WithContext(ctx).Where("code IN ?", codes).Find(&perms).Error; err != nil {
		return
	}
	now := time.Now()
	for _, p := range perms {
		if have[p.ID] {
			continue
		}
		s.db.WithContext(ctx).Create(&model.RolePermission{RoleID: roleID, PermissionID: p.ID, CreatedAt: now})
	}
}

// bindAllMenus 为角色绑定全部菜单（幂等：仅补齐缺失的菜单/按钮绑定，重复调用不产生重复数据）。
func (s *Store) bindAllMenus(ctx context.Context, roleID int64) {
	db := s.db.WithContext(ctx)
	var menus []model.Menu
	db.Find(&menus)
	menuSet := s.existingRoleMenuSet(ctx, roleID)
	now := time.Now()
	for _, m := range menus {
		if menuSet[m.ID] {
			continue
		}
		db.Create(&model.RoleMenu{RoleID: roleID, MenuID: m.ID})
	}
	// admin 拥有全部按钮
	var btns []model.MenuBtn
	db.Find(&btns)
	btnSet := s.existingRoleMenuBtnSet(ctx, roleID)
	for _, b := range btns {
		if btnSet[itoa64(b.MenuID)+"|"+itoa64(b.ID)] {
			continue
		}
		db.Create(&model.RoleMenuBtn{RoleID: roleID, MenuID: b.MenuID, BtnID: b.ID, CreatedAt: now})
	}
}

// bindRoleMenus 按权限码为角色绑定对应菜单，并同步按钮（幂等：仅补齐缺失，不覆盖管理员手动调整）。
func (s *Store) bindRoleMenus(ctx context.Context, roleID int64, permCodes []string) {
	db := s.db.WithContext(ctx)
	permSet := make(map[string]bool, len(permCodes))
	for _, p := range permCodes {
		permSet[p] = true
	}
	// 菜单按 PermCode 匹配
	var menus []model.Menu
	db.Find(&menus)
	menuSet := s.existingRoleMenuSet(ctx, roleID)
	btnSet := s.existingRoleMenuBtnSet(ctx, roleID)
	now := time.Now()
	for _, m := range menus {
		if m.PermCode != "" && !permSet[m.PermCode] {
			continue
		}
		if !menuSet[m.ID] {
			db.Create(&model.RoleMenu{RoleID: roleID, MenuID: m.ID})
		}
		// 同步该菜单下 PermCode 匹配的按钮
		for _, b := range s.GetMenuBtns(ctx, m.ID) {
			if b.PermCode != "" && !permSet[b.PermCode] {
				continue
			}
			if btnSet[itoa64(m.ID)+"|"+itoa64(b.ID)] {
				continue
			}
			db.Create(&model.RoleMenuBtn{RoleID: roleID, MenuID: m.ID, BtnID: b.ID, CreatedAt: now})
		}
	}
}

// existingRoleMenuSet 返回该角色已绑定的菜单 ID 集合。
func (s *Store) existingRoleMenuSet(ctx context.Context, roleID int64) map[int64]bool {
	var rms []model.RoleMenu
	s.db.WithContext(ctx).Where("role_id = ?", roleID).Find(&rms)
	set := make(map[int64]bool, len(rms))
	for _, rm := range rms {
		set[rm.MenuID] = true
	}
	return set
}

// existingRoleMenuBtnSet 返回该角色已绑定的按钮集合，key 为 "menuID|btnID"。
func (s *Store) existingRoleMenuBtnSet(ctx context.Context, roleID int64) map[string]bool {
	var rbs []model.RoleMenuBtn
	s.db.WithContext(ctx).Where("role_id = ?", roleID).Find(&rbs)
	set := make(map[string]bool, len(rbs))
	for _, rb := range rbs {
		set[itoa64(rb.MenuID)+"|"+itoa64(rb.BtnID)] = true
	}
	return set
}

// itoa64 将 int64 转十进制字符串（避免引入 strconv 的额外负担可自行简化）。
func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}

// bindAllApis 为角色绑定全部受管 API（admin）。
func (s *Store) bindAllApis(ctx context.Context, enforcer *casbin.Enforcer) {
	var apis []model.Api
	s.db.WithContext(ctx).Find(&apis)
	admin, _ := s.GetRoleByCode(ctx, "admin")
	if admin == nil {
		return
	}
	enforcer.ClearRolePolicy(admin.ID)
	enforcer.AddPolicies(admin.ID, apis)
}

// bindRoleApis 为角色绑定指定 API（按 "METHOD path" 字符串）。
func (s *Store) bindRoleApis(ctx context.Context, enforcer *casbin.Enforcer, roleID int64, specs []string) {
	var apis []model.Api
	s.db.WithContext(ctx).Find(&apis)
	apiBySpec := make(map[string]model.Api, len(apis))
	for _, a := range apis {
		apiBySpec[a.Method+" "+a.Path] = a
	}
	var selected []model.Api
	for _, spec := range specs {
		if a, ok := apiBySpec[spec]; ok {
			selected = append(selected, a)
		}
	}
	enforcer.ClearRolePolicy(roleID)
	enforcer.AddPolicies(roleID, selected)
}
