<template>
  <el-dialog v-model="visible" :title="form.id ? '编辑任务' : '新增任务'" width="640px" top="6vh">
    <el-form :model="form" label-width="100px" size="default">
      <el-row :gutter="12">
        <el-col :span="14">
          <el-form-item label="任务名称" required>
            <el-input v-model="form.name" placeholder="请输入任务名称" />
          </el-form-item>
        </el-col>
        <el-col :span="10">
          <el-form-item label="任务类型">
            <el-select v-model="form.type" style="width: 100%" @change="onTypeChange">
              <el-option v-for="(l, v) in typeMap" :key="v" :label="l" :value="v" />
            </el-select>
          </el-form-item>
        </el-col>
      </el-row>

      <!-- Cron 类型 -->
      <template v-if="form.type === 'cron'">
        <el-form-item label="Cron 表达式" required>
          <el-input v-model="form.cronExpr" placeholder="如 0 2 * * *（分 时 日 月 周）" />
        </el-form-item>
      </template>

      <!-- HTTP 类型 -->
      <template v-if="form.type === 'http'">
        <el-form-item label="请求 Path" required>
          <el-input v-model="httpConfig.path" placeholder="/api/xxx（与绑定节点 address 拼接成完整 URL）" />
        </el-form-item>
        <el-form-item label="请求方法">
          <el-select v-model="httpConfig.method" style="width: 100%">
            <el-option label="GET" value="GET" />
            <el-option label="POST" value="POST" />
            <el-option label="PUT" value="PUT" />
            <el-option label="DELETE" value="DELETE" />
          </el-select>
        </el-form-item>
        <el-form-item label="请求体">
          <el-input v-model="httpConfig.body" type="textarea" :rows="2" placeholder='JSON，如 {"name":"demo"}' />
        </el-form-item>
        <el-form-item label="Headers">
          <el-input v-model="httpConfig.headersText" type="textarea" :rows="2" placeholder='一行一个，如 Content-Type: application/json' />
        </el-form-item>
        <el-divider content-position="left">认证</el-divider>
        <el-form-item label="认证方式">
          <el-select v-model="httpConfig.authType" style="width: 100%">
            <el-option label="无认证" value="none" />
            <el-option label="Basic" value="basic" />
            <el-option label="Bearer Token" value="bearer" />
          </el-select>
        </el-form-item>
        <template v-if="httpConfig.authType === 'basic'">
          <el-row :gutter="12">
            <el-col :span="12">
              <el-form-item label="用户名"><el-input v-model="httpConfig.username" /></el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="密码"><el-input v-model="httpConfig.password" type="password" show-password /></el-form-item>
            </el-col>
          </el-row>
        </template>
        <template v-if="httpConfig.authType === 'bearer'">
          <el-form-item label="Token"><el-input v-model="httpConfig.token" placeholder="Bearer 令牌" /></el-form-item>
        </template>
        <el-divider content-position="left">响应断言</el-divider>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="期望状态码">
              <el-input-number v-model="httpConfig.assertStatus" :min="0" :max="599" style="width: 100%" placeholder="0 不校验" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="响应体包含">
              <el-input v-model="httpConfig.assertBody" placeholder="响应体需包含的子串，留空不校验" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-divider content-position="left">变量提取</el-divider>
        <el-form-item label="提取变量">
          <el-input v-model="httpConfig.extractText" type="textarea" :rows="2" placeholder='每行一个：变量名 = 响应JSON路径，如 token = $.data.token' />
        </el-form-item>
      </template>

      <!-- Shell 类型 -->
      <template v-if="form.type === 'shell'">
        <el-form-item label="Shell 命令" required>
          <el-input v-model="shellConfig.command" type="textarea" :rows="3" placeholder="如 echo hello; date" />
        </el-form-item>
      </template>

      <!-- gRPC 类型 -->
      <template v-if="form.type === 'grpc'">
        <el-form-item label="服务名" required>
          <el-input v-model="grpcConfig.service" placeholder="如 hello.HelloService（服务端需开启反射）" />
        </el-form-item>
        <el-form-item label="方法名" required>
          <el-input v-model="grpcConfig.methodName" placeholder="如 SayHello" />
        </el-form-item>
        <el-form-item label="请求体(JSON)">
          <el-input v-model="grpcConfig.body" type="textarea" :rows="2" placeholder='JSON，如 {"name":"world"}' />
        </el-form-item>
        <el-form-item label="Metadata">
          <el-input v-model="grpcConfig.metadataText" type="textarea" :rows="2" placeholder='一行一个，如 authorization: Bearer xxx' />
        </el-form-item>
        <el-divider content-position="left">变量提取</el-divider>
        <el-form-item label="提取变量">
          <el-input v-model="grpcConfig.extractText" type="textarea" :rows="2" placeholder='每行一个：变量名 = 响应JSON路径，如 name = $.message' />
        </el-form-item>
      </template>

      <el-divider content-position="left">调度配置</el-divider>
      <el-form-item v-if="form.type !== 'cron'" label="Cron 表达式">
        <el-input v-model="form.cronExpr" placeholder="留空则不自动调度，如 0 */5 * * *（每5分钟）" />
        <div class="form-tip">填写后将按 Cron 定时执行，留空则仅支持手动触发</div>
      </el-form-item>
      <el-form-item v-if="form.type !== 'cron'" label="绑定节点">
        <el-select
          v-model="form.nodeId"
          :disabled="nodeLocked"
          clearable
          style="width: 100%"
          :placeholder="nodeLocked ? '固定绑定当前节点' : '选择执行的目标机器（单选）'"
        >
          <el-option v-for="n in nodeOptions" :key="n.id" :label="`${n.name} (${n.address})`" :value="n.id">
            <div class="node-opt">
              <span>{{ n.name }}</span>
              <span class="node-opt-addr">{{ n.address }}</span>
            </div>
          </el-option>
        </el-select>
        <div class="selected-nodes" v-if="selectedNodes.length">
          <el-tag v-for="n in selectedNodes" :key="n.id" size="small" type="success" effect="plain" class="node-tag">
            {{ n.name }}
          </el-tag>
        </div>
        <div class="form-tip">{{ nodeLocked ? '该任务固定绑定当前节点，不可修改' : '任务将在绑定的目标节点上执行，需绑定一个节点' }}</div>
      </el-form-item>
      <el-form-item label="任务分组">
        <el-input v-model="form.group" placeholder="如 default / ops / data" />
      </el-form-item>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="超时(秒)">
            <el-input-number v-model="form.timeout" :min="1" :max="3600" style="width: 100%" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="并发数">
            <el-input-number v-model="form.concurrency" :min="1" :max="100" style="width: 100%" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="重试次数">
            <el-input-number v-model="form.retryCount" :min="0" :max="10" style="width: 100%" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="重试间隔(s)">
            <el-input-number v-model="form.retryInterval" :min="0" :max="600" style="width: 100%" />
          </el-form-item>
        </el-col>
      </el-row>

      <!-- 通知配置 -->
      <el-divider content-position="left">通知配置</el-divider>
      <el-form-item label="启用通知">
        <el-switch v-model="form.notifyEnabled" />
        <span class="form-tip">任务执行失败 / 超时时发送通知</span>
      </el-form-item>
      <template v-if="form.notifyEnabled">
        <el-form-item label="通知渠道">
          <el-checkbox-group v-model="form.notifyTypeArr">
            <el-checkbox value="dingtalk">钉钉机器人</el-checkbox>
            <el-checkbox value="webhook">Webhook</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <template v-if="form.notifyTypeArr.includes('dingtalk')">
          <el-form-item label="钉钉 Webhook">
            <el-input v-model="form.notifyDingtalk" placeholder="留空则使用全局配置" clearable />
          </el-form-item>
          <el-form-item label="钉钉密钥">
            <el-input v-model="form.notifySecret" placeholder="加签密钥（可选）" clearable />
          </el-form-item>
        </template>
        <template v-if="form.notifyTypeArr.includes('webhook')">
          <el-form-item label="Webhook 地址">
            <el-input v-model="form.notifyWebhook" placeholder="留空则使用全局配置" clearable />
          </el-form-item>
          <el-form-item label="Webhook 密钥">
            <el-input v-model="form.notifyWebhookSec" placeholder="签名密钥（可选）" clearable />
          </el-form-item>
        </template>
        <el-form-item label="成功时通知">
          <el-switch v-model="form.notifyOnSuccess" />
          <span class="form-tip">开启后成功也会通知</span>
        </el-form-item>
      </template>

      <el-form-item label="描述">
        <el-input v-model="form.description" type="textarea" :rows="2" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { createTask, updateTask } from '@/api/task'
