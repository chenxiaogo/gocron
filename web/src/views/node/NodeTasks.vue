<template>
  <div class="node-tasks-page">
    <div class="app-card">
      <div class="card-header">
        <div class="card-title">
          <el-icon color="var(--primary-color)"><Monitor /></el-icon>
          <span>节点任务</span>
          <span v-if="node" class="node-info">{{ node.name }} · {{ node.address }}</span>
        </div>
        <div class="card-actions">
          <el-button :icon="ArrowLeft" @click="$router.push('/nodes')">返回</el-button>
          <el-button :icon="Refresh" @click="loadAll" :loading="loading">刷新</el-button>
        </div>
      </div>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="任务列表" name="tasks">
          <div class="tab-toolbar">
            <el-button type="primary" :icon="Plus" v-perm="'task:create'" @click="openCreateTask">新建任务</el-button>
          </div>
          <el-table :data="tasks" v-loading="loading" empty-text="该节点暂无绑定任务" @row-click="openTaskDetail">
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column prop="name" label="任务名称" min-width="140" />
            <el-table-column label="类型" width="90">
              <template #default="{ row }">
                <el-tag size="small" type="info" effect="plain">{{ typeMap[row.type] || row.type }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="Cron" width="120">
              <template #default="{ row }">{{ row.cronExpr || '-' }}</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="plain">
                  {{ row.status === 1 ? '启用' : '停用' }}
                </el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="执行记录" name="executions">
          <el-table :data="executions" v-loading="loading" empty-text="该节点暂无执行记录">
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
            <el-table-column prop="workerName" label="节点" width="110" />
            <el-table-column label="错误" min-width="140" show-overflow-tooltip>
              <template #default="{ row }"><span class="error-text">{{ row.errorMessage || '-' }}</span></template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </div>

    <!-- 任务详情：任务信息 / 执行记录 两个 Tab -->
    <el-drawer v-model="detailVisible" :title="`任务详情 · ${detail?.name || ''}`" size="52%">
      <template v-if="detail">
        <el-tabs v-model="detailTab">
          <el-tab-pane label="任务信息" name="info">
            <el-descriptions :column="1" border>
              <el-descriptions-item label="ID">{{ detail.id }}</el-descriptions-item>
              <el-descriptions-item label="名称">{{ detail.name }}</el-descriptions-item>
              <el-descriptions-item label="编码">{{ detail.code }}</el-descriptions-item>
              <el-descriptions-item label="类型">{{ typeMap[detail.type] || detail.type }}</el-descriptions-item>
              <el-descriptions-item label="Cron">{{ detail.cronExpr || '-' }}</el-descriptions-item>
              <el-descriptions-item label="分组">{{ detail.group || '-' }}</el-descriptions-item>
              <el-descriptions-item label="超时(秒)">{{ detail.timeout }}</el-descriptions-item>
              <el-descriptions-item label="并发数">{{ detail.concurrency }}</el-descriptions-item>
              <el-descriptions-item label="重试">{{ detail.retryCount }} 次 / 间隔 {{ detail.retryInterval }}s</el-descriptions-item>
              <el-descriptions-item label="绑定节点">{{ (detail.nodes || []).map(n => n.name).join('、') || '未绑定' }}</el-descriptions-item>
              <el-descriptions-item label="状态">{{ detail.status === 1 ? '启用' : '停用' }}</el-descriptions-item>
              <el-descriptions-item label="描述">{{ detail.description || '-' }}</el-descriptions-item>
            </el-descriptions>
          </el-tab-pane>
          <el-tab-pane label="执行记录" name="exec">
            <el-table :data="history" v-loading="detailLoading" empty-text="暂无执行记录" size="small">
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
                <template #default="{ row }"><span class="error-text">{{ row.errorMessage || '-' }}</span></template>
              </el-table-column>
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </template>
    </el-drawer>

    <!-- 新建任务（复用任务管理同款表单，绑定节点固定为当前节点） -->
    <TaskFormDialog v-model="createVisible" :lock-node-id="nodeId" default-type="http" @saved="loadAll" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Monitor, Refresh, Plus, ArrowLeft } from '@element-plus/icons-vue'
import { listNodes } from '@/api/node'
import { listTasks, getTask } from '@/api/task'
import { listExecutions, getTaskHistory } from '@/api/execution'
import StatusTag from '@/components/StatusTag.vue'
import TaskFormDialog from '@/components/TaskFormDialog.vue'

const route = useRoute()
const router = useRouter()

const nodeId = Number(route.params.id)
const node = ref(null)
const loading = ref(false)
const activeTab = ref('tasks')
const tasks = ref([])
const executions = ref([])

const typeMap = { cron: 'Cron', http: 'HTTP', shell: 'Shell', grpc: 'gRPC' }
const triggerMap = { manual: '手动', cron: 'Cron', schedule: '调度', http: 'HTTP', shell: 'Shell', grpc: 'gRPC' }

async function loadAll() {
  loading.value = true
  try {
    // 节点基础信息（列表接口无单节点查询，本地定位）
    const nodes = await listNodes()
    node.value = nodes.find(n => n.id === nodeId) || null
    const [taskData, execData] = await Promise.all([
      listTasks({ page: 1, pageSize: 500 }),
      listExecutions({ worker: node.value?.name || '', page: 1, pageSize: 200 })
    ])
    const list = Array.isArray(taskData) ? taskData : (taskData.list || [])
    tasks.value = list.filter(t => (t.nodes || []).some(n => n.id === nodeId))
    executions.value = execData.list || []
  } finally {
    loading.value = false
  }
}

// ---------- 任务详情 ----------
const detailVisible = ref(false)
const detailTab = ref('info')
const detail = ref(null)
const history = ref([])
const detailLoading = ref(false)

async function openTaskDetail(row) {
  detailVisible.value = true
  detailTab.value = 'info'
  detail.value = null
  detailLoading.value = true
  history.value = []
  try {
    const [d, h] = await Promise.all([getTask(row.id), getTaskHistory(row.id, 50)])
    detail.value = d
    history.value = h || []
  } finally {
    detailLoading.value = false
  }
}

// ---------- 新建任务 ----------
const createVisible = ref(false)
function openCreateTask() {
  createVisible.value = true
}
function pad(n) { return n < 10 ? '0' + n : '' + n }
function formatTime(t) {
  if (!t) return '-'
  const d = new Date(t)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
function durationLabel(ms) {
  if (ms == null) return '-'
  if (ms < 1000) return ms + 'ms'
  const s = Math.floor(ms / 1000)
  if (s < 60) return s + 's'
  return Math.floor(s / 60) + 'm' + (s % 60) + 's'
}

onMounted(loadAll)
</script>

<style scoped>
.node-tasks-page { display: flex; flex-direction: column; gap: 16px; }
.card-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
.card-title { display: flex; align-items: center; gap: 8px; font-size: 16px; font-weight: 600; color: #303133; }
.node-info { font-size: 12px; font-weight: 400; color: #909399; margin-left: 4px; }
.card-actions { display: flex; gap: 8px; }
.tab-toolbar { margin-bottom: 12px; }
.error-text { color: #f56c6c; }
</style>
