<template>
  <div class="node-page">
    <PageHero title="节点管理" desc="管理任务执行节点与资源状态" icon="Monitor" color="#06b6d4" color2="#10b981">
      <template #actions>
        <el-button v-perm="'node:manage'" type="primary" :icon="Plus" class="hero-btn" @click="openCreate">
          新增节点
        </el-button>
      </template>
    </PageHero>

    <div class="app-card">
      <div class="card-header">
        <div class="card-title">
          <el-icon color="#13c2c2"><Monitor /></el-icon>
          <span>节点列表</span>
        </div>
        <div class="card-actions">
          <el-button :icon="Refresh" @click="loadNodes">刷新</el-button>
        </div>
      </div>

      <div class="node-grid">
        <div v-for="n in nodes" :key="n.id" class="node-card" :class="'status-' + n.status">
          <div class="node-header">
            <span class="node-name">
              <StatusTag :status="n.status" />
              {{ n.name }}
            </span>
            <div class="node-actions">
              <el-dropdown v-if="userStore.hasPerm('node:manage')" @command="(cmd) => handleCommand(cmd, n)">
                <el-icon :size="18" color="#909399" class="more-icon"><MoreFilled /></el-icon>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="edit">编辑</el-dropdown-item>
                    <el-dropdown-item command="delete" divided>删除</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
              <el-icon :size="20" color="#909399"><Monitor /></el-icon>
            </div>
          </div>
          <div class="node-addr">{{ n.address }} · {{ n.os }}</div>
          <div class="node-metrics-url" v-if="n.metricsUrl">
            <el-tooltip :content="n.metricsUrl" placement="top">
              <span class="metrics-url-text">{{ n.metricsUrl }}</span>
            </el-tooltip>
          </div>
          <div class="node-metrics">
            <div class="metric">
              <span class="metric-label">CPU</span>
              <el-progress :percentage="Math.round(n.cpuUsage)" :stroke-width="10" color="var(--primary-color)" />
            </div>
            <div class="metric">
              <span class="metric-label">内存</span>
              <el-progress :percentage="Math.round(n.memoryUsage)" :stroke-width="10" color="#67c23a" />
            </div>
            <div class="metric">
              <span class="metric-label">磁盘</span>
              <el-progress v-if="n.diskUsage > 0" :percentage="Math.round(n.diskUsage)" :stroke-width="10" color="#e6a23c" />
              <span v-else class="metric-na">-</span>
            </div>
          </div>
          <div class="node-footer">
            <span>负载 {{ n.loadAverage }}</span>
            <span>运行 {{ n.runningTasks }}/{{ n.maxConcurrency }}</span>
            <span v-if="n.metricsUrl" class="scrape-btn" @click="doScrape(n)">采集</span>
            <span class="heartbeat">心跳 {{ timeAgo(n.lastHeartbeat) }}</span>
          </div>
        </div>
        <el-empty v-if="!nodes.length" description="暂无节点" :image-size="80" style="grid-column: 1/-1" />
      </div>
    </div>

    <!-- 新增/编辑节点 -->
    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑节点' : '新增节点'" width="460px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="节点名称" required>
          <el-input v-model="form.name" placeholder="节点名称" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.address" placeholder="如 192.168.1.10:8080" />
        </el-form-item>
        <el-form-item label="系统">
          <el-input v-model="form.os" placeholder="如 Linux" />
        </el-form-item>
        <el-form-item label="Metrics URL">
          <el-input v-model="form.metricsUrl" placeholder="如 http://192.168.1.10:9100/metrics" />
        </el-form-item>
        <el-form-item label="最大并发">
          <el-input-number v-model="form.maxConcurrency" :min="1" :max="1000" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveNode">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Refresh, Plus, MoreFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listNodes, createNode, updateNode, deleteNode, scrapeNode } from '@/api/node'
import StatusTag from '@/components/StatusTag.vue'
import PageHero from '@/components/PageHero.vue'
import { useUserStore } from '@/stores/user'

const userStore = useUserStore()
const nodes = ref([])
const dialogVisible = ref(false)
const form = reactive({ id: null, name: '', address: '', metricsUrl: '', os: '', maxConcurrency: 10 })

function resetForm() {
  Object.assign(form, { id: null, name: '', address: '', metricsUrl: '', os: '', maxConcurrency: 10 })
}

async function loadNodes() {
  nodes.value = await listNodes()
}

const scraping = ref(false)

async function doScrape(row) {
  if (scraping.value) return
  scraping.value = true
  try {
    const res = await scrapeNode(row.id)
    if (res.code === 0) {
      ElMessage.success(`采集成功: CPU ${res.data.cpuUsage}% 内存 ${res.data.memoryUsage}% 磁盘 ${res.data.diskUsage}%`)
      loadNodes()
    } else {
      ElMessage.warning(res.message || '采集失败')
    }
  } catch (e) {
    ElMessage.error('采集请求失败')
  } finally {
    scraping.value = false
  }
}

function timeAgo(t) {
  if (!t) return '-'
  const diff = Date.now() - new Date(t).getTime()
  if (diff < 60000) return Math.floor(diff / 1000) + 's前'
  if (diff < 3600000) return Math.floor(diff / 60000) + 'm前'
  return Math.floor(diff / 3600000) + 'h前'
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row) {
  Object.assign(form, {
    id: row.id, name: row.name, address: row.address, metricsUrl: row.metricsUrl || '',
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

async function handleCommand(cmd, row) {
  if (cmd === 'edit') {
    openEdit(row)
  } else if (cmd === 'delete') {
    await ElMessageBox.confirm(`确认删除节点「${row.name}」？`, '删除确认', { type: 'error' })
    await deleteNode(row.id)
    ElMessage.success('节点已删除')
    loadNodes()
  }
}

onMounted(loadNodes)
</script>

<style scoped>
.node-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.hero-btn {
  background: rgba(255, 255, 255, 0.95) !important;
  color: #06b6d4 !important;
  border-color: transparent !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15) !important;
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
.node-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.more-icon {
  cursor: pointer;
}
.node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}
.node-card {
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 10px;
  padding: 16px;
  transition: transform 0.3s, box-shadow 0.3s;
}
.node-card:hover {
  transform: translateY(-3px);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.1);
}
.node-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.node-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}
.node-addr {
  font-size: 12px;
  color: #909399;
  margin-bottom: 12px;
}
.node-metrics {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}
.metric {
  display: grid;
  grid-template-columns: 36px 1fr;
  align-items: center;
  gap: 8px;
}
.metric-label {
  font-size: 13px;
  color: #606266;
}
.metric-na {
  font-size: 12px;
  color: #c0c4cc;
}
.node-footer {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: #909399;
  border-top: 1px dashed #ebeef5;
  padding-top: 10px;
}
.heartbeat {
  margin-left: auto;
}
.node-metrics-url {
  margin-bottom: 8px;
}
.metrics-url-text {
  font-size: 11px;
  color: #a0aec0;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.scrape-btn {
  font-size: 12px;
  color: var(--primary-color);
  cursor: pointer;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 12px;
  background: rgba(79, 110, 247, 0.08);
  transition: all 0.2s;
  margin-left: auto;
}
.scrape-btn:hover {
  background: rgba(79, 110, 247, 0.18);
  color: #3b5de7;
}
</style>
