# goscheduler 功能与交互参考

本文件详细记录 `goscheduler` 的功能模块、数据模型与前端交互模式，供构建调度平台时**全量搬迁**其功能并做企业级增强。

> 后端技术栈：goscheduler 用 Macaron + Xorm；调度平台升级为 **Gin + GORM**（默认 SQLite，可切 MySQL）。本文件在交互/字段层面保持对齐，迁移技术栈见 `gorm-mysql-persistence.md`。

## 1. 功能模块总览

| 模块 | 页面 | 核心能力 |
|---|---|---|
| 任务管理 | `task/list.vue` `task/edit.vue` | 创建/编辑/启停/删除/手动执行/查看日志/依赖任务 |
| 执行日志 | `taskLog/list.vue` | 按任务/节点查看日志 |
| 节点管理 | `host/list.vue` `host/edit.vue` | 节点增删改查、状态 |
| 用户权限 | `user/list.vue` `user/edit.vue` | 用户 CRUD、修改密码、admin 判断 |
| 系统设置 | `system/` | 通知渠道（email/slack/webhook）、登录日志 |
| 安装引导 | `install/index.vue` | 首次安装初始化 |
| 后端路由 | `internal/routers/` | task/tasklog/host/user/loginlog/install/base/manage |

## 2. 数据模型（Xorm → GORM 映射，全量搬迁）

### 2.1 task（任务）
字段（搬迁 + 增强）：
- `Id/Name`、`Level`（1 父任务/2 子任务）
- `DependencyTaskId`（依赖任务ID，逗号分隔）、`DependencyStatus`（1 强依赖/2 弱依赖）
- `Spec`（crontab）、`Protocol`（1 http/2 rpc）
- `Command`（URL 或 shell 命令）、`RequestBody`（http post 请求体）、`HttpMethod`（1 GET/2 POST）
- `Timeout`（秒，0 不限制）、`Multi`（是否允许多实例）、`RetryTimes`、`RetryInterval`
- `NotifyStatus`（0 不通知/1 失败通知/2 执行结束通知/3 关键字匹配）、`NotifyType`（1 邮件/2 slack/3 webhook）、`NotifyReceiverId`（setting 主键，逗号分隔）、`NotifyKeyword`
- `Tag`、`Remark`、`Status`（1 正常/0 停止）、`Created/Deleted`（软删除）
- 关系字段：`Hosts []TaskHostDetail`（关联节点）、`NextRunTime`

### 2.2 task_host（任务-节点多对多）
- `Id/TaskId/HostId`，`TaskHostDetail` 联表带出 `Name/Port/Alias`
- 关键方法：`Add(taskId, hostIds)`（先删后插）、`GetHostIdsByTaskId`、`GetTaskIdsByHostId`、`HostIdExist`

### 2.3 host（节点）
- `Id/Name/Alias/Port(默认5921)/Remark`、`Selected`

### 2.4 task_log（执行日志）
- `Id/TaskId/Name/Spec/Protocol/Command/Timeout/RetryTimes/Hostname/StartTime/EndTime/Status/Result/TotalTime`
- `Status`：0 失败 / 1 执行中 / 2 完成 / 3 取消 / 4 异步
- `TotalTime` 为计算字段（EndTime - StartTime，执行中按 Now 计算）

### 2.5 user（用户）
- `Id/Name/Password/Salt(6位)/Email/IsAdmin/Status`
- 密码：`Md5(password + salt)` 加盐加密；`UpdatePassword` 重新生成盐

### 2.6 login_log（登录日志）
- `Id/Username/Ip/Created`

### 2.7 setting（通知渠道配置）
- `Id/Code/Key/Value` 键值存储
- 渠道：`slack`（url/template/channel）、`mail`（server/template/user）、`webhook`（url/template）
- 模板：slack/email/webhook 各有默认模板（`{{.TaskId}}` `{{.TaskName}}` `{{.Status}}` `{{.Result}}` `{{.Remark}}`）

