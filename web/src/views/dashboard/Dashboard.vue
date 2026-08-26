<template>
  <div class="dashboard">
    <!-- 欢迎横幅 -->
    <div class="welcome-hero fade-in">
      <div class="hero-content">
        <div class="hero-greeting">
          {{ greeting }}，{{ userStore.nickname }} 👋
        </div>
        <div class="hero-desc">欢迎回到调度平台，这里是今天的任务执行概览。</div>
      </div>
      <div class="hero-time">
        <div class="hero-date">{{ todayText }}</div>
      </div>
      <div class="hero-deco deco-1"></div>
      <div class="hero-deco deco-2"></div>
    </div>

    <!-- 顶部指标卡 -->
    <div class="stat-grid">
      <StatCard
        label="任务总数"
        :value="overview.taskTotal"
        icon="SetUp"
        color="#4f6ef7"
        :trend="overview.taskWeekPct"
        :extra="`较上周 ${overview.taskWeekGrowth > 0 ? '+' : ''}${overview.taskWeekGrowth}`"
      />
      <StatCard
        label="今日执行次数"
        :value="overview.todayExecutions"
        icon="Clock"
        color="#7c5cf0"
        :trend="overview.todayExecPct"
        :extra="`较昨日 ${overview.todayExecGrowth > 0 ? '+' : ''}${overview.todayExecGrowth}`"
      />
      <StatCard
        label="今日成功"
        :value="overview.todaySuccess"
        icon="CircleCheck"
        color="#22c55e"
        :extra="`成功率 ${overview.successRate}%`"
      />
      <StatCard
        label="今日失败"
        :value="overview.todayFailed"
        icon="CircleClose"
        color="#ef4444"
        :extra="`异常任务 ${overview.abnormalTasks}`"
      />
      <StatCard
        label="执行中"
        :value="overview.running"
        icon="Loading"
        color="#06b6d4"
        extra="实时"
      />
      <StatCard
        label="在线节点"
        :value="overview.onlineNodes"
        icon="Monitor"
        color="#10b981"
        :extra="`离线 ${overview.offlineNodes}`"
      />
    </div>

    <!-- 集群资源聚合指标卡 -->
    <div class="app-card fade-in-up cluster-card">
      <div class="card-header">
        <div class="card-title">
          <el-icon color="#13c2c2"><Odometer /></el-icon>
          <span>集群资源</span>
        </div>
        <span class="cluster-sub">全部节点聚合 · 平均使用率</span>
      </div>
      <div class="stat-grid">
        <StatCard
          label="节点数"
          :value="cluster.nodeCount"
          icon="Monitor"
          color="#10b981"
          extra="在线 {{ cluster.onlineCount }}"
        />
        <StatCard
          label="CPU 平均使用率"
          :value="cluster.cpuUsage != null ? cluster.cpuUsage + '%' : '-'"
          icon="Cpu"
          color="#4f6ef7"
          extra="聚合平均"
        />
        <StatCard
          label="内存平均使用率"
          :value="cluster.memoryUsage != null ? cluster.memoryUsage + '%' : '-'"
          icon="Coin"
          color="#7c5cf0"
          extra="聚合平均"
        />
        <StatCard
          label="磁盘平均使用率"
          :value="cluster.diskUsage != null ? cluster.diskUsage + '%' : '-'"
          icon="Files"
          color="#e6a23c"
          extra="聚合平均"
        />
        <StatCard
          label="运行中任务"
          :value="cluster.runningTasks"
          icon="Loading"
          color="#06b6d4"
          extra="实时"
        />
        <StatCard
          label="平均负载"
          :value="cluster.loadAverage != null ? cluster.loadAverage : '-'"
          icon="TrendCharts"
          color="#f59e0b"
          extra="Load Average"
        />
      </div>
    </div>

    <!-- 图形方块区 -->
    <el-row :gutter="16" class="row">
      <!-- 执行趋势（大） -->
      <el-col :xs="24" :lg="12">
        <div class="app-card fade-in-up">
          <div class="card-header">
            <div class="card-title">
              <el-icon color="var(--primary-color)"><TrendCharts /></el-icon>
              <span>执行趋势</span>
            </div>
            <el-radio-group v-model="trendRange" size="small" @change="loadTrend">
              <el-radio-button label="day">今日</el-radio-button>
              <el-radio-button label="7d">近7天</el-radio-button>
              <el-radio-button label="30d">近30天</el-radio-button>
            </el-radio-group>
          </div>
          <div ref="trendRef" class="chart-block chart-tall"></div>
        </div>
      </el-col>
      <!-- 任务类型分布 -->
      <el-col :xs="24" :lg="12">
        <div class="app-card fade-in-up">
          <div class="card-header">
            <div class="card-title">
              <el-icon color="#7c5cf0"><Grid /></el-icon>
              <span>任务类型分布</span>
            </div>
          </div>
          <div ref="typeChartRef" class="chart-block chart-tall"></div>
        </div>
      </el-col>
    </el-row>

    <!-- 第二行小方块 -->
    <el-row :gutter="16" class="row">
      <el-col :xs="24" :lg="8">
        <div class="app-card fade-in-up">
          <div class="card-header">
            <div class="card-title">
              <el-icon color="#22c55e"><CircleCheck /></el-icon>
              <span>今日执行状态</span>
            </div>
          </div>
          <div ref="execChartRef" class="chart-block"></div>
        </div>
      </el-col>
      <el-col :xs="24" :lg="8">
        <div class="app-card fade-in-up">
          <div class="card-header">
            <div class="card-title">
              <el-icon color="#4f6ef7"><SetUp /></el-icon>
              <span>任务状态</span>
            </div>
          </div>
          <div ref="statusChartRef" class="chart-block"></div>
        </div>
      </el-col>
      <el-col :xs="24" :lg="8">
        <div class="app-card fade-in-up">
          <div class="card-header">
            <div class="card-title">
              <el-icon color="#10b981"><Monitor /></el-icon>
              <span>节点状态</span>
            </div>
          </div>
          <div ref="nodeChartRef" class="chart-block"></div>
        </div>
      </el-col>
    </el-row>

    <!-- 节点资源概览 -->
    <div class="app-card fade-in-up">
      <div class="card-header">
        <div class="card-title">
          <el-icon color="#13c2c2"><Odometer /></el-icon>
          <span>节点资源概览</span>
        </div>
        <span class="live-badge"><span class="live-dot"></span>实时</span>
      </div>
      <div class="node-grid">
        <div v-for="n in resources" :key="n.id" class="node-item">
          <div class="node-top">
            <span class="node-name">
              <StatusTag :status="n.status" />
              {{ n.name }}
            </span>
            <span class="node-addr">{{ n.address }}</span>
          </div>
          <div class="node-metrics">
            <div class="metric">
              <span class="metric-label">CPU</span>
              <el-progress :percentage="Math.round(n.cpuUsage)" :stroke-width="8" :show-text="false"
                color="var(--primary-color)" />
              <span class="metric-val">{{ n.cpuUsage }}%</span>
            </div>
            <div class="metric">
              <span class="metric-label">内存</span>
              <el-progress :percentage="Math.round(n.memoryUsage)" :stroke-width="8" :show-text="false"
                color="#67c23a" />
              <span class="metric-val">{{ n.memoryUsage }}%</span>
            </div>
            <div class="metric">
              <span class="metric-label">磁盘</span>
              <el-progress :percentage="Math.round(n.diskUsage)" :stroke-width="8" :show-text="false"
                color="#e6a23c" />
              <span class="metric-val">{{ n.diskUsage }}%</span>
            </div>
          </div>
        </div>
        <el-empty v-if="!resources.length" description="暂无节点" :image-size="60" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import * as echarts from 'echarts'
