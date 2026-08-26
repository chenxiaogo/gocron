import request from './request'

export function getNotifySettings() {
  return request.get('/notify/settings')
}

export function saveNotifySettings(data) {
  return request.put('/notify/settings', data)
}

export function sendNotifyTest() {
  return request.post('/notify/test')
}
