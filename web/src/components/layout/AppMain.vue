<template>
  <div class="app-main">
    <router-view v-slot="{ Component, route }">
      <transition name="fade-transform" mode="out-in">
        <keep-alive :include="cachedViews">
          <component :is="Component" :key="route.path" />
        </keep-alive>
      </transition>
    </router-view>
  </div>
</template>

<script setup>
import { ref } from 'vue'

// 需要缓存的视图组件名称
const cachedViews = ref(['Dashboard', 'Rooms', 'History'])
</script>

<style scoped lang="scss">
.app-main {
  width: 100%;
  min-height: calc(100vh - var(--navbar-height));
  padding: var(--spacing-lg);
  background-color: var(--bg-color-primary);
  overflow-x: hidden;
}

/* 页面切换动画 */
.fade-transform-enter-active,
.fade-transform-leave-active {
  transition: all 0.3s ease;
}

.fade-transform-enter-from {
  opacity: 0;
  transform: translateX(-20px);
}

.fade-transform-leave-to {
  opacity: 0;
  transform: translateX(20px);
}

/* 响应式 */
@media (max-width: 768px) {
  .app-main {
    padding: var(--spacing-md);
  }
}

@media (max-width: 480px) {
  .app-main {
    padding: var(--spacing-sm);
  }
}
</style>
