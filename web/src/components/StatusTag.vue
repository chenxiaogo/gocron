<template>
  <el-tag :type="tagType" size="small" effect="light" round>
    <span class="dot" :style="{ background: dotColor }"></span>
    {{ label }}
  </el-tag>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  status: { type: String, default: 'success' }
})

const map = {
  success: { label: '成功', type: 'success', dot: '#67c23a' },
  failed: { label: '失败', type: 'danger', dot: '#f56c6c' },
  timeout: { label: '超时', type: 'warning', dot: '#e6a23c' },
  running: { label: '执行中', type: 'primary', dot: '#4f6ef7' },
  pending: { label: '待执行', type: 'info', dot: '#909399' },
  retrying: { label: '重试中', type: 'warning', dot: '#e6a23c' },
  canceled: { label: '已取消', type: 'info', dot: '#909399' },
  online: { label: '在线', type: 'success', dot: '#13ce66' },
  offline: { label: '离线', type: 'info', dot: '#909399' },
  busy: { label: '繁忙', type: 'warning', dot: '#e6a23c' },
  warning: { label: '警告', type: 'warning', dot: '#e6a23c' },
  error: { label: '异常', type: 'danger', dot: '#f56c6c' },
  enabled: { label: '启用', type: 'success', dot: '#67c23a' },
  disabled: { label: '停用', type: 'info', dot: '#909399' }
}

const conf = computed(() => map[props.status] || map.success)
const tagType = computed(() => conf.value.type)
const label = computed(() => conf.value.label)
const dotColor = computed(() => conf.value.dot)
</script>

<style scoped>
.dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: 4px;
  vertical-align: middle;
}
</style>
