import { defineStore } from 'pinia'

export const useUserStore = defineStore('user', {
  state: () => ({
    token: localStorage.getItem('scheduler_token') || '',
    user: JSON.parse(localStorage.getItem('scheduler_user') || 'null'),
    menus: JSON.parse(localStorage.getItem('scheduler_menus') || '[]')
  }),
  getters: {
    isLogin: (state) => !!state.token,
    isAdmin: (state) => state.user?.isAdmin === true,
    nickname: (state) => state.user?.nickname || state.user?.username || '未登录',
    roleName: (state) => state.user?.roleName || '',
    perms: (state) => state.user?.perms || [],
    // 权限点校验（兼容既有 v-perm / hasPerm 用法）
    hasPerm: (state) => (perm) => {
      if (state.user?.isAdmin) return true
      return (state.user?.perms || []).includes(perm)
    },
    // 按钮权限：通过菜单树中该菜单下的按钮名判断。
    // v-btn="['task:add']" 或 v-btn="'task:add'"。
    hasBtn: (state) => (permCode) => {
      if (state.user?.isAdmin) return true
      return (state.user?.perms || []).includes(permCode)
    },
    // 是否拥有某菜单（动态路由/侧边栏）
    hasMenu: (state) => (path) => {
      if (state.user?.isAdmin) return true
      return state.menus.some((m) => m.path === path)
    }
  },
  actions: {
    setLogin(token, user, menus) {
      this.token = token
      this.user = user
      if (menus) {
        this.menus = menus
        localStorage.setItem('scheduler_menus', JSON.stringify(menus))
      }
      localStorage.setItem('scheduler_token', token)
      localStorage.setItem('scheduler_user', JSON.stringify(user))
    },
    setMenus(menus) {
      this.menus = menus
      localStorage.setItem('scheduler_menus', JSON.stringify(menus))
    },
    setUser(user) {
      this.user = { ...this.user, ...user }
      localStorage.setItem('scheduler_user', JSON.stringify(this.user))
    },
    logout() {
      this.token = ''
      this.user = null
      this.menus = []
      localStorage.removeItem('scheduler_token')
      localStorage.removeItem('scheduler_user')
      localStorage.removeItem('scheduler_menus')
    }
  }
})
