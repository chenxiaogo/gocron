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
		// 0.1 菜单结构迁移：旧的「系统管理」目录 → 新的「设置」目录
		s.migrateSystemMenu(tx)
		// 0.2 菜单结构迁移：任务/执行/节点 → 「任务管理」目录下子菜单；用户/角色/菜单/接口 → 「设置」目录下子菜单
		menuMigrated = s.migrateMenuStructure(tx)

		// 1. 菜单 + 按钮
		var menuCount int64
		if err := tx.Model(&model.Menu{}).Count(&menuCount).Error; err != nil {
			return err
		}
		if menuCount == 0 {
			if err := s.seedMenus(tx); err != nil {
				return err
			}
		} else {
			// 旧库升级：幂等补齐通知配置菜单（挂到设置目录下）
			s.ensureNotifyMenu(tx)
		}

		// 2. 受管 API（幂等：缺失的 API 自动补齐，兼容既有库升级）
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
		"view/dashboard/index":   "view/dashboard/Dashboard",
		"view/task/index":        "view/task/TaskList",
		"view/execution/index":   "view/execution/ExecutionList",
		"view/node/index":        "view/node/NodeList",
		"view/user/index":        "view/user/UserList",
		"view/role/index":        "view/role/RoleList",
		"view/system/menu":       "view/system/MenuManage",
		"view/system/api":        "view/system/ApiManage",
		"view/task/list":         "view/task/TaskList",
		"view/execution/list":    "view/execution/ExecutionList",
		"view/node/list":         "view/node/NodeList",
		"view/user/list":         "view/user/UserList",
		"view/role/list":         "view/role/RoleList",
		"view/system/menus":      "view/system/MenuManage",
		"view/system/apis":       "view/system/ApiManage",
	}
	for old, current := range legacy {
		tx.Model(&model.Menu{}).Where("component = ?", old).Update("component", current)
	}
}

// migrateSystemMenu 将旧的「系统管理」目录迁移为新的「设置」顶级菜单。
// 幂等：仅当存在名为 System 的顶级目录时才处理；子菜单（用户/角色/菜单/接口）保留路由但改为隐藏，
// 使它们不在侧边栏单独展示，而是由设置页按权限以内嵌标签页呈现。
func (s *Store) migrateSystemMenu(tx *gorm.DB) {
	var sys model.Menu
	err := tx.Where("name = ? AND type = ? AND parent_id = 0", "System", "dir").First(&sys).Error
	if err != nil || sys.ID == 0 {
		// 无旧结构（新库或已迁移），无需处理
		return
	}
	// 更新为「设置」目录，其下子菜单在侧边栏展开显示
	updates := map[string]interface{}{
		"title":     "设置",
		"icon":      "Setting",
		"path":      "/settings",
		"type":      "dir",
		"component": "",
		"name":      "Settings",
		"perm_code": model.PermRoleManage,
	}
	tx.Model(&model.Menu{}).Where("id = ?", sys.ID).Updates(updates)
	// 子菜单取消隐藏，改为在侧边栏展开显示
	tx.Model(&model.Menu{}).Where("parent_id = ?", sys.ID).Update("hidden", false)
}