### 2.8 migration（建表与升级）
- `Install(dbName)`：首次安装 Sync2 建全部表 + `InitBasicField()`
- `Upgrade(oldVersionId)`：版本化升级（110/122/130/140/150/154），事务包裹

### 2.9 全局状态常量
```go
Disabled = 0 // 禁用/失败
Enabled  = 1 // 启用/运行中
Finish   = 2 // 完成
Cancel   = 3 // 取消
```

## 3. 功能清单（全量搬迁核对表）

构建调度平台时，以下 goscheduler 能力**必须全部覆盖**：

- [ ] 任务列表搜索（任务ID/名称/标签/协议/节点/状态）
- [ ] 任务创建/编辑（含请求体、重试、超时、通知、依赖、标签、备注）
- [ ] 任务启停开关（管理员可切换，普通用户只读）
- [ ] 任务手动执行（二次确认）
- [ ] 任务删除（二次确认）
- [ ] 任务行展开详情（创建时间/类型/单实例/超时/重试/命令/备注/关联节点）
- [ ] 任务名称唯一校验
- [ ] 依赖任务（父/子任务，强/弱依赖，DependencyTaskId 逗号分隔）
- [ ] 任务-节点多对多绑定
- [ ] 执行日志（按任务/节点/协议/状态过滤，计算耗时）
- [ ] 日志清空、删除 N 个月前日志
- [ ] 节点增删改查 + 名称唯一 + 被引用检查
- [ ] 用户 CRUD、改密（管理员/自己）、用户名/邮箱唯一、启停
- [ ] 登录（用户名或邮箱 + 加盐密码）
- [ ] 登录日志
- [ ] 通知渠道配置（email/slack/webhook + 渠道增删 + 模板）
- [ ] 安装引导（首次建库建表 + 初始化 admin + 通知基础字段）

## 4. 任务管理（task）

### 4.1 任务列表 `task/list.vue`

搜索条件：任务ID、任务名称、标签、执行方式、任务节点、状态。

```html
<el-form :inline="true">
  <el-row>
    <el-form-item label="任务ID"><el-input v-model.trim="searchParams.id"></el-input></el-form-item>
    <el-form-item label="任务名称"><el-input v-model.trim="searchParams.name"></el-input></el-form-item>
    <el-form-item label="标签"><el-input v-model.trim="searchParams.tag"></el-input></el-form-item>
  </el-row>
  <el-row>
    <el-form-item label="执行方式">
      <el-select v-model.trim="searchParams.protocol">
        <el-option label="全部" value=""></el-option>
        <el-option v-for="item in protocolList" :key="item.value" :label="item.label" :value="item.value"></el-option>
      </el-select>
    </el-form-item>
    <!-- 节点、状态 -->
    <el-form-item><el-button type="primary" @click="search()">搜索</el-button></el-form-item>
  </el-row>
</el-form>
```

操作列：
- 编辑、手动执行（需二次确认 `$appConfirm`）
- 查看日志、删除（需二次确认）
- 启停开关：管理员可切换，普通用户只读

```html
<el-switch
  v-model="scope.row.status"
  :active-value="1"
  :inactive-value="0"
  active-color="#13ce66"
  inactive-color="#ff4949"
  @change="changeStatus(scope.row)">
</el-switch>
```

行展开（`type="expand"`）展示详情：创建时间、任务类型、单实例、超时、重试、命令、备注。

分页使用 `el-pagination`，支持 `prev/pager/next/sizes/total`。

### 4.2 权限控制

```js
isAdmin: this.$store.getters.user.isAdmin
```
- 管理员：完整操作（新增/编辑/删除/启停/手动执行）
- 普通用户：查看 + 只读状态

### 4.3 关键交互

- **手动执行**：`taskService.run(id, cb)`，成功后 `$message.success('任务已开始执行')`
- **删除**：`this.$appConfirm(() => { taskService.remove(id, ...) })` 二次确认
- **跳转日志**：`this.$router.push(`/task/log?task_id=${item.id}`)`