import { listNodes } from '@/api/node'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  // 编辑时传入的任务对象；为 null 表示新建
  task: { type: Object, default: null },
  // 锁定绑定节点：传入节点 ID 后，绑定节点固定为该节点且不可修改
  lockNodeId: { type: Number, default: null },
  // 新建时的默认任务类型（Cron 类型不绑定节点，故默认用 http 以便选择节点）
  defaultType: { type: String, default: 'cron' }
})
const emit = defineEmits(['update:modelValue', 'saved'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const nodeOptions = ref([])
const saving = ref(false)
const typeMap = { cron: 'Cron', http: 'HTTP', shell: 'Shell', grpc: 'gRPC' }
const nodeLocked = computed(() => props.lockNodeId != null)

const form = reactive({
  id: null, name: '', type: 'cron', cronExpr: '', group: '',
  timeout: 60, concurrency: 1, retryCount: 0, retryInterval: 60,
  nodeId: null, description: '',
  notifyEnabled: false,
  notifyTypeArr: [],
  notifyOnSuccess: false,
  notifyDingtalk: '',
  notifySecret: '',
  notifyWebhook: '',
  notifyWebhookSec: ''
})

const httpConfig = reactive({
  path: '', method: 'GET', body: '', headersText: '',
  authType: 'none', username: '', password: '', token: '',
  assertStatus: 0, assertBody: '', extractText: ''
})
const shellConfig = reactive({ command: '' })
const grpcConfig = reactive({
  service: '', methodName: '', body: '', metadataText: '', extractText: ''
})

// 单节点：锁定模式下为锁定节点，否则为表单选中节点
const effectiveNodeId = computed(() => (nodeLocked.value ? props.lockNodeId : form.nodeId))

const selectedNodes = computed(() => {
  if (!effectiveNodeId.value) return []
  return nodeOptions.value.filter((n) => n.id === effectiveNodeId.value)
})

watch(
  () => props.modelValue,
  (v) => { if (v) initForm() }
)

async function initForm() {
  resetForm()
  if (!nodeOptions.value.length) nodeOptions.value = await listNodes()
  if (props.task) fillFromRow(props.task)
  if (props.lockNodeId != null) form.nodeId = props.lockNodeId
}

function resetForm() {
  Object.assign(form, {
    id: null, name: '', type: props.defaultType, cronExpr: '', group: '',
    timeout: 60, concurrency: 1, retryCount: 0, retryInterval: 60,
    nodeId: null, description: '',
    notifyEnabled: false,
    notifyTypeArr: [],
    notifyOnSuccess: false,
    notifyDingtalk: '',
    notifySecret: '',
    notifyWebhook: '',
    notifyWebhookSec: ''
  })
  Object.assign(httpConfig, {
    path: '', method: 'GET', body: '', headersText: '',
    authType: 'none', username: '', password: '', token: '',
    assertStatus: 0, assertBody: '', extractText: ''
  })
  Object.assign(shellConfig, { command: '' })
  Object.assign(grpcConfig, {
    service: '', methodName: '', body: '', metadataText: '', extractText: ''
  })
}

function onTypeChange() {
  // 切换类型时保留 cronExpr（所有类型都可配置定时调度）
  if (nodeLocked.value && form.type !== 'cron') form.nodeId = props.lockNodeId
}

function fillFromRow(row) {
  Object.assign(form, {
    id: row.id, name: row.name, type: row.type, cronExpr: row.cronExpr || '',
    group: row.group,
    timeout: row.timeout || 60, concurrency: row.concurrency || 1,
    retryCount: row.retryCount || 0, retryInterval: row.retryInterval || 60,
    nodeId: row.nodeIds && row.nodeIds.length ? row.nodeIds[0] : null, description: row.description,
    notifyEnabled: !!row.notifyEnabled,
    notifyTypeArr: row.notifyType ? row.notifyType.split(',').filter(Boolean) : [],
    notifyOnSuccess: !!row.notifyOnSuccess,
    notifyDingtalk: row.notifyDingtalk || '',
    notifySecret: row.notifySecret || '',
    notifyWebhook: row.notifyWebhook || '',
    notifyWebhookSec: row.notifyWebhookSec || ''
  })
  let cfg = {}
  if (row.executorConfig) {
    try { cfg = JSON.parse(row.executorConfig) } catch (e) { cfg = {} }
  }
  if (row.type === 'http') {
    httpConfig.path = cfg.path || cfg.url || ''
    httpConfig.method = cfg.method || 'GET'
    httpConfig.body = cfg.body || ''
    httpConfig.headersText = Object.entries(cfg.headers || {})
      .map(([k, v]) => `${k}: ${v}`).join('\n')
    httpConfig.authType = cfg.authType || 'none'
    httpConfig.username = cfg.username || ''
    httpConfig.password = cfg.password || ''
    httpConfig.token = cfg.token || ''
    httpConfig.assertStatus = cfg.assertStatus || 0
    httpConfig.assertBody = cfg.assertBody || ''
    httpConfig.extractText = Object.entries(cfg.extract || {})
      .map(([k, v]) => `${k} = ${v}`).join('\n')
  } else if (row.type === 'shell') {
    shellConfig.command = cfg.command || ''
  } else if (row.type === 'grpc') {
    grpcConfig.service = cfg.service || ''
    grpcConfig.methodName = cfg.methodName || ''
    grpcConfig.body = cfg.body || ''
    grpcConfig.metadataText = Object.entries(cfg.headers || {})
      .map(([k, v]) => `${k}: ${v}`).join('\n')
    grpcConfig.extractText = Object.entries(cfg.extract || {})
      .map(([k, v]) => `${k} = ${v}`).join('\n')
  }
}

function buildExecutorConfig() {
  let cfg = {}
  if (form.type === 'http') {
    cfg = {
      path: httpConfig.path,
      method: httpConfig.method,
      body: httpConfig.body,
      headers: parseHeaders(httpConfig.headersText),
      authType: httpConfig.authType,
      username: httpConfig.username,
      password: httpConfig.password,
      token: httpConfig.token,
      assertStatus: httpConfig.assertStatus || 0,
      assertBody: httpConfig.assertBody,
      extract: parseExtract(httpConfig.extractText)
    }
  } else if (form.type === 'shell') {
    cfg = { command: shellConfig.command }
  } else if (form.type === 'grpc') {
    cfg = {
      service: grpcConfig.service,
      methodName: grpcConfig.methodName,
      body: grpcConfig.body,
      headers: parseHeaders(grpcConfig.metadataText),
      extract: parseExtract(grpcConfig.extractText)
    }
  }
  return Object.keys(cfg).length ? JSON.stringify(cfg) : ''
}

function parseHeaders(text) {
  const headers = {}
  for (const line of (text || '').split('\n')) {
    const idx = line.indexOf(':')
    if (idx > 0) headers[line.slice(0, idx).trim()] = line.slice(idx + 1).trim()
  }
  return headers
}

function parseExtract(text) {
  const extract = {}
  for (const line of (text || '').split('\n')) {
    const idx = line.indexOf('=')
    if (idx > 0) {
      const name = line.slice(0, idx).trim()
      const path = line.slice(idx + 1).trim()
      if (name && path) extract[name] = path
    }
  }
  return Object.keys(extract).length ? extract : undefined
}

function validate() {
  if (!form.name) return '请输入任务名称'
  if (form.cronExpr) {
    const cronReg = /^(\*|([0-9,-\/]+))\s+(\*|([0-9,-\/]+))\s+(\*|([0-9,-\/]+))\s+(\*|([0-9,-\/]+))\s+(\*|([0-9,-\/]+))$/
    if (!cronReg.test(form.cronExpr.trim())) return 'Cron 表达式格式不正确（5 字段：分 时 日 月 周）'
  }
  if (form.type === 'http' && !httpConfig.path) return '请输入请求 Path'
  if (form.type === 'shell' && !shellConfig.command) return '请输入 Shell 命令'
  if (form.type === 'grpc') {
    if (!grpcConfig.service) return '请输入 gRPC 服务名（service）'
    if (!grpcConfig.methodName) return '请输入 gRPC 方法名（methodName）'
  }
  // Cron 类型不绑定节点；其余类型必须绑定一个节点
  if (form.type !== 'cron' && !effectiveNodeId.value) return '请选择绑定的目标节点'
  return ''
}

async function save() {
  const err = validate()
  if (err) return ElMessage.warning(err)
  const nodeIds = form.type === 'cron' ? [] : (effectiveNodeId.value ? [effectiveNodeId.value] : [])
  const payload = {
    name: form.name, type: form.type, cronExpr: form.cronExpr || '',
    group: form.group,
    timeout: form.timeout, concurrency: form.concurrency,
    retryCount: form.retryCount, retryInterval: form.retryInterval,
    nodeIds, description: form.description,
    executorConfig: buildExecutorConfig(),
    notifyEnabled: form.notifyEnabled,
    notifyType: form.notifyTypeArr.join(','),
    notifyOnSuccess: form.notifyOnSuccess,
    notifyDingtalk: form.notifyDingtalk,
    notifySecret: form.notifySecret,
    notifyWebhook: form.notifyWebhook,
    notifyWebhookSec: form.notifyWebhookSec
  }
  saving.value = true
  try {
    if (form.id) {
      await updateTask(form.id, payload)
      ElMessage.success('任务已更新')
    } else {
      await createTask(payload)
      ElMessage.success('任务已创建')
    }
    visible.value = false
    emit('saved')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  nodeOptions.value = await listNodes()
})
</script>

<style scoped>
.node-opt {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}
.node-opt-addr {
  color: #909399;
  font-size: 12px;
}
.selected-nodes {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 6px;
}
.node-tag {
  margin-right: 4px;
}
.form-tip {
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
  margin-top: 2px;
}
</style>
