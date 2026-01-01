<template>
  <div class="navbar-container">
    <div class="navbar-left">
      <el-button
        class="collapse-btn"
        :icon="isCollapse ? Expand : Fold"
        circle
        @click="handleToggleSidebar"
      />
      <h3 class="page-title">{{ currentTitle }}</h3>
    </div>
    
    <div class="navbar-right">
      <!-- 用户信息下拉菜单 -->
      <el-dropdown @command="handleCommand" trigger="click">
        <div class="user-info">
          <el-avatar 
            class="user-avatar" 
            size="small"
            :style="{ 
              backgroundColor: 'var(--primary-color)',
              color: 'white'
            }"
          >
            {{ username.charAt(0).toUpperCase() }}
          </el-avatar>
          <span class="username">{{ username }}</span>
          <el-icon class="dropdown-icon"><ArrowDown /></el-icon>
        </div>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item disabled>
              <el-icon><User /></el-icon>
              {{ username }}
            </el-dropdown-item>
            <el-dropdown-item divided command="logout">
              <el-icon><SwitchButton /></el-icon>
              退出登录
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox, ElMessage } from 'element-plus'
import { 
  Fold, 
  Expand, 
  ArrowDown, 
  User, 
  SwitchButton 
} from '@element-plus/icons-vue'

const props = defineProps({
  isCollapse: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['toggle-sidebar'])

const route = useRoute()
const router = useRouter()

const username = ref(localStorage.getItem('username') || 'Admin')

const currentTitle = computed(() => {
  return route.meta.title || 'GoBup'
})

const handleToggleSidebar = () => {
  emit('toggle-sidebar')
}

const handleCommand = (command) => {
  if (command === 'logout') {
    ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    }).then(() => {
      localStorage.removeItem('username')
      localStorage.removeItem('password')
      ElMessage.success('已退出登录')
      router.push('/login')
    }).catch(() => {})
  }
}
</script>

<style scoped lang="scss">
.navbar-container {
  height: var(--navbar-height);
  background: #ffffff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 var(--spacing-lg);
  position: sticky;
  top: 0;
  z-index: var(--z-navbar);
}

.navbar-left {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  
  .collapse-btn {
    border: 1px solid var(--border-color);
    transition: var(--transition-normal);
    
    &:hover {
      border-color: var(--primary-color);
      color: var(--primary-color);
    }
  }
  
  .page-title {
    margin: 0;
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--text-color-primary);
  }
}

.navbar-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.user-info {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: 8px 12px;
  border-radius: var(--border-radius-medium);
  cursor: pointer;
  transition: var(--transition-normal);
  
  &:hover {
    background-color: var(--bg-color-hover);
  }
  
  .user-avatar {
    font-weight: var(--font-weight-semibold);
  }
  
  .username {
    font-size: var(--font-size-sm);
    color: var(--text-color-primary);
    font-weight: var(--font-weight-medium);
  }
  
  .dropdown-icon {
    color: var(--text-color-secondary);
    font-size: 12px;
  }
}

/* 响应式 */
@media (max-width: 768px) {
  .navbar-container {
    padding: 0 var(--spacing-md);
  }
  
  .page-title {
    font-size: var(--font-size-base);
  }
  
  .username {
    display: none;
  }
}
</style>
