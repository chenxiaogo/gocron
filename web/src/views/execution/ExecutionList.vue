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
          <el-input
            v-model="query.keyword"
            placeholder="任务名称"
            clearable
            style="width: 180px"
            @keyup.enter="loadExecutions(1)"
            @clear="loadExecutions(1)"
          />
          <el-input
            v-model="query.worker"
            placeholder="节点名称"
            clearable
            style="width: 140px"
            @keyup.enter="loadExecutions(1)"
            @clear="loadExecutions(1)"
          />
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
          <el-date-picker
            v-model="timeRange"
            type="daterange"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            value-format="YYYY-MM-DD"
            style="width: 260px"
          />
          <el-button type="primary" :icon="Search" @click="loadExecutions(1)">查询</el-button>
          <el-button :icon="RefreshLeft" @click="resetQuery">重置</el-button>
          <el-button type="warning" plain :icon="Delete" @click="openCleanup">清理</el-button>
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

    <!-- 清理执行记录 -->
    <el-dialog v-model="cleanupVisible" title="清理执行记录" width="480px">
      <el-alert
        type="info"
        :closable="false"
        style="margin-bottom: 16px"
        title="系统每日 03:07 自动按保留策略清理执行记录（保留最近 N 天，每任务超量删除最旧记录）。也可手动立即清理。"
      />
      <el-form label-width="130px">
        <el-form-item label="保留天数">
          <el-input-number v-model="cleanupForm.retainDays" :min="1" :max="365" />
          <span class="form-hint">删除早于该天数的执行记录</span>
        </el-form-item>
        <el-form-item label="每任务最大条数">
          <el-input-number v-model="cleanupForm.maxPerTask" :min="0" :max="100000" />
          <span class="form-hint">0 = 不限制（仅按时间清理）</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cleanupVisible = false">取消</el-button>
        <el-button type="warning" :loading="cleanupLoading" @click="runCleanup">立即清理</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, RefreshLeft, Search, Delete } from '@element-plus/icons-vue'
import { listExecutions, getLogs, getRetention, cleanupExecutions } from '@/api/execution'
import StatusTag from '@/components/StatusTag.vue'
import PageHero from '@/components/PageHero.vue'

const loading = ref(false)
const executions = ref([])
const total = ref(0)
const drawerVisible = ref(false)
const currentEx = ref(null)
const logs = ref([])
const timeRange = ref(null)

const query = reactive({ keyword: '', worker: '', status: '', trigger: '', page: 1, pageSize: 20 })

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
    const params = {
      keyword: query.keyword,
      worker: query.worker,
      status: query.status,
      trigger: query.trigger,
      page: query.page,
      pageSize: query.pageSize
    }
    if (timeRange.value && timeRange.value.length === 2) {
      params.start = `${timeRange.value[0]}T00:00:00`
      params.end = `${timeRange.value[1]}T23:59:59`
    }
    const data = await listExecutions(params)
    executions.value = data.list || []
    total.value = data.total || 0
  } finally {
    loading.value = false
  }
}

function resetQuery() {
  query.keyword = ''
  query.worker = ''
  query.status = ''
  query.trigger = ''
  timeRange.value = null
  loadExecutions(1)
}

async function viewDetail(row) {
  currentEx.value = row
  logs.value = await getLogs(row.id)
  drawerVisible.value = true
}

const cleanupVisible = ref(false)
const cleanupLoading = ref(false)
const cleanupForm = reactive({ retainDays: 30, maxPerTask: 500 })

async function openCleanup() {
  try {
    const res = await getRetention()
    const r = res.data || res
    if (r && r.executionRetainDays) cleanupForm.retainDays = r.executionRetainDays
    if (r && r.executionMaxPerTask != null) cleanupForm.maxPerTask = r.executionMaxPerTask
  } catch (e) {
    // 忽略，使用默认值
  }
  cleanupVisible.value = true
}

async function runCleanup() {
  try {
    await ElMessageBox.confirm(
      `确认清理？将删除早于 ${cleanupForm.retainDays} 天前的执行记录` +
        (cleanupForm.maxPerTask > 0
          ? `，并将每任务超过 ${cleanupForm.maxPerTask} 条的最旧记录一并删除。`
          : '。'),
      '清理确认',
      { type: 'warning' }
    )
  } catch {
    return // 用户取消
  }
  cleanupLoading.value = true
  try {
    const res = await cleanupExecutions({
      retainDays: cleanupForm.retainDays,
      maxPerTask: cleanupForm.maxPerTask
    })
    const d = (res.data || res) || {}
    ElMessage.success(`已清理 ${d.deletedExecutions || 0} 条执行记录、${d.deletedLogs || 0} 条日志`)
    cleanupVisible.value = false
    loadExecutions(1)
  } finally {
    cleanupLoading.value = false
  }
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
.form-hint {
  margin-left: 10px;
  color: #909399;
  font-size: 12px;
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
