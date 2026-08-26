<template>
  <div class="stat-card fade-in-up" :style="cardStyle">
    <div class="stat-glow" :style="glowStyle"></div>
    <div class="stat-icon" :style="iconStyle">
      <el-icon :size="24">
        <component :is="icon" />
      </el-icon>
    </div>
    <div class="stat-body">
      <div class="stat-label">{{ label }}</div>
      <div class="stat-value" :style="{ color }">{{ value }}</div>
      <div class="stat-extra">
        <span v-if="extra" class="extra-text">{{ extra }}</span>
        <span v-if="trend" class="trend" :class="trend > 0 ? 'up' : 'down'">
          <el-icon :size="12">
            <component :is="trend > 0 ? 'Top' : 'Bottom'" />
          </el-icon>
          {{ Math.abs(trend) }}%
        </span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  label: { type: String, default: '' },
  value: { type: [String, Number], default: 0 },
  icon: { type: String, default: 'DataLine' },
  color: { type: String, default: '#4f6ef7' },
  extra: { type: String, default: '' },
  trend: { type: Number, default: 0 }
})

// 渐变图标底
const iconStyle = computed(() => ({
  background: `linear-gradient(135deg, ${props.color}dd, ${props.color})`,
  boxShadow: `0 6px 16px ${props.color}55`
}))

// 卡片左侧渐变装饰条
const cardStyle = computed(() => ({
  borderLeft: `3px solid ${props.color}`
}))

// 卡片角落的柔和光晕
const glowStyle = computed(() => ({
  background: `radial-gradient(circle at top right, ${props.color}22, transparent 60%)`
}))
</script>

<style scoped>
.stat-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  background: #fff;
  border-radius: var(--card-radius);
  padding: 20px;
  box-shadow: var(--card-shadow);
  cursor: pointer;
  overflow: hidden;
  transition: transform 0.3s ease, box-shadow 0.3s ease;
}
.stat-card:hover {
  transform: translateY(-5px);
  box-shadow: 0 14px 30px rgba(30, 41, 59, 0.12);
}
.stat-glow {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.stat-icon {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 14px;
  color: #fff;
  flex-shrink: 0;
  z-index: 1;
}
.stat-body {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 2px;
  z-index: 1;
}
.stat-label {
  font-size: 13px;
  color: #94a3b8;
}
.stat-value {
  font-size: 28px;
  font-weight: 800;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.stat-extra {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: #cbd5e1;
}
.trend {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-weight: 600;
}
.trend.up { color: #22c55e; }
.trend.down { color: #ef4444; }
</style>
