<template>
  <div 
    class="sidebar-container"
    :class="{ 'is-collapse': isCollapse, 'is-mobile': isMobile }"
  >
    <!-- Logo区域 -->
    <div class="sidebar-logo">
      <transition name="fade">
        <h1 v-if="!isCollapse">GoBup</h1>
        <h1 v-else class="logo-collapsed">G</h1>
      </transition>
      <span v-if="!isCollapse">直播录制管理</span>
    </div>

    <!-- 菜单 -->
    <el-scrollbar class="scrollbar-wrapper">
      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapse"
        :unique-opened="true"
        :collapse-transition="false"
        router
        class="sidebar-menu"
      >
        <el-menu-item index="/dashboard">
          <el-icon><Odometer /></el-icon>
          <template #title>控制面板</template>
        </el-menu-item>
        
        <el-menu-item index="/rooms">
          <el-icon><VideoCamera /></el-icon>
          <template #title>房间管理</template>
        </el-menu-item>
        
        <el-menu-item index="/history">
          <el-icon><DocumentCopy /></el-icon>
          <template #title>录制历史</template>
        </el-menu-item>
        
        <el-menu-item index="/users">
          <el-icon><User /></el-icon>
          <template #title>用户管理</template>
        </el-menu-item>
        
        <el-menu-item index="/logs">
          <el-icon><Document /></el-icon>
          <template #title>系统日志</template>
        </el-menu-item>
      </el-menu>
    </el-scrollbar>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { 
  Odometer, 
  VideoCamera, 
  DocumentCopy, 
  User, 
  Document,
  Warning
} from '@element-plus/icons-vue'

defineProps({
  isCollapse: {
    type: Boolean,
    default: false
  },
  isMobile: {
    type: Boolean,
    default: false
  }
})

const route = useRoute()
const activeMenu = computed(() => route.path)
</script>

<style scoped lang="scss">
.sidebar-container {
  height: 100vh;
  width: var(--sidebar-width);
  background: var(--sidebar-bg);
  box-shadow: var(--sidebar-shadow);
  border-right: 1px solid var(--border-color);
  transition: width 0.3s ease, background-color 0.3s ease, border-color 0.3s ease, box-shadow 0.3s ease;
  overflow: hidden;
  position: fixed;
  left: 0;
  top: 0;
  z-index: var(--z-sidebar);
  
  &.is-collapse {
    width: var(--sidebar-width-collapsed);
  }
  
  &.is-mobile {
    transform: translateX(-100%);
    
    &:not(.is-collapse) {
      transform: translateX(0);
    }
  }
}

.sidebar-logo {
  height: var(--navbar-height);
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  padding: var(--spacing-md);
  background: var(--sidebar-logo-bg);
  border-bottom: 1px solid var(--sidebar-logo-border);
  margin-bottom: var(--spacing-sm);
  
  h1 {
    color: var(--sidebar-logo-title);
    font-weight: var(--font-weight-bold);
    font-size: var(--font-size-2xl);
    margin: 0;
    transition: opacity 0.3s, color 0.3s;
    line-height: 1.2;
    
    &.logo-collapsed {
      font-size: var(--font-size-3xl);
    }
  }
  
  span {
    font-size: var(--font-size-xs);
    color: var(--sidebar-logo-subtitle);
    margin-top: 4px;
    white-space: nowrap;
    transition: color 0.3s;
  }
}

.scrollbar-wrapper {
  height: calc(100vh - var(--navbar-height));
  overflow-x: hidden;
}

.sidebar-menu {
  border: none;
  background: transparent;
  padding: var(--spacing-sm) 0;
  
  :deep(.el-menu-item) {
    height: 48px;
    line-height: 48px;
    color: var(--sidebar-item-text);
    border-left: 3px solid transparent;
    transition: var(--transition-normal);
    margin: var(--spacing-xs) 0;
    padding: 0 var(--spacing-lg);
    background-color: transparent;
    
    &:hover {
      background-color: var(--sidebar-item-hover-bg);
      color: var(--sidebar-item-hover-text);
    }
    
    &.is-active {
      background-color: var(--sidebar-item-active-bg);
      color: var(--sidebar-item-active-text);
      border-left-color: var(--sidebar-item-active-border);
      font-weight: var(--font-weight-semibold);
    }
    
    .el-icon {
      color: inherit;
      font-size: 18px;
      margin-right: 12px;
    }
  }
  
  &.el-menu--collapse {
    :deep(.el-menu-item) {
      padding: 0 20px;
      text-align: center;
      
      .el-icon {
        margin-right: 0;
      }
    }
  }
}

/* Fade transition */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* 响应式 */
@media (max-width: 768px) {
  .sidebar-container {
    &:not(.is-collapse) {
      box-shadow: 2px 0 16px rgba(0, 0, 0, 0.2);
    }
  }
}
</style>
