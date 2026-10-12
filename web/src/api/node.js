import request from './request'

export function listNodes() {
  return request.get('/nodes')
}

export function getNode(id) {
  return request.get(`/nodes/${id}`)
}

export function createNode(data) {
  return request.post('/nodes', data)
}

export function updateNode(id, data) {
  return request.put(`/nodes/${id}`, data)
}

export function deleteNode(id) {
  return request.delete(`/nodes/${id}`)
}

export function getNodeMetrics(id, range) {
  return request.get(`/nodes/${id}/metrics`, { params: { range } })
}

// 全局出站代理（机场类 HTTP / SOCKS5）
export function getGlobalProxy() {
  return request.get('/node-proxy')
}

export function saveGlobalProxy(data) {
  return request.put('/node-proxy', data)
}

export function testGlobalProxy(target) {
  return request.post('/node-proxy/test', { target })
}
