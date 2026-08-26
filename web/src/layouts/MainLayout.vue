<template>
  <div class="layout">
    <!-- 深色侧边栏 -->
    <aside class="sidebar" :class="{ collapsed }">
      <div class="logo">
        <div class="logo-badge">
          <el-icon :size="22" color="#fff"><SetUp /></el-icon>
        </div>
        <transition name="fade">
          <div v-if="!collapsed" class="logo-text">
            <span class="logo-name">调度平台</span>
            <span class="logo-sub">Scheduler</span>
          </div>
        </transition>
      </div>

      <div class="menu-scroll">
        <el-menu
          :default-active="$route.path"
          :collapse="collapsed"
          router
          class="side-menu"
          background-color="transparent"
          text-color="var(--sidebar-text)"
          active-text-color="#fff"
          :collapse-transition="false"
        >
          <template v-for="item in menuItems" :key="item.path">
            <!-- 有可见子菜单 → 子菜单展开 -->
            <el-sub-menu v-if="visibleChildren(item).length" :index="item.path">
              <template #title>
                <el-icon><component :is="item.icon || 'Menu'" /></el-icon>
                <span>{{ item.title }}</span>
              </template>
              <el-menu-item v-for="child in visibleChildren(item)" :key="child.path" :index="child.path">
                <el-icon><component :is="child.icon || 'Menu'" /></el-icon>
                <span>{{ child.title }}</span>
              </el-menu-item>
            </el-sub-menu>
            <!-- 普通菜单（含隐藏子菜单的顶级入口，如「设置」→ 进入设置页） -->
            <el-menu-item v-else-if="!item.hidden" :index="item.path">
              <el-icon><component :is="item.icon || 'Menu'" /></el-icon>
              <span>{{ item.title }}</span>
            </el-menu-item>
          </template>
        </el-menu>
      </div>

      <div class="sidebar-footer">
        <div class="version-chip">v1.0.0</div>
      </div>
    </aside>

    <!-- 右侧主区域 -->
    <div class="main-area" :class="{ collapsed }">
      <!-- 顶栏 -->
      <header class="header">
        <div class="header-left">
          <div class="collapse-btn" @click="collapsed = !collapsed">
            <el-icon :size="18">
              <Expand v-if="collapsed" />
              <Fold v-else />
            </el-icon>
          </div>
          <el-breadcrumb separator="/" class="breadcrumb">
            <el-breadcrumb-item :to="{ path: '/dashboard' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item v-if="!isHome">{{ $route.meta.title }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>

        <div class="header-right">
          <el-tooltip content="当前环境：生产" placement="bottom">
            <div class="env-tag">
              <span class="env-dot"></span>
              生产环境
            </div>
          </el-tooltip>
          <el-dropdown @command="handleCommand" trigger="click">
            <div class="user-info">
              <el-avatar :size="32" class="user-avatar">{{ avatarText }}</el-avatar>
              <div class="user-meta">
                <span class="user-name">{{ userStore.nickname }}</span>
                <span class="user-role">{{ userStore.roleName || '用户' }}</span>
              </div>
              <el-icon class="arrow"><ArrowDown /></el-icon>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile">
                  <el-icon><Setting /></el-icon>个人设置
                </el-dropdown-item>
                <el-dropdown-item divided command="logout">
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <!-- 标签页 -->
      <div class="tabs-bar">
        <div class="tabs-scroll">
          <div
            v-for="t in tabs"
            :key="t.path"
            class="tab-item"
            :class="{ active: $route.path === t.path }"
            @click="router.push(t.path)"
          >
            <span>{{ t.title }}</span>
            <el-icon v-if="t.closable" class="tab-close" @click.stop="closeTab(t)"><Close /></el-icon>
          </div>
        </div>
      </div>

      <!-- 主内容 -->
      <main class="main">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" :key="$route.path" />
          </transition>
        </router-view>
      </main>
    </div>

    <!-- 个人设置弹窗 -->
    <el-dialog v-model="profileVisible" title="个人设置" width="440px" align-center>
      <el-form label-width="80px">
        <el-form-item label="昵称">
          <el-input v-model="profileForm.nickname" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="profileForm.email" />
        </el-form-item>
        <el-form-item label="角色">
          <el-tag v-if="userStore.isAdmin" type="danger" effect="dark">管理员</el-tag>
          <el-tag v-else>{{ userStore.roleName || '用户' }}</el-tag>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="profileVisible = false">关闭</el-button>
        <el-button type="primary" @click="saveProfile">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { ElMessage } from 'element-plus'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const collapsed = ref(false)
const profileVisible = ref(false)
const profileForm = ref({ nickname: '', email: '' })

const menuItems = computed(() => userStore.menus || [])
const isHome = computed(() => route.path === '/dashboard')

// 仅返回可见（未隐藏）的子菜单，用于判断是否渲染子菜单
function visibleChildren(item) {
  return (item.children || []).filter((c) => !c.hidden)
}

const avatarText = computed(() => (userStore.nickname || 'U').slice(0, 1).toUpperCase())

// 打开个人设置时填充当前信息
watch(profileVisible, (v) => {
  if (v) {
    profileForm.value = {
      nickname: userStore.user?.nickname || '',
      email: userStore.user?.email || ''
    }
  }
})

