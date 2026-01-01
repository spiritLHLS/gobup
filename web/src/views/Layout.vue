<template>
  <div class="layout-container" :class="{ 'is-mobile': isMobile }">
    <!-- 侧边栏 -->
    <Sidebar 
      :is-collapse="isCollapse" 
      :is-mobile="isMobile"
    />
    
    <!-- 主内容区域 -->
    <div class="main-container" :class="{ 'is-collapse': isCollapse }">
      <!-- 导航栏 -->
      <Navbar 
        :is-collapse="isCollapse"
        @toggle-sidebar="handleToggleSidebar" 
      />
      
      <!-- 主要内容 -->
      <AppMain />
    </div>
    
    <!-- 移动端遮罩 -->
    <div 
      v-if="isMobile && !isCollapse"
      class="mobile-mask"
      @click="handleToggleSidebar"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { Sidebar, Navbar, AppMain } from '@/components/layout'

const isCollapse = ref(false)
const isMobile = ref(false)

// 检测设备类型
const checkDevice = () => {
  const width = window.innerWidth
  isMobile.value = width < 768
  
  // 移动端默认折叠侧边栏
  if (isMobile.value) {
    isCollapse.value = true
  }
}

// 切换侧边栏
const handleToggleSidebar = () => {
  isCollapse.value = !isCollapse.value
}

// 响应式监听
let resizeTimer = null
const handleResize = () => {
  if (resizeTimer) clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    checkDevice()
  }, 100)
}

onMounted(() => {
  checkDevice()
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  if (resizeTimer) clearTimeout(resizeTimer)
})
</script>

<style scoped lang="scss">
.layout-container {
  display: flex;
  width: 100%;
  min-height: 100vh;
  background-color: var(--bg-color-primary);
  
  &.is-mobile {
    .main-container {
      margin-left: 0;
    }
  }
}

.main-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  margin-left: var(--sidebar-width);
  transition: margin-left var(--transition-normal);
  min-height: 100vh;
  
  &.is-collapse {
    margin-left: var(--sidebar-width-collapsed);
  }
}

.mobile-mask {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-color: rgba(0, 0, 0, 0.5);
  z-index: calc(var(--z-sidebar) - 1);
  animation: fadeIn 0.3s;
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

/* 响应式 */
@media (max-width: 768px) {
  .main-container {
    margin-left: 0 !important;
  }
}
</style>
