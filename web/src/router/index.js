import { createRouter, createWebHashHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { getMyMenus } from '@/api/menu'
import { getMe } from '@/api/auth'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/login/Login.vue'),
      meta: { public: true }
    },
    {
      path: '/',
      name: 'Root',
      component: () => import('@/layouts/MainLayout.vue'),
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'Dashboard',
          component: () => import('@/views/dashboard/Dashboard.vue'),
          meta: { title: '首页', icon: 'Odometer' }
        },
        {
          path: 'nodes/:id/tasks',
          name: 'NodeTasks',
          component: () => import('@/views/node/NodeTasks.vue'),
          meta: { title: '节点任务', icon: 'List' }
        }
      ]
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/dashboard'
    }
  ]
})

// 前端视图组件映射：component 字符串 → 动态 import
const viewModules = import.meta.glob('../views/**/*.vue')

function loadView(component) {
  if (!component) return () => import('@/views/dashboard/Dashboard.vue')
  const normalized = component.replace(/^view\//, '').replace(/\.vue$/, '')
  const key = `../views/${normalized}.vue`
  if (viewModules[key]) return viewModules[key]
  // 取末段文件名（如 TaskList）做大小写不敏感模糊匹配
  const last = normalized.split('/').pop()
  if (last) {
    const lower = last.toLowerCase()
    const match = Object.keys(viewModules).find((k) => {
      const base = k.split('/').pop().replace(/\.vue$/, '')
      return base.toLowerCase() === lower
    })
    if (match) return viewModules[match]
  }
  // 兼容带前缀的路径（含大小写不敏感）
  const lc = normalized.toLowerCase()
  const broad = Object.keys(viewModules).find((k) => {
    const base = k.replace(/^\.\.\/views\//, '').replace(/\.vue$/, '').toLowerCase()
    return base === lc
  })
  if (broad) return viewModules[broad]
  // 兜底首页
  return () => import('@/views/dashboard/Dashboard.vue')
}

// 将后端菜单树转成 vue-router 子路由。
// 仅对带 component 的叶子菜单注册路由（目录/分组不注册路由，避免嵌套 path 拼接问题）；
// 侧边栏的子菜单展开结构由 MainLayout 依据菜单树渲染，路由本身是扁平的。
function menuToRoutes(menus) {
  const result = []
  const walk = (list) => {
    for (const m of list) {
      if (m.type === 'dir' && m.children?.length) {
        walk(m.children)
      } else if (m.component) {
        result.push({
          path: m.path.replace(/^\//, ''),
          name: m.name,
          meta: {
            title: m.title,
            icon: m.icon,
            hidden: m.hidden,
            btns: m.btns || []
          },
          component: loadView(m.component)
        })
      }
    }
  }
  walk(menus)
  return result
}

// 登录后调用：根据菜单树动态注册路由
export function setupDynamicRoutes(menus) {
  const dynamic = menuToRoutes(menus)
  for (const r of dynamic) {
    router.addRoute('Root', r)
  }
  // 兜底：确保已注册路由生效
  return router.getRoutes()
}

// 全局前置守卫
router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const userStore = useUserStore()
  if (!userStore.isLogin) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  // 首次进入：刷新用户信息（权限点/角色）+ 拉取动态菜单并注册路由
  if (!userStore._menusLoaded) {
    try {
      // 刷新当前用户信息，保证权限点/角色始终最新
      if (userStore.token) {
        const me = await getMe()
        if (me && me.username) userStore.setUser(me)
      }
      const menus = await getMyMenus()
      userStore.setMenus(menus)
      userStore._menusLoaded = true
      setupDynamicRoutes(menus)
      // 重新导航到目标（路由可能刚注册）
      return { ...to, replace: true }
    } catch (e) {
      userStore.logout()
      return { path: '/login' }
    }
  }
  // 权限码校验（菜单 permCode）
  if (to.meta.perm && !userStore.hasPerm(to.meta.perm)) {
    return { path: '/dashboard' }
  }
  return true
})

export default router