function handleCommand(command) {
  if (command === 'logout') {
    userStore.logout()
    router.push('/login')
  } else if (command === 'profile') {
    profileVisible.value = true
  }
}

async function saveProfile() {
  // 这里可接入后端更新个人资料接口
  ElMessage.success('个人资料保存成功')
  profileVisible.value = false
}

// 顶部标签页（面包屑的延伸，便于多页面快速切换）
const tabs = ref([])
function ensureTab() {
  const path = route.path
  if (path === '/dashboard' || !route.meta.title) return
  if (!tabs.value.some((t) => t.path === path)) {
    tabs.value.push({ path, title: route.meta.title, closable: true })
  }
}
function closeTab(t) {
  const idx = tabs.value.findIndex((x) => x.path === t.path)
  if (idx > -1) tabs.value.splice(idx, 1)
  if (route.path === t.path) {
    const prev = tabs.value[idx - 1] || tabs.value[idx]
    router.push(prev ? prev.path : '/dashboard')
  }
}
watch(() => route.path, ensureTab, { immediate: true })
</script>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  width: 100%;
  overflow: hidden;
}

/* ============ 侧边栏 ============ */
.sidebar {
  width: var(--sidebar-width);
  background: var(--sidebar-bg);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  transition: width 0.28s cubic-bezier(0.16, 1, 0.3, 1);
  position: relative;
  z-index: 20;
  overflow: hidden;
}
.sidebar.collapsed {
  width: 64px;
}
.logo {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 60px;
  padding: 0 16px;
  flex-shrink: 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  overflow: hidden;
  white-space: nowrap;
}
.logo-badge {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  background: var(--primary-gradient);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 12px rgba(79, 110, 247, 0.4);
  flex-shrink: 0;
}
.logo-text {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}
.logo-name {
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 0.5px;
}
.logo-sub {
  color: var(--sidebar-text);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 1.5px;
}
.menu-scroll {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 10px 8px;
}
.sidebar ::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.15); }

.side-menu {
  border-right: none;
  --el-menu-bg-color: transparent;
  --el-menu-text-color: var(--sidebar-text);
  --el-menu-active-color: #fff;
}
.side-menu :deep(.el-menu-item),
.side-menu :deep(.el-sub-menu__title) {
  height: 44px;
  line-height: 44px;
  margin: 3px 0;
  border-radius: 8px;
  color: var(--sidebar-text);
  font-weight: 500;
}
.side-menu :deep(.el-menu-item.is-active) {
  background: var(--sidebar-active);
  color: #fff !important;
  box-shadow: 0 4px 14px rgba(79, 110, 247, 0.4);
}
.side-menu :deep(.el-menu-item:hover),
.side-menu :deep(.el-sub-menu__title:hover) {
  background: var(--sidebar-hover);
  color: #fff;
}
.side-menu :deep(.el-menu-item .el-icon),
.side-menu :deep(.el-sub-menu__title .el-icon) {
  color: inherit;
}
.sidebar-footer {
  padding: 12px 16px;
  flex-shrink: 0;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}
.version-chip {
  color: var(--sidebar-text);
  font-size: 12px;
  text-align: center;
  letter-spacing: 1px;
}

/* ============ 主区域 ============ */
.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: var(--bg-color);
}
.header {
  height: 60px;
  background: #fff;
  border-bottom: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  flex-shrink: 0;
  box-shadow: 0 1px 2px rgba(30, 41, 59, 0.04);
}
.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}
.collapse-btn {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: #64748b;
  cursor: pointer;
  transition: background 0.2s;
}
.collapse-btn:hover {
  background: #f1f5f9;
  color: var(--primary-color);
}
.breadcrumb {
  font-size: 14px;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 20px;
}
.env-tag {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #64748b;
  background: #f1f5f9;
  padding: 4px 12px;
  border-radius: 20px;
  font-weight: 500;
}
.env-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.2);
}
.user-info {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 10px;
  transition: background 0.2s;
  outline: none;
}
.user-info:hover { background: #f1f5f9; }
.user-avatar {
  background: var(--primary-gradient);
  font-weight: 600;
  color: #fff;
}
.user-meta {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}
.user-name {
  font-size: 14px;
  font-weight: 600;
  color: #1e293b;
}
.user-role {
  font-size: 11px;
  color: #94a3b8;
}
.arrow {
  color: #94a3b8;
  font-size: 12px;
}

/* ============ 标签页 ============ */
.tabs-bar {
  height: 40px;
  background: #fff;
  border-bottom: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  padding: 0 16px;
  flex-shrink: 0;
}
.tabs-scroll {
  display: flex;
  gap: 8px;
  overflow-x: auto;
}
.tab-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  font-size: 12px;
  border-radius: 6px;
  color: #64748b;
  cursor: pointer;
  white-space: nowrap;
  border: 1px solid transparent;
  transition: all 0.2s;
}
.tab-item:hover { background: #f1f5f9; }
.tab-item.active {
  background: var(--primary-light);
  color: var(--primary-color);
  font-weight: 600;
  border-color: rgba(79, 110, 247, 0.2);
}
.tab-close { font-size: 12px; opacity: 0.6; }
.tab-close:hover { opacity: 1; color: var(--danger-color); }

/* ============ 主内容 ============ */
.main {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
}
.page-enter-active,
.page-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}
.page-enter-from { opacity: 0; transform: translateY(8px); }
.page-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
