import request from './request'

// 首页概览指标
export function getOverview() {
  return request.get('/dashboard/overview')
}

// 执行趋势
export function getTrend(range) {
  return request.get('/dashboard/trend', { params: { range } })
}

// 异常任务
export function getFailures() {
  return request.get('/dashboard/failures')
}

// 最近执行
export function getRecent() {
  return request.get('/dashboard/recent')
}

// 即将执行
export function getUpcoming() {
  return request.get('/dashboard/upcoming')
}

// 节点资源
export function getResources() {
  return request.get('/dashboard/resources')
}

// 集群聚合资源统计
export function getCluster() {
  return request.get('/dashboard/cluster')
}

// 图表分布统计（任务类型/状态、执行状态、节点状态）
export function getCharts() {
  return request.get('/dashboard/charts')
}
