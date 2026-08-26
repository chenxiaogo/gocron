<template>
  <div class="login-page">
    <div class="login-bg">
      <div class="bg-grid"></div>
      <div class="bg-orbs">
        <div class="orb orb-1"></div>
        <div class="orb orb-2"></div>
        <div class="orb orb-3"></div>
      </div>
    </div>
    <div class="login-card fade-in-up">
      <div class="login-logo">
        <div class="logo-badge">
          <el-icon :size="30" color="#fff"><SetUp /></el-icon>
        </div>
      </div>
      <div class="login-header">
        <h2>调度平台</h2>
        <p>企业级任务调度与执行平台</p>
        <p class="sub">统一任务配置 · 调度 · 执行 · 监控</p>
      </div>
      <el-form ref="formRef" :model="form" :rules="rules" size="large" @keyup.enter="submit">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" :prefix-icon="User" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" placeholder="密码" :prefix-icon="Lock" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" class="login-btn" @click="submit">
            登 录
          </el-button>
        </el-form-item>
      </el-form>
      <div class="login-tip">默认账号：admin / admin123</div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { login } from '@/api/auth'
import { getMyMenus } from '@/api/menu'
import { setupDynamicRoutes } from '@/router'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const formRef = ref()
const loading = ref(false)
const form = reactive({ username: '', password: '' })
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

async function submit() {
  await formRef.value.validate()
  loading.value = true
  try {
    const res = await login(form)
    if (res.code === 0) {
      userStore.setLogin(res.token, res.user)
      // 拉取动态菜单并注册路由
      const menus = await getMyMenus()
      userStore.setMenus(menus)
      userStore._menusLoaded = true
      setupDynamicRoutes(menus)
      ElMessage.success('登录成功')
      router.push(route.query.redirect || '/dashboard')
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  height: 100vh;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  overflow: hidden;
}
.login-bg {
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, #1c2333 0%, #2a2f5a 45%, #4f6ef7 100%);
}
.bg-grid {
  position: absolute;
  inset: 0;
  background-image: linear-gradient(rgba(255, 255, 255, 0.05) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.05) 1px, transparent 1px);
  background-size: 40px 40px;
}
.bg-orbs {
  position: absolute;
  inset: 0;
}
.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(60px);
  opacity: 0.5;
}
.orb-1 {
  width: 300px;
  height: 300px;
  background: #7c5cf0;
  top: -80px;
  right: 10%;
}
.orb-2 {
  width: 260px;
  height: 260px;
  background: #4f6ef7;
  bottom: -60px;
  left: 8%;
}
.orb-3 {
  width: 200px;
  height: 200px;
  background: rgba(255, 255, 255, 0.25);
  top: 40%;
  left: 55%;
}
.login-card {
  position: relative;
  z-index: 2;
  width: 420px;
  padding: 36px 40px 30px;
  background: rgba(255, 255, 255, 0.95);
  border-radius: 20px;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.35);
  backdrop-filter: blur(12px);
}
.login-logo {
  display: flex;
  justify-content: center;
  margin-bottom: 20px;
}
.logo-badge {
  width: 64px;
  height: 64px;
  border-radius: 18px;
  background: var(--primary-gradient);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 8px 24px rgba(79, 110, 247, 0.4);
}
.login-header {
  text-align: center;
  margin-bottom: 28px;
}
.login-header h2 {
  font-size: 24px;
  font-weight: 700;
  color: #1e293b;
  margin-bottom: 6px;
}
.login-header p {
  font-size: 13px;
  color: #64748b;
}
.login-header .sub {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 6px;
}
.login-btn {
  width: 100%;
  height: 44px;
  font-size: 16px;
  font-weight: 600;
}
.login-tip {
  text-align: center;
  font-size: 12px;
  color: #cbd5e1;
  margin-top: 14px;
}
</style>
