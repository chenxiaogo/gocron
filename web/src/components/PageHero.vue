<template>
  <div class="page-hero fade-in" :style="heroStyle">
    <div class="hero-left">
      <div class="hero-icon" :style="iconStyle">
        <el-icon :size="24"><component :is="icon" /></el-icon>
      </div>
      <div>
        <div class="hero-title">{{ title }}</div>
        <div v-if="desc" class="hero-desc">{{ desc }}</div>
      </div>
    </div>
    <div class="hero-right">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  title: { type: String, default: '' },
  desc: { type: String, default: '' },
  icon: { type: String, default: 'DataLine' },
  color: { type: String, default: '#4f6ef7' },
  color2: { type: String, default: '#7c5cf0' }
})

const heroStyle = computed(() => ({
  background: `linear-gradient(135deg, ${props.color} 0%, ${props.color2} 100%)`,
  boxShadow: `0 8px 24px ${props.color}40`
}))

const iconStyle = computed(() => ({
  background: 'rgba(255,255,255,0.2)'
}))
</script>

<style scoped>
.page-hero {
  border-radius: var(--card-radius);
  padding: 20px 24px;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: relative;
  overflow: hidden;
}
.page-hero::after {
  content: '';
  position: absolute;
  right: -30px;
  top: -50px;
  width: 150px;
  height: 150px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
}
.hero-left {
  display: flex;
  align-items: center;
  gap: 14px;
  position: relative;
  z-index: 1;
}
.hero-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  backdrop-filter: blur(4px);
}
.hero-title {
  font-size: 19px;
  font-weight: 700;
}
.hero-desc {
  font-size: 12px;
  opacity: 0.85;
  margin-top: 3px;
}
.hero-right {
  position: relative;
  z-index: 1;
}
</style>
