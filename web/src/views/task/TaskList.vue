<template>
  <div class="task-page">
    <!-- 页头 -->
    <PageHero title="任务管理" desc="配置与管理 Cron / HTTP / Shell / gRPC 任务" icon="List">
      <template #actions>
        <el-button v-perm="'task:create'" type="primary" :icon="Plus" @click="openCreate" class="hero-btn">
          新增任务
        </el-button>
      </template>
    </PageHero>

    <!-- 搜索 + 操作 -->
    <div class="app-card toolbar">
      <el-form :inline="true" class="search-form" @submit.prevent>
        <el-form-item label="关键字">
          <el-input v-model="search.keyword" placeholder="任务名称/编码" clearable style="width: 180px" @keyup.enter="loadTasks" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="search.type" placeholder="全部" clearable style="width: 120px">
            <el-option v-for="(l, v) in typeMap" :key="v" :label="l" :value="v" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="search.status" placeholder="全部" clearable style="width: 120px">
            <el-option label="启用" :value="1" />
            <el-option label="停用" :value="0" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :icon="Search" @click="loadTasks(1)">搜索</el-button>
          <el-button :icon="Refresh" @click="resetSearch">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <!-- 表格 -->
    <div class="app-card">
      <el-table :data="tasks" v-loading="loading" style="width: 100%" empty-text="暂无任务">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="任务名称" min-width="130" />
        <el-table-column prop="code" label="编码" width="105" />
        <el-table-column label="类型" width="85">
          <template #default="{ row }">
            <el-tag size="small" type="info" effect="plain">{{ typeMap[row.type] }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="group" label="分组" width="95" />
        <el-table-column prop="cronExpr" label="Cron" width="115">
          <template #default="{ row }">{{ row.cronExpr || '-' }}</template>
        </el-table-column>
        <el-table-column label="绑定节点" width="150">
          <template #default="{ row }">
            <template v-if="row.type !== 'cron' && row.nodes && row.nodes.length">
              <el-tooltip placement="top">
                <template #content>
                  <div class="node-tip">
                    <div v-for="n in row.nodes" :key="n.id" class="node-tip-item">
                      {{ n.name }}<span class="node-tip-addr">{{ n.address }}</span>
                    </div>
                  </div>
                </template>
                <div class="node-tags">
                  <el-tag v-for="n in row.nodes.slice(0, 2)" :key="n.id" size="small" type="success" effect="plain" class="node-tag">
                    {{ n.name }}
                  </el-tag>
                  <span v-if="row.nodes.length > 2" class="node-more">+{{ row.nodes.length - 2 }}</span>
                </div>
              </el-tooltip>
            </template>
            <span v-else-if="row.type !== 'cron'" class="text-muted">未绑定</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="下次执行" width="160">
          <template #default="{ row }">
            <span>{{ formatTime(row.nextRunTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-switch
              :model-value="row.status === 1"
              :disabled="!userStore.hasPerm('task:run')"
              active-color="#13ce66"
              inactive-color="#ff4949"
              @change="(v) => changeStatus(row, v)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button v-if="userStore.hasPerm('task:update')" size="small" type="primary" link @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="info" link @click="openHistory(row)">历史</el-button>
            <el-button v-if="userStore.hasPerm('task:run')" size="small" type="success" link @click="runTask(row)">执行</el-button>
            <el-button v-if="userStore.hasPerm('task:delete')" size="small" type="danger" link @click="removeTask(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          background
          @size-change="loadTasks(1)"
          @current-change="loadTasks()"
        />
      </div>
    </div>

    <!-- 任务历史抽屉 -->
    <el-drawer v-model="historyVisible" :title="`执行历史 · ${historyTask?.name}`" size="55%">
      <template v-if="historyTask">
        <div class="hist-stats" v-if="taskStats">
          <div class="hist-stat">
            <div class="num">{{ taskStats.total }}</div>
            <div class="label">总执行</div>
          </div>
          <div class="hist-stat success">
            <div class="num">{{ taskStats.success }}</div>
            <div class="label">成功</div>
          </div>
          <div class="hist-stat failed">
            <div class="num">{{ taskStats.failed }}</div>
            <div class="label">失败</div>
          </div>
          <div class="hist-stat timeout">
            <div class="num">{{ taskStats.timeout }}</div>
            <div class="label">超时</div>
          </div>
          <div class="hist-stat">
            <div class="num">{{ taskStats.successRate ? taskStats.successRate.toFixed(1) + '%' : '-' }}</div>
            <div class="label">成功率</div>
          </div>
          <div class="hist-stat">
            <div class="num">{{ taskStats.avgDurationMs ? (taskStats.avgDurationMs / 1000).toFixed(2) + 's' : '-' }}</div>
            <div class="label">平均耗时</div>
          </div>
        </div>
        <el-table :data="taskHistory" v-loading="historyLoading" empty-text="暂无执行记录" size="small">
          <el-table-column prop="id" label="执行ID" width="90" />
          <el-table-column label="状态" width="100">
            <template #default="{ row }"><StatusTag :status="row.status" /></template>
          </el-table-column>
          <el-table-column label="触发" width="80">
            <template #default="{ row }">{{ triggerMap[row.triggerType] || row.triggerType }}</template>
          </el-table-column>
          <el-table-column label="开始时间" width="160">
            <template #default="{ row }">{{ formatTime(row.startedAt) }}</template>
          </el-table-column>
          <el-table-column label="耗时" width="100">
            <template #default="{ row }">{{ durationLabel(row.durationMs) }}</template>
          </el-table-column>
          <el-table-column prop="workerName" label="节点" width="100" />
          <el-table-column label="错误" min-width="140" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="error-text">{{ row.errorMessage || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="70" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="primary" link @click="viewLog(row.id)">日志</el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </el-drawer>

    <!-- 日志抽屉 -->
    <el-drawer v-model="logDrawerVisible" :title="`执行日志 #${logExecId}`" size="50%" class="log-drawer">
      <div class="log-viewer">
        <div v-for="log in logLines" :key="log.id" class="log-line" :class="log.level">
          <span class="log-time">{{ log.time }}</span>
          <span class="log-level" :class="log.level">{{ log.level }}</span>
          <span class="log-content">{{ log.content }}</span>
        </div>
        <el-empty v-if="!logLines.length" description="暂无日志" :image-size="60" />
      </div>
    </el-drawer>

    <!-- 新增/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑任务' : '新增任务'" width="640px" top="6vh">
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
          <el-select v-model="form.nodeIds" multiple clearable style="width: 100%" placeholder="选择执行的目标机器（可多选）">
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
          <div class="form-tip">任务将向每个绑定的目标节点发起执行，需至少绑定一个节点</div>
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
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveTask">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { Search, Refresh, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listTasks, createTask, updateTask, deleteTask, runTask as apiRunTask, enableTask, disableTask } from '@/api/task'
import { getTaskStats, getTaskHistory, getLogs } from '@/api/execution'
import { listNodes } from '@/api/node'
import { useUserStore } from '@/stores/user'
import StatusTag from '@/components/StatusTag.vue'
import PageHero from '@/components/PageHero.vue'

const userStore = useUserStore()
const loading = ref(false)
const tasks = ref([])
const nodeOptions = ref([])
const dialogVisible = ref(false)
const typeMap = { cron: 'Cron', http: 'HTTP', shell: 'Shell', grpc: 'gRPC' }
const search = reactive({ keyword: '', type: '', status: '' })
const query = reactive({ page: 1, pageSize: 20 })
const total = ref(0)

const httpConfig = reactive({
  path: '', method: 'GET', body: '', headersText: '',
  authType: 'none', username: '', password: '', token: '',
  assertStatus: 0, assertBody: '', extractText: ''
})
const shellConfig = reactive({ command: '' })
const grpcConfig = reactive({
  service: '', methodName: '', body: '', metadataText: '', extractText: ''
})

const form = reactive({
  id: null, name: '', type: 'cron', cronExpr: '', group: '',
  timeout: 60, concurrency: 1, retryCount: 0, retryInterval: 60,
  nodeIds: [], description: '',
  // 通知配置
  notifyEnabled: false,
  notifyTypeArr: [], // 前端渠道数组（dingtalk/webhook）
  notifyOnSuccess: false,
  notifyDingtalk: '',
  notifySecret: '',
  notifyWebhook: '',
  notifyWebhookSec: ''
})

// 历史抽屉
const historyVisible = ref(false)
const historyTask = ref(null)
const taskStats = ref(null)
const taskHistory = ref([])
const historyLoading = ref(false)
const triggerMap = { cron: '调度', manual: '手动', retry: '重试' }

function formatTime(t) {
  if (!t) return '-'
  const s = t.replace('T', ' ').slice(0, 19)
  // Go time.Time 零值 / 空值判断
  if (s.startsWith('0001-01-01') || s.startsWith('0000-00-00')) return '-'
  return s
}
function durationLabel(ms) {
  if (ms == null) return '-'
  if (ms < 1000) return ms + 'ms'
  return (ms / 1000).toFixed(2) + 's'
}

async function openHistory(row) {
  historyTask.value = row
  historyVisible.value = true
  historyLoading.value = true
  try {
    const [stats, history] = await Promise.all([
      getTaskStats(row.id),
      getTaskHistory(row.id, 30)
    ])
    taskStats.value = stats
    taskHistory.value = history
  } finally {
    historyLoading.value = false
  }
}

// 日志抽屉
const logDrawerVisible = ref(false)
const logExecId = ref(0)
const logLines = ref([])

async function viewLog(execId) {
  logExecId.value = execId
  logDrawerVisible.value = true
  logLines.value = []
  const logs = await getLogs(execId)
  logLines.value = (logs || []).map((l) => ({
    id: l.id,
    time: formatTime(l.createdAt),
    level: l.level || 'info',
    content: l.content
  }))
}

function resetForm() {
  Object.assign(form, {
    id: null, name: '', type: 'cron', cronExpr: '', group: '',
    timeout: 60, concurrency: 1, retryCount: 0, retryInterval: 60,
    nodeIds: [], description: '',
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
}

// 解析行数据到表单
function fillFromRow(row) {
  resetForm()
  Object.assign(form, {
    id: row.id, name: row.name, type: row.type, cronExpr: row.cronExpr || '',
    group: row.group,
    timeout: row.timeout || 60, concurrency: row.concurrency || 1,
    retryCount: row.retryCount || 0, retryInterval: row.retryInterval || 60,
    nodeIds: row.nodeIds || [], description: row.description,
    notifyEnabled: !!row.notifyEnabled,
    notifyTypeArr: row.notifyType ? row.notifyType.split(',').filter(Boolean) : [],
    notifyOnSuccess: !!row.notifyOnSuccess,
    notifyDingtalk: row.notifyDingtalk || '',
    notifySecret: row.notifySecret || '',
    notifyWebhook: row.notifyWebhook || '',
    notifyWebhookSec: row.notifyWebhookSec || ''
  })
  // 解析 executorConfig
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

// parseExtract 解析变量提取文本：每行 `变量名 = 路径`，路径如 $.data.token
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
  // 有 cronExpr 时校验格式（所有类型都可以配置定时）
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
  // HTTP/gRPC/Shell 需绑定目标节点（Cron 类型不绑定节点）
  if (form.type !== 'cron' && (!form.nodeIds || !form.nodeIds.length)) return '请选择绑定的目标节点'
  return ''
}

async function loadTasks(page) {
  if (page) query.page = page
  loading.value = true
  try {
    const data = await listTasks({
      keyword: search.keyword,
      type: search.type,
      status: search.status,
      page: query.page,
      pageSize: query.pageSize
    })
    tasks.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

async function loadNodes() {
  nodeOptions.value = await listNodes()
}

// 已选中的绑定节点详情（用于表单下方回显）
const selectedNodes = computed(() => {
  if (!form.nodeIds?.length) return []
  return nodeOptions.value.filter((n) => form.nodeIds.includes(n.id))
})

function resetSearch() {
  search.keyword = ''
  search.type = ''
  search.status = ''
  loadTasks()
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
  fillFromRow(row)
  dialogVisible.value = true
}

async function saveTask() {
  const err = validate()
  if (err) return ElMessage.warning(err)
  // Cron 定时任务不绑定节点；HTTP/gRPC/Shell 任务在绑定节点（目标机器）上执行
  const nodeIds = form.type === 'cron' ? [] : form.nodeIds
  const payload = {
    name: form.name, type: form.type, cronExpr: form.cronExpr || '',
    group: form.group,
    timeout: form.timeout, concurrency: form.concurrency,
    retryCount: form.retryCount, retryInterval: form.retryInterval,
    nodeIds, description: form.description,
    executorConfig: buildExecutorConfig(),
    // 通知配置
    notifyEnabled: form.notifyEnabled,
    notifyType: form.notifyTypeArr.join(','),
    notifyOnSuccess: form.notifyOnSuccess,
    notifyDingtalk: form.notifyDingtalk,
    notifySecret: form.notifySecret,
    notifyWebhook: form.notifyWebhook,
    notifyWebhookSec: form.notifyWebhookSec
  }
  if (form.id) {
    await updateTask(form.id, payload)
    ElMessage.success('任务已更新')
  } else {
    await createTask(payload)
    ElMessage.success('任务已创建')
  }
  dialogVisible.value = false
  loadTasks()
}

async function runTask(row) {
  await ElMessageBox.confirm(`确认立即执行任务「${row.name}」？`, '提示', { type: 'warning' })
  await apiRunTask(row.id)
  ElMessage.success('任务已触发执行')
}

async function removeTask(row) {
  await ElMessageBox.confirm(`确认删除任务「${row.name}」？此操作不可恢复。`, '删除确认', { type: 'error' })
  await deleteTask(row.id)
  ElMessage.success('任务已删除')
  loadTasks()
}

async function changeStatus(row, v) {
  if (v) {
    await enableTask(row.id)
  } else {
    await disableTask(row.id)
  }
  ElMessage.success('状态已更新')
  loadTasks()
}

onMounted(async () => {
  await loadNodes()
  loadTasks()
})
</script>

<style scoped>
.task-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.hero-btn {
  background: rgba(255, 255, 255, 0.95) !important;
  color: var(--primary-color) !important;
  border-color: transparent !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15) !important;
}
.hero-btn:hover {
  opacity: 0.9;
}
.toolbar {
  padding: 16px 20px 0;
}
.search-form {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
}
.right-actions {
  margin-left: auto;
}
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding: 0 20px 16px;
}
.node-tag {
  margin-right: 4px;
}
.node-tags {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 2px;
}
.node-more {
  font-size: 12px;
  color: #909399;
  margin-left: 2px;
}
.node-tip {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.node-tip-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.node-tip-addr {
  color: #909399;
}
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
.form-tip {
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
  margin-top: 2px;
}
.text-muted {
  color: #c0c4cc;
}
.hist-stats {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.hist-stat {
  flex: 1;
  min-width: 100px;
  background: #f9fafb;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  padding: 12px 16px;
  text-align: center;
}
.hist-stat .num {
  font-size: 20px;
  font-weight: 600;
  color: #303133;
}
.hist-stat .label {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}
.hist-stat.success .num { color: #13ce66; }
.hist-stat.failed .num { color: #f56c6c; }
.hist-stat.timeout .num { color: #e6a23c; }
.error-text {
  color: #f56c6c;
}
</style>

<style>
/* 日志抽屉整体黑色主题 */
.log-drawer .el-drawer__header {
  background: #1e1e1e;
  color: #e0e0e0;
  border-bottom: 1px solid #2d2d2d;
  margin-bottom: 0;
  padding: 16px 20px;
}
.log-drawer .el-drawer__title {
  color: #e0e0e0 !important;
  font-family: 'Courier New', monospace;
  font-weight: 500;
}
.log-drawer .el-drawer__close-btn {
  color: #888 !important;
}
.log-drawer .el-drawer__close-btn:hover {
  color: #fff !important;
}
.log-drawer .el-drawer__body {
  background: #1e1e1e;
  padding: 0;
}
/* 日志查看器（VS Code 风格） */
.log-viewer {
  background: #1e1e1e;
  padding: 12px 16px;
  height: calc(100vh - 60px);
  overflow-y: auto;
  font-family: 'JetBrains Mono', 'Fira Code', 'Courier New', monospace;
  font-size: 12.5px;
  line-height: 1.6;
}
.log-line {
  display: grid;
  grid-template-columns: 150px 55px 1fr;
  gap: 10px;
  padding: 1px 0;
  color: #d4d4d4;
  border-bottom: 1px solid rgba(255,255,255,0.03);
}
.log-line:hover {
  background: rgba(255,255,255,0.04);
}
.log-time {
  color: #858585;
  font-size: 11.5px;
}
.log-level {
  font-weight: 600;
  text-transform: uppercase;
  font-size: 11px;
}
.log-level.info {
  color: #4fc1ff;
}
.log-level.error {
  color: #f48771;
}
.log-level.warn {
  color: #ce9178;
}
.log-content {
  word-break: break-all;
  white-space: pre-wrap;
}
/* 滚动条样式 */
.log-viewer::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
.log-viewer::-webkit-scrollbar-track {
  background: #1e1e1e;
}
.log-viewer::-webkit-scrollbar-thumb {
  background: #3e3e3e;
  border-radius: 4px;
}
.log-viewer::-webkit-scrollbar-thumb:hover {
  background: #555;
}
</style>