// migrateMenuStructure 将旧版菜单结构迁移为新版多级结构。
// 目标结构：首页(menu) / 任务管理(dir：任务列表/执行记录/节点管理) / 设置(dir：用户/角色/菜单/接口)
// 兼容多种历史结构：任务/执行/节点可能为独立顶级 menu，用户/角色/菜单/接口可能为顶级 menu。
// 返回是否发生了迁移（调用方需据此在事务提交后重建内置角色菜单绑定）。
// 幂等：以 name 为锚点定位菜单，检测到已迁移则仅补全设置子项可见性并返回 false。
func (s *Store) migrateMenuStructure(tx *gorm.DB) bool {
	getByName := func(name string) *model.Menu {
		var m model.Menu
		if err := tx.Where("name = ?", name).First(&m).Error; err != nil {
			return nil
		}
		return &m
	}

	tasks := getByName("Tasks")
	if tasks == nil {
		return false
	}
	taskList := getByName("TaskList")
	// 幂等判断：任务管理已是目录且存在挂在其下的任务列表子菜单 → 已迁移
	if tasks.Type == "dir" && tasks.ParentID == 0 && taskList != nil && taskList.ParentID == tasks.ID {
		if settings := getByName("Settings"); settings != nil {
			tx.Model(&model.Menu{}).Where("parent_id = ?", settings.ID).Update("hidden", false)
		}
		return false
	}

	now := time.Now()

	// 1. 任务管理 → 目录
	tx.Model(&model.Menu{}).Where("id = ?", tasks.ID).Updates(map[string]interface{}{
		"type": "dir", "title": "任务管理", "icon": "List", "component": "",
		"path": "/tasks", "perm_code": model.PermTaskView,
	})

	// 2. 执行管理 → 任务管理下的「执行记录」子菜单
	if execs := getByName("Executions"); execs != nil {
		tx.Model(&model.Menu{}).Where("id = ?", execs.ID).Updates(map[string]interface{}{
			"parent_id": tasks.ID, "name": "ExecutionList", "title": "执行记录",
			"path": "/tasks/executions", "sort": 2, "hidden": false, "perm_code": model.PermExecView,
		})
	}

	// 3. 节点管理 → 任务管理下的「节点管理」子菜单
	if nodes := getByName("Nodes"); nodes != nil {
		tx.Model(&model.Menu{}).Where("id = ?", nodes.ID).Updates(map[string]interface{}{
			"parent_id": tasks.ID, "name": "NodeList", "title": "节点管理",
			"path": "/tasks/nodes", "sort": 3, "hidden": false, "perm_code": model.PermNodeView,
		})
	}

	// 4. 新增/对齐「任务列表」子菜单，并将 Tasks 原按钮归并到其上
	if taskList == nil {
		taskList = &model.Menu{Name: "TaskList", Path: "/tasks/list", Component: "view/task/TaskList", Title: "任务列表", Icon: "List", Sort: 1, Type: "menu", PermCode: model.PermTaskView, ParentID: tasks.ID, CreatedAt: now}
		tx.Create(taskList)
	}
	tx.Model(&model.Menu{}).Where("id = ?", taskList.ID).Updates(map[string]interface{}{
		"parent_id": tasks.ID, "path": "/tasks/list", "component": "view/task/TaskList",
		"title": "任务列表", "sort": 1, "hidden": false, "perm_code": model.PermTaskView,
	})
	tx.Model(&model.MenuBtn{}).Where("menu_id = ?", tasks.ID).Update("menu_id", taskList.ID)

	// 5. 设置 → 目录；将顶级/子级 Users/Roles/Menus/Apis 统一归入其下
	settings := getByName("Settings")
	if settings != nil {
		tx.Model(&model.Menu{}).Where("id = ?", settings.ID).Updates(map[string]interface{}{
			"type": "dir", "title": "设置", "icon": "Setting", "path": "/settings",
			"component": "", "perm_code": model.PermRoleManage,
		})
		children := []struct {
			name string
			path string
		}{
			{"Users", "/settings/users"},
			{"Roles", "/settings/roles"},
			{"Menus", "/settings/menus"},
			{"Apis", "/settings/apis"},
		}
		for i, c := range children {
			if cm := getByName(c.name); cm != nil {
				tx.Model(&model.Menu{}).Where("id = ?", cm.ID).Updates(map[string]interface{}{
					"parent_id": settings.ID, "path": c.path, "hidden": false, "sort": i + 1,
				})
			}
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

// seedMenus 写入默认菜单树与按钮。
// 新库初始化时的菜单结构（旧库由 migrateMenuStructure 迁移对齐）：
//   首页（顶级）｜任务管理（目录：任务列表 / 执行记录 / 节点管理）｜设置（目录：用户 / 角色 / 菜单 / 接口）
func (s *Store) seedMenus(tx *gorm.DB) error {
	now := time.Now()

	// 顶级菜单：首页
	dashboard := &model.Menu{Name: "Dashboard", Path: "/dashboard", Component: "view/dashboard/Dashboard", Title: "首页", Icon: "Odometer", Sort: 1, Type: "menu", CreatedAt: now}
	if err := tx.Create(dashboard).Error; err != nil {
		return err
	}

	// 任务管理：目录，其下含任务列表 / 执行记录 / 节点管理
	taskDir := &model.Menu{Name: "Tasks", Path: "/tasks", Title: "任务管理", Icon: "List", Sort: 2, Type: "dir", PermCode: model.PermTaskView, CreatedAt: now}
	if err := tx.Create(taskDir).Error; err != nil {
		return err
	}
	taskList := &model.Menu{Name: "TaskList", Path: "/tasks/list", Component: "view/task/TaskList", Title: "任务列表", Icon: "List", Sort: 1, Type: "menu", PermCode: model.PermTaskView, CreatedAt: now}
	execList := &model.Menu{Name: "ExecutionList", Path: "/tasks/executions", Component: "view/execution/ExecutionList", Title: "执行记录", Icon: "Timer", Sort: 2, Type: "menu", PermCode: model.PermExecView, CreatedAt: now}
	nodeList := &model.Menu{Name: "NodeList", Path: "/tasks/nodes", Component: "view/node/NodeList", Title: "节点管理", Icon: "Monitor", Sort: 3, Type: "menu", PermCode: model.PermNodeView, CreatedAt: now}
	taskChildren := []*model.Menu{taskList, execList, nodeList}
	for _, c := range taskChildren {
		c.ParentID = taskDir.ID
		if err := tx.Create(c).Error; err != nil {
			return err
		}
	}

	// 设置：目录，其下含用户 / 角色 / 菜单 / 接口管理
	// 挂 PermRoleManage，保证仅具备角色管理权限的角色（默认仅 admin）能看到「设置」入口。
	settingsDir := &model.Menu{Name: "Settings", Path: "/settings", Title: "设置", Icon: "Setting", Sort: 3, Type: "dir", PermCode: model.PermRoleManage, CreatedAt: now}
	if err := tx.Create(settingsDir).Error; err != nil {
		return err
	}
	userMenu := &model.Menu{Name: "Users", Path: "/settings/users", Component: "view/user/UserList", Title: "用户管理", Icon: "User", Sort: 1, Type: "menu", PermCode: model.PermUserManage, CreatedAt: now}
	roleMenu := &model.Menu{Name: "Roles", Path: "/settings/roles", Component: "view/role/RoleList", Title: "角色管理", Icon: "Key", Sort: 2, Type: "menu", PermCode: model.PermRoleManage, CreatedAt: now}
	menuMgmt := &model.Menu{Name: "Menus", Path: "/settings/menus", Component: "view/system/MenuManage", Title: "菜单管理", Icon: "Grid", Sort: 3, Type: "menu", PermCode: model.PermRoleManage, CreatedAt: now}
	apiMgmt := &model.Menu{Name: "Apis", Path: "/settings/apis", Component: "view/system/ApiManage", Title: "接口管理", Icon: "Link", Sort: 4, Type: "menu", PermCode: model.PermRoleManage, CreatedAt: now}
	notifyMenu := &model.Menu{Name: "Notify", Path: "/settings/notify", Component: "view/settings/NotifyManage", Title: "通知配置", Icon: "Bell", Sort: 5, Type: "menu", PermCode: model.PermNotifyManage, CreatedAt: now}
	settingChildren := []*model.Menu{userMenu, roleMenu, menuMgmt, apiMgmt, notifyMenu}
	for _, c := range settingChildren {
		c.ParentID = settingsDir.ID
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
		{taskList.ID, taskBtns},
		{execList.ID, execBtns},
		{nodeList.ID, nodeBtns},
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

// ensureNotifyMenu 幂等补齐「通知配置」子菜单（挂到设置目录下）。
// 用于既有库升级：seedMenus 只在新库调用，此处保证旧库也能出现通知菜单。
func (s *Store) ensureNotifyMenu(tx *gorm.DB) {
	var settings model.Menu
	if err := tx.Where("name = ? AND type = ?", "Settings", "dir").First(&settings).Error; err != nil || settings.ID == 0 {
		return
	}
	var n model.Menu
	if err := tx.Where("name = ?", "Notify").First(&n).Error; err == nil && n.ID != 0 {
		// 已存在：仅对齐挂载关系
		tx.Model(&model.Menu{}).Where("id = ?", n.ID).Updates(map[string]interface{}{
			"parent_id": settings.ID, "path": "/settings/notify", "hidden": false, "sort": 5,
		})
		return
	}
	tx.Create(&model.Menu{
		Name: "Notify", ParentID: settings.ID, Path: "/settings/notify",
		Component: "view/settings/NotifyManage", Title: "通知配置", Icon: "Bell",
		Sort: 5, Type: "menu", PermCode: model.PermNotifyManage, CreatedAt: time.Now(),
	})
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
