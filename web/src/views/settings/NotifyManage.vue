<template>
  <div class="notify-page">
    <PageHero title="通知配置" desc="配置任务执行结果的钉钉 / Webhook 通知渠道" icon="Bell">
    </PageHero>

    <div class="app-card">
      <div class="card-header">
        <div class="card-title">
          <el-icon color="var(--primary-color)"><Bell /></el-icon>
          <span>全局通知渠道</span>
        </div>
        <div class="card-actions">
          <el-button :icon="Refresh" @click="loadSettings">刷新</el-button>
          <el-button type="primary" :icon="Promotion" :loading="testing" @click="sendTest">发送测试</el-button>
        </div>
      </div>

      <el-alert type="info" :closable="false" class="tip-alert"
        title="全局默认配置：任务未单独配置通知地址时，将使用这里的全局渠道发送。任务在「编辑任务」中可单独开启通知。" />

      <el-form :model="form" label-width="130px" class="notify-form">
        <el-divider content-position="left">基础</el-divider>
        <el-form-item label="启用全局通知">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="成功时也通知">
          <el-switch v-model="form.notifyOnSuccess" />
          <span class="form-tip">默认仅失败 / 超时通知</span>
        </el-form-item>

        <el-divider content-position="left">钉钉机器人</el-divider>
        <el-form-item label="Webhook 地址">
          <el-input v-model="form.dingtalkWebhook" placeholder="https://oapi.dingtalk.com/robot/send?access_token=xxx" clearable />
        </el-form-item>
        <el-form-item label="加签密钥">
          <el-input v-model="form.dingtalkSecret" placeholder="钉钉机器人的加签密钥（SEC 开头），留空则不签名" clearable />
          <span class="form-tip">在钉钉机器人安全设置中选择「加签」，将密钥填到这里</span>
        </el-form-item>

        <el-divider content-position="left">通用 Webhook</el-divider>
        <el-form-item label="Webhook 地址">
          <el-input v-model="form.webhookUrl" placeholder="https://example.com/hook/xxx" clearable />
        </el-form-item>
        <el-form-item label="签名密钥">
          <el-input v-model="form.webhookSecret" placeholder="可选，用于请求签名" clearable />
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :icon="Check" :loading="saving" @click="saveSettings">保存配置</el-button>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Bell, Refresh, Check, Promotion } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import PageHero from '@/components/PageHero.vue'
import { getNotifySettings, saveNotifySettings, sendNotifyTest } from '@/api/notify'

const saving = ref(false)
const testing = ref(false)
const form = reactive({
  enabled: false,
  notifyOnSuccess: false,
  dingtalkWebhook: '',
  dingtalkSecret: '',
  webhookUrl: '',
  webhookSecret: ''
})

async function loadSettings() {
  const data = await getNotifySettings()
  Object.assign(form, {
    enabled: !!data.enabled,
    notifyOnSuccess: !!data.notifyOnSuccess,
    dingtalkWebhook: data.dingtalkWebhook || '',
    dingtalkSecret: data.dingtalkSecret || '',
    webhookUrl: data.webhookUrl || '',
    webhookSecret: data.webhookSecret || ''
  })
}

async function saveSettings() {
  saving.value = true
  try {
    await saveNotifySettings({ ...form })
    ElMessage.success('通知配置已保存')
  } finally {
    saving.value = false
  }
}

async function sendTest() {
  testing.value = true
  try {
    await sendNotifyTest()
    ElMessage.success('测试通知已发送')
  } finally {
    testing.value = false
  }
}

onMounted(loadSettings)
</script>

<style scoped>
.notify-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}
.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}
.card-actions {
  display: flex;
  gap: 8px;
}
.tip-alert {
  margin-bottom: 8px;
}
.notify-form {
  max-width: 720px;
}
.form-tip {
  font-size: 12px;
  color: #909399;
  margin-left: 12px;
}
</style>
