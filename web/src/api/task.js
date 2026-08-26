import request from './request'

export function listTasks(params) {
  return request.get('/tasks', { params })
}

export function getTask(id) {
  return request.get(`/tasks/${id}`)
}

export function createTask(data) {
  return request.post('/tasks', data)
}

export function updateTask(id, data) {
  return request.put(`/tasks/${id}`, data)
}

export function deleteTask(id) {
  return request.delete(`/tasks/${id}`)
}

export function runTask(id) {
  return request.post(`/tasks/${id}/run`)
}

export function enableTask(id) {
  return request.post(`/tasks/${id}/enable`)
}

export function disableTask(id) {
  return request.post(`/tasks/${id}/disable`)
}
