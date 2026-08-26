<template>
  <div class="execution-page">
    <PageHero title="执行管理" desc="查看任务执行记录、耗时与日志" icon="Timer" color="#7c5cf0" color2="#4f6ef7">
      <template #actions>
        <el-button :icon="Refresh" class="hero-btn" @click="loadExecutions()">刷新</el-button>
      </template>
    </PageHero>

    <div class="app-card">
      <div class="card-header">
        <div class="card-title">
          <el-icon color="#722ed1"><Timer /></el-icon>
          <span>执行记录</span>
        </div>
        <div class="card-actions">
          <el-select
            v-model="query.status"
            placeholder="状态"
            clearable
            style="width: 120px"
            @change="loadExecutions(1)"
          >
            <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
          </el-select>
          <el-select
            v-model="query.trigger"
            placeholder="触发方式"
            clearable
            style="width: 130px"
            @change="loadExecutions(1)"
          >
            <el-option v-for="t in triggerOptions" :key="t.value" :label="t.label" :value="t.value" />
          </el-select>
        </div>
      </div>

      <el-table :data="executions" v-loading="loading" style="width: 100%" empty-text="暂无执行记录">
        <el-table-column prop="id" label="执行ID" width="90" />
        <el-table-column prop="taskName" label="任务" min-width="150" show-overflow-tooltip />
        <el-table-column label="类型" width="80">
          <template #default="{ row }">
            <el-tag size="small" type="info" effect="plain">{{ typeLabel(row.taskType) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="触发方式" width="90">
          <template #default="{ row }">{{ triggerLabel(row.triggerType) }}</template>
        </el-table-column>
        <el-table-column prop="workerName" label="节点" width="100" />
        <el-table-column label="开始时间" width="160">
          <template #default="{ row }">{{ formatTime(row.startedAt) }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="100">
          <template #default="{ row }">{{ durationLabel(row.durationMs) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <StatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="viewDetail(row)">日志</el-button>
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
          @size-change="loadExecutions(1)"
          @current-change="loadExecutions()"
        />
      </div>
    </div>

    <!-- 日志抽屉 -->
    <el-drawer v-model="drawerVisible" :title="`执行日志 #${currentEx?.id} · ${currentEx?.taskName}`" size="52%">
      <div class="drawer-meta" v-if="currentEx">
        <el-descriptions :column="3" border size="small">
          <el-descriptions-item label="状态">
            <StatusTag :status="currentEx.status" />
          </el-descriptions-item>
          <el-descriptions-item label="触发方式">{{ triggerLabel(currentEx.triggerType) }}</el-descriptions-item>
          <el-descriptions-item label="节点">{{ currentEx.workerName }}</el-descriptions-item>
          <el-descriptions-item label="开始时间">{{ formatTime(currentEx.startedAt) }}</el-descriptions-item>
          <el-descriptions-item label="耗时">{{ durationLabel(currentEx.durationMs) }}</el-descriptions-item>
          <el-descriptions-item label="重试次数">{{ currentEx.retryCount }}</el-descriptions-item>
          <el-descriptions-item label="错误信息" :span="3" v-if="currentEx.errorMessage">
            <span class="error-text">{{ currentEx.errorMessage }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </div>
      <div class="log-viewer">
        <div v-for="log in logs" :key="log.id" class="log-line" :class="log.Level">
          <span class="log-time">{{ formatTime(log.createdAt) }}</span>
          <span class="log-level" :class="log.Level">{{ log.Level }}</span>
          <span class="log-content">{{ log.content }}</span>
        </div>
        <el-empty v-if="!logs.length" description="暂无日志" :image-size="60" />
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { listExecutions, getLogs } from '@/api/execution'
import StatusTag from '@/components/StatusTag.vue'
import PageHero from '@/components/PageHero.vue'

const loading = ref(false)
const executions = ref([])
const total = ref(0)
const drawerVisible = ref(false)
const currentEx = ref(null)
const logs = ref([])

const query = reactive({ status: '', trigger: '', page: 1, pageSize: 20 })

const typeMap = { cron: 'Cron', http: 'HTTP', shell: 'Shell', grpc: 'gRPC' }
const typeLabel = (t) => typeMap[t] || t
const triggerMap = { cron: '调度', manual: '手动', retry: '重试' }
const triggerLabel = (t) => triggerMap[t] || t

const statusOptions = [
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '运行中', value: 'running' },
  { label: '超时', value: 'timeout' },
  { label: '已取消', value: 'canceled' },
  { label: '重试中', value: 'retrying' },
  { label: '待执行', value: 'pending' }
]
const triggerOptions = [
  { label: '调度', value: 'cron' },
  { label: '手动', value: 'manual' },
  { label: '重试', value: 'retry' }
]

function formatTime(t) {
  if (!t) return '-'
  return t.replace('T', ' ').slice(0, 19)
}
function durationLabel(ms) {
  if (ms == null) return '-'
  if (ms < 1000) return ms + 'ms'
  return (ms / 1000).toFixed(2) + 's'
}

async function loadExecutions(page) {
  if (page) query.page = page
  loading.value = true
  try {
    const data = await listExecutions(query)
    executions.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

async function viewDetail(row) {
  currentEx.value = row
  logs.value = await getLogs(row.id)
  drawerVisible.value = true
}

onMounted(loadExecutions)
</script>

<style scoped>
.execution-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.hero-btn {
  background: rgba(255, 255, 255, 0.95) !important;
  color: #7c5cf0 !important;
  border-color: transparent !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15) !important;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 12px;
}
.card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
.drawer-meta {
  margin-bottom: 16px;
}
.error-text {
  color: #f56c6c;
}
.log-viewer {
  background: #1e1e1e;
  border-radius: 8px;
  padding: 12px;
  height: calc(100vh - 300px);
  overflow-y: auto;
  font-family: 'Courier New', monospace;
  font-size: 12px;
}
.log-line {
  display: grid;
  grid-template-columns: 130px 50px 1fr;
  gap: 8px;
  padding: 2px 0;
  color: #d4d4d4;
}
.log-time {
  color: #858585;
}
.log-level.info {
  color: #569cd6;
}
.log-level.error {
  color: #f48771;
}
.log-level.warn {
  color: #ce9178;
}
</style>