import StatCard from '@/components/StatCard.vue'
import StatusTag from '@/components/StatusTag.vue'
import { getOverview, getTrend, getResources, getCluster, getCharts } from '@/api/dashboard'
import { useUserStore } from '@/stores/user'

const userStore = useUserStore()
const overview = ref({})

// 问候语 + 日期
const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '早上好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
})
const todayText = computed(() => {
  const d = new Date()
  const week = ['日', '一', '二', '三', '四', '五', '六'][d.getDay()]
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 · 星期${week}`
})
const resources = ref([])
const cluster = ref({})
const charts = ref({ taskType: [], taskStatus: [], execStatus: [], nodeStatus: [] })
const trendRange = ref('7d')
const trendRef = ref()
const typeChartRef = ref()
const execChartRef = ref()
const statusChartRef = ref()
const nodeChartRef = ref()

// echarts 实例容器
const chartsMap = {}
let timer = null

// 实时刷新：统计数据每 5s 拉取，趋势图每 30s 拉取
const REFRESH_INTERVAL = 5000
const TREND_INTERVAL = 30000

async function loadStats() {
  const [ov, rs, cl, ch] = await Promise.all([
    getOverview(),
    getResources(),
    getCluster(),
    getCharts()
  ])
  overview.value = ov
  resources.value = rs
  cluster.value = cl
  charts.value = ch
  renderPies()
}

async function loadTrend() {
  const data = await getTrend(trendRange.value)
  renderTrend(data)
}

function renderTrend(data) {
  if (!trendRef.value) return
  if (!chartsMap.trend) chartsMap.trend = echarts.init(trendRef.value)
  chartsMap.trend.setOption({
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(31,45,61,0.9)',
      borderWidth: 0,
      textStyle: { color: '#fff' }
    },
    legend: {
      data: ['执行', '成功', '失败', '超时'],
      top: 0,
      textStyle: { color: '#606266' }
    },
    grid: { left: 20, right: 20, top: 36, bottom: 20, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.map((d) => d.date),
      axisLine: { lineStyle: { color: '#e4e7ed' } },
      axisLabel: { color: '#909399' }
    },
    yAxis: {
      type: 'value',
      axisLine: { show: false },
      splitLine: { lineStyle: { color: '#f0f2f5' } },
      axisLabel: { color: '#909399' }
    },
    series: [
      { name: '执行', type: 'line', smooth: true, data: data.map((d) => d.total), lineStyle: { color: '#4f6ef7', width: 3 }, itemStyle: { color: '#4f6ef7' }, areaStyle: { color: 'rgba(79,110,247,0.1)' } },
      { name: '成功', type: 'line', smooth: true, data: data.map((d) => d.success), lineStyle: { color: '#22c55e', width: 3 }, itemStyle: { color: '#22c55e' } },
      { name: '失败', type: 'line', smooth: true, data: data.map((d) => d.failed), lineStyle: { color: '#ef4444', width: 3 }, itemStyle: { color: '#ef4444' } },
      { name: '超时', type: 'line', smooth: true, data: data.map((d) => d.timeout), lineStyle: { color: '#f59e0b', width: 3 }, itemStyle: { color: '#f59e0b' } }
    ]
  })
}

// 通用环形图渲染
function renderPie(elRef, key, data) {
  if (!elRef?.value || !data?.length) return
  if (!chartsMap[key]) chartsMap[key] = echarts.init(elRef.value)
  chartsMap[key].setOption({
    color: data.map((d) => d.color),
    tooltip: {
      trigger: 'item',
      backgroundColor: 'rgba(31,45,61,0.9)',
      borderWidth: 0,
      textStyle: { color: '#fff' }
    },
    legend: {
      bottom: 0,
      icon: 'circle',
      textStyle: { color: '#606266', fontSize: 12 }
    },
    series: [
      {
        name: '分布',
        type: 'pie',
        radius: ['55%', '75%'],
        center: ['50%', '45%'],
        avoidLabelOverlap: false,
        itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
        label: { show: false },
        emphasis: { label: { show: true, fontSize: 16, fontWeight: 'bold' } },
        data
      }
    ]
  })
}

function renderPies() {
  renderPie(typeChartRef, 'type', charts.value.taskType)
  renderPie(execChartRef, 'exec', charts.value.execStatus)
  renderPie(statusChartRef, 'status', charts.value.taskStatus)
  renderPie(nodeChartRef, 'node', charts.value.nodeStatus)
}

function handleResize() {
  Object.values(chartsMap).forEach((c) => c?.resize())
}

onMounted(async () => {
  await loadStats()
  await nextTick()
  loadTrend()
  renderPies()
  window.addEventListener('resize', handleResize)
  // 实时轮询：统计数据每 5s，趋势图每 30s
  timer = setInterval(() => {
    loadStats()
    if (Date.now() % TREND_INTERVAL < 1000) loadTrend()
  }, REFRESH_INTERVAL)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  window.removeEventListener('resize', handleResize)
  Object.values(chartsMap).forEach((c) => c?.dispose())
  chartsMap.trend = null
})
</script>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.welcome-hero {
  position: relative;
  background: var(--primary-gradient);
  border-radius: var(--card-radius);
  padding: 24px 28px;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  overflow: hidden;
  box-shadow: 0 8px 24px rgba(79, 110, 247, 0.25);
}
.hero-content {
  position: relative;
  z-index: 2;
}
.hero-greeting {
  font-size: 22px;
  font-weight: 700;
}
.hero-desc {
  font-size: 13px;
  opacity: 0.85;
  margin-top: 6px;
}
.hero-time {
  position: relative;
  z-index: 2;
  font-size: 14px;
  opacity: 0.9;
  font-weight: 500;
}
.hero-deco {
  position: absolute;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
}
.deco-1 {
  width: 160px;
  height: 160px;
  right: 120px;
  top: -60px;
}
.deco-2 {
  width: 100px;
  height: 100px;
  right: 30px;
  bottom: -40px;
}
.row {
  margin-bottom: 0;
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
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}
.cluster-card {
  padding: 20px 20px 12px;
}
.cluster-sub {
  font-size: 12px;
  color: #909399;
}
.live-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
  padding: 2px 10px;
  border-radius: 20px;
}
.live-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #10b981;
  animation: live-pulse 1.5s infinite;
}
@keyframes live-pulse {
  0% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.5); }
  70% { box-shadow: 0 0 0 6px rgba(16, 185, 129, 0); }
  100% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0); }
}
.chart-block {
  height: 260px;
  width: 100%;
}
.chart-tall {
  height: 320px;
}
.node-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.node-item {
  padding: 12px;
  background: #f8f9fb;
  border-radius: 8px;
}
.node-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.node-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  color: #303133;
}
.node-addr {
  font-size: 12px;
  color: #909399;
}
.node-metrics {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.metric {
  display: grid;
  grid-template-columns: 34px 1fr 46px;
  align-items: center;
  gap: 8px;
}
.metric-label {
  font-size: 12px;
  color: #606266;
}
.metric-val {
  font-size: 12px;
  color: #606266;
  text-align: right;
}
</style>
