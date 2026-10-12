<template>
  <div class="node-page">
    <div class="app-card">
      <div class="card-header">
        <div class="card-title">
          <el-icon color="var(--primary-color)"><Monitor /></el-icon>
          <span>节点列表</span>
          <span class="node-count">{{ nodes.length }} 个节点 · {{ onlineCount }} 在线</span>
        </div>
        <div class="card-actions">
          <el-input
            v-model="searchKey"
            placeholder="搜索节点名称 / 地址"
            clearable
            :prefix-icon="Search"
            class="node-search"
          />
          <el-button :icon="Refresh" @click="loadNodes" :loading="loading">刷新</el-button>
          <el-button type="primary" :icon="Plus" v-perm="'node:manage'" @click="openCreate">新增节点</el-button>
        </div>
      </div>

      <el-table :data="filteredNodes" v-loading="loading" style="width: 100%" empty-text="暂无节点">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column label="节点名称" min-width="160">
          <template #default="{ row }">
            <span class="node-name"><StatusTag :status="row.status" />{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="address" label="地址" min-width="150" />
        <el-table-column label="操作系统" width="110">
          <template #default="{ row }">{{ row.os || '-' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small" effect="plain">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="CPU" width="130">
          <template #default="{ row }">
            <el-progress :percentage="Math.round(row.cpuUsage || 0)" :stroke-width="6" :color="progressColor(row.cpuUsage)"
              :format="() => (row.cpuUsage != null ? row.cpuUsage.toFixed(0) + '%' : '-')" />
          </template>
        </el-table-column>
        <el-table-column label="内存" width="130">
          <template #default="{ row }">
            <el-progress :percentage="Math.round(row.memoryUsage || 0)" :stroke-width="6" :color="progressColor(row.memoryUsage)"
              :format="() => (row.memoryUsage != null ? row.memoryUsage.toFixed(0) + '%' : '-')" />
          </template>
        </el-table-column>
        <el-table-column label="磁盘" width="130">
          <template #default="{ row }">
            <el-progress :percentage="Math.round(row.diskUsage || 0)" :stroke-width="6" :color="progressColor(row.diskUsage)"
              :format="() => (row.diskUsage ? row.diskUsage.toFixed(0) + '%' : '-')" />
          </template>
        </el-table-column>
        <el-table-column label="最后心跳" width="150">
          <template #default="{ row }">{{ formatTime(row.lastHeartbeat) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="openNodeTasks(row)">任务</el-button>
            <el-button size="small" type="primary" link v-perm="'node:manage'" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" link v-perm="'node:manage'" @click="removeNode(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑节点' : '新增节点'" width="460px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="节点名称" required><el-input v-model="form.name" placeholder="节点名称" /></el-form-item>
        <el-form-item label="地址"><el-input v-model="form.address" placeholder="如 192.168.1.10:8080" /></el-form-item>
        <el-form-item label="系统"><el-input v-model="form.os" placeholder="如 Linux" /></el-form-item>
        <el-form-item label="Metrics URL"><el-input v-model="form.metricsUrl" placeholder="如 http://192.168.1.10:9100/metrics" /></el-form-item>
        <el-form-item label="节点代理">
          <el-input v-model="form.proxy" placeholder="翻墙用，如 http://user:pass@host:port 或 socks5://host:port，留空走全局" />
        </el-form-item>
        <el-form-item label="最大并发"><el-input-number v-model="form.maxConcurrency" :min="1" :max="1000" style="width:100%" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveNode">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Plus, Monitor, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listNodes, createNode, updateNode, deleteNode } from '@/api/node'
import StatusTag from '@/components/StatusTag.vue'

const router = useRouter()

const nodes = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const searchKey = ref('')
const form = reactive({ id: null, name: '', address: '', metricsUrl: '', proxy: '', os: '', maxConcurrency: 10 })

const onlineCount = computed(() => nodes.value.filter(n => n.status === 'online' || n.status === 'busy').length)

const filteredNodes = computed(() => {
  const k = searchKey.value.trim().toLowerCase()
  if (!k) return nodes.value
  return nodes.value.filter(n =>
    (n.name || '').toLowerCase().includes(k) || (n.address || '').toLowerCase().includes(k)
  )
})

function statusText(s) {
  const map = { online: '在线', offline: '离线', busy: '繁忙', warning: '告警', error: '异常' }
  return map[s] || s || '-'
}
function statusType(s) {
  const map = { online: 'success', offline: 'info', busy: 'warning', warning: 'warning', error: 'danger' }
  return map[s] || 'info'
}
function progressColor(val) {
  if (val == null) return '#c0c4cc'
  if (val >= 85) return '#ef4444'
  if (val >= 70) return '#f59e0b'
  return '#22c55e'
}
function pad(n) { return n < 10 ? '0' + n : '' + n }
function formatTime(t) {
  if (!t) return '-'
  const d = new Date(t)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function resetForm() {
  Object.assign(form, { id: null, name: '', address: '', metricsUrl: '', proxy: '', os: '', maxConcurrency: 10 })
}

async function loadNodes() {
  loading.value = true
  try {
    nodes.value = await listNodes()
  } finally {
    loading.value = false
  }
}

function openNodeTasks(row) {
  router.push(`/nodes/${row.id}/tasks`)
}

function openCreate() { resetForm(); dialogVisible.value = true }

function openEdit(row) {
  Object.assign(form, {
    id: row.id, name: row.name, address: row.address, metricsUrl: row.metricsUrl || '',
    proxy: row.proxy || '',
    os: row.os, maxConcurrency: row.maxConcurrency || 10
  })
  dialogVisible.value = true
}

async function saveNode() {
  if (!form.name) return ElMessage.warning('请输入节点名称')
  if (form.id) {
    await updateNode(form.id, form)
    ElMessage.success('节点已更新')
  } else {
    await createNode(form)
    ElMessage.success('节点已创建')
  }
  dialogVisible.value = false
  loadNodes()
}

async function removeNode(row) {
  await ElMessageBox.confirm(`确认删除节点「${row.name}」？`, '删除确认', { type: 'error' })
  await deleteNode(row.id)
  ElMessage.success('节点已删除')
  loadNodes()
}

onMounted(loadNodes)
</script>

<style scoped>
.node-page { display: flex; flex-direction: column; gap: 16px; }
.card-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
.card-title { display: flex; align-items: center; gap: 8px; font-size: 16px; font-weight: 600; color: #303133; }
.node-count { font-size: 12px; font-weight: 400; color: #909399; margin-left: 4px; }
.card-actions { display: flex; gap: 8px; }
.node-search { width: 240px; }
.node-name { display: inline-flex; align-items: center; gap: 8px; font-weight: 500; color: #303133; }
</style>
