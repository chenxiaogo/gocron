import request from './request'

export function listExecutions(params) {
  return request.get('/executions', { params })
}

export function getExecution(id) {
  return request.get(`/executions/${id}`)
}

export function getLogs(id) {
  return request.get(`/executions/${id}/logs`)
}

export function getTaskStats(taskId) {
  return request.get(`/executions/task/${taskId}/stats`)
}

export function getTaskHistory(taskId, limit) {
  return request.get(`/executions/task/${taskId}/history`, { params: { limit } })
}

export function getRetention() {
  return request.get('/executions/retention')
}

export function cleanupExecutions(payload) {
  return request.post('/executions/cleanup', payload)
}
