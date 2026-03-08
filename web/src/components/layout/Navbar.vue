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
      <!-- 隐私模式 -->
      <el-tooltip :content="privacyMode ? '隐私模式已开启' : '开启隐私模式'" placement="bottom">
        <el-button
          class="nav-icon-btn"
          :class="{ active: privacyMode }"
          circle
          @click="togglePrivacy"
        >
          <el-icon><component :is="privacyMode ? Hide : View" /></el-icon>
        </el-button>
      </el-tooltip>

      <!-- 主题切换 -->
      <el-tooltip :content="isDark ? '切换到浅色模式' : '切换到深色模式'" placement="bottom">
        <el-button
          class="nav-icon-btn"
          circle
          @click="toggleTheme"
        >
          <el-icon><component :is="isDark ? Sunny : Moon" /></el-icon>
        </el-button>
      </el-tooltip>

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
            {{ displayUsername.charAt(0).toUpperCase() }}
          </el-avatar>
          <span class="username">{{ displayUsername }}</span>
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
import { computed, ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox, ElMessage } from 'element-plus'
import { 
  Fold, 
  Expand, 
  ArrowDown, 
  User, 
  SwitchButton,
  Moon,
  Sunny,
  View,
  Hide
} from '@element-plus/icons-vue'

const props = defineProps({
  isCollapse: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['toggle-sidebar', 'update:privacy-mode'])

const route = useRoute()
const router = useRouter()

const username = ref(localStorage.getItem('username') || 'Admin')
const isDark = ref(localStorage.getItem('theme') === 'dark')
const privacyMode = ref(localStorage.getItem('privacyMode') === 'true')

const displayUsername = computed(() => {
  if (privacyMode.value) return '***'
  return username.value
})

const currentTitle = computed(() => {
  return route.meta.title || 'GoBup'
})

const applyTheme = () => {
  if (isDark.value) {
    document.documentElement.setAttribute('data-theme', 'dark')
  } else {
    document.documentElement.removeAttribute('data-theme')
  }
}

const toggleTheme = () => {
  isDark.value = !isDark.value
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
  applyTheme()
}

const togglePrivacy = () => {
  privacyMode.value = !privacyMode.value
  localStorage.setItem('privacyMode', String(privacyMode.value))
  emit('update:privacy-mode', privacyMode.value)
  ElMessage({
    message: privacyMode.value ? '隐私模式已开启' : '隐私模式已关闭',
    type: privacyMode.value ? 'warning' : 'success',
    duration: 1500
  })
}

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

onMounted(() => {
  applyTheme()
  emit('update:privacy-mode', privacyMode.value)
})
</script>

<style scoped lang="scss">
.navbar-container {
  height: var(--navbar-height);
  background: var(--bg-color-secondary);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 var(--spacing-lg);
  position: sticky;
  top: 0;
  z-index: var(--z-navbar);
  border-bottom: 1px solid var(--border-color);
  transition: background-color var(--transition-normal), border-color var(--transition-normal);
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
  gap: var(--spacing-sm);
}

.nav-icon-btn {
  border: 1px solid var(--border-color);
  background: transparent;
  color: var(--text-color-secondary);
  transition: all var(--transition-normal);
  
  &:hover {
    border-color: var(--primary-color);
    color: var(--primary-color);
    background-color: var(--bg-color-hover);
  }

  &.active {
    border-color: var(--warning-color);
    color: var(--warning-color);
    background-color: rgba(230, 162, 60, 0.1);
  }
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