## 5. 执行日志（taskLog）

按任务 ID 或节点过滤查询执行记录，展示：
- 任务名、节点、执行结果、耗时、开始/结束时间
- 支持查看任务执行历史

## 6. 节点管理（host）

`host/list.vue` 维护任务节点（Worker）：
- 节点别名 + 名称 + 端口 + 状态
- 创建/编辑节点信息
- 节点与任务关联（一个任务可绑定多节点）

任务详情中展示关联节点：
```html
<div v-for="item in scope.row.hosts" :key="item.host_id">
  {{item.alias}} - {{item.name}}:{{item.port}}
</div>
```

## 7. 用户权限（user）

- `login.vue`：登录对话框，成功后写入 store（token/uid/username/isAdmin），跳转 redirect。
- `list.vue`：用户列表，admin 可管理用户。
- `edit.vue` / `editPassword.vue` / `editMyPassword.vue`：创建用户、管理员改密、用户改自己密码。

登录后 store 结构：

```js
this.$store.commit('setUser', {
  token: data.token,
  uid: data.uid,
  username: data.username,
  isAdmin: data.is_admin
})
this.$router.push(this.$route.query.redirect || '/')
```

## 8. 应用骨架（App.vue + main.js）

### App.vue 布局

```
<el-container>
  <el-header>
    <app-header></app-header>
    <app-nav-menu></app-nav-menu>
  </el-header>
  <el-main>
    <div id="main-container" v-cloak><router-view/></div>
  </el-main>
  <el-footer><app-footer></app-footer></el-footer>
</el-container>
```

- `v-cloak` 防止闪烁
- 安装检测：`installService.status()` 若未安装跳转 `/install`

### main.js 全局能力

```js
// 全局二次确认
Vue.prototype.$appConfirm = function (callback) {
  this.$confirm('确定执行此操作?', '提示', {
    confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning'
  }).then(() => { callback() })
}

// 全局时间格式化 filter
Vue.filter('formatTime', function (time) { /* ... */ })
```

## 9. 应用到调度平台的建议

构建调度平台时，参考 goscheduler 的以下交互：

1. **搜索表单**：`el-form inline` 多行搜索 + `el-select` 下拉 + `el-button` 搜索。
2. **表格**：`el-table` + `el-table-column`，展开行展示详情，`el-switch` 启停。
3. **二次确认**：所有删除/停止/执行 Shell 等危险操作使用确认对话框。
4. **分页**：`el-pagination` 完整布局。
5. **权限**：基于 `isAdmin` 判断操作权限，非管理员只读。
6. **消息反馈**：操作成功 `$message.success(...)`，失败 `$message.error(...)`。
7. **路由跳转**：列表到详情/日志通过 `$router.push` 带 query 参数。

## 10. 新旧差异（goscheduler → 调度平台）

goscheduler 是 Vue2 + Element UI，调度平台升级为 **Vue3 + Element Plus + Vite + Pinia**，需迁移：

| 旧（goscheduler） | 新（调度平台） |
|---|---|
| Vue2 | Vue3 |
| Element UI | Element Plus |
| webpack | Vite |
| Vuex | Pinia |
| options API | Composition API / `<script setup>` |
| Macaron + Xorm | Gin + GORM |
| 无 Dashboard | 丰富首页 Dashboard |

功能演进建议：
- 保留：任务 CRUD、启停、手动执行、日志查看、节点管理、用户权限、登录。
- 新增：首页 Dashboard、执行趋势图、节点资源监控、异常任务告警、执行重试策略。
- 升级：任务类型从 HTTP/Shell 扩展到 Cron/gRPC，增加失败重试、超时、并发控制。
- 持久化：Xorm 全部迁移到 **GORM**（默认 SQLite / 可切 MySQL，见 `gorm-mysql-persistence.md`）。
- 增强：DAG 依赖、RBAC 权限、操作审计、任务版本、告警中心（见 `enhanced-features.md`）。
