<template>
  <div class="dashboard-container">
    <div class="page-header">
      <h2>系统概览</h2>
      <p>查看录制、上传、投稿和运维入口状态</p>
    </div>

    <DashboardStats :stats="stats" />

    <div class="quick-grid">
      <el-card v-for="item in quickLinks" :key="item.path" class="quick-card" shadow="never" @click="go(item.path)">
        <div class="quick-icon">
          <el-icon><component :is="item.icon" /></el-icon>
        </div>
        <div class="quick-body">
          <div class="quick-title">{{ item.title }}</div>
          <div class="quick-desc">{{ item.desc }}</div>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Connection, Document, Operation, Setting, VideoCamera } from '@element-plus/icons-vue'
import api from '@/api'
import DashboardStats from '@/components/dashboard/DashboardStats.vue'

const router = useRouter()
const stats = ref({
  totalRecordings: 0,
  uploadedCount: 0,
  pendingCount: 0,
  failedCount: 0
})
let statsTimer = null

const quickLinks = [
  { title: '任务队列', desc: '上传队列、后台任务、暂停和重试操作', path: '/operations', icon: Operation },
  { title: '系统设置', desc: '扫盘、维护、弹幕烧录、Agent 运行配置', path: '/settings', icon: Setting },
  { title: 'Agent 管理', desc: 'Agent 节点增删改、检测、屏蔽和强制删除', path: '/agents', icon: Connection },
  { title: '录制历史', desc: '查看分P、投稿、追加和同步状态', path: '/history', icon: VideoCamera },
  { title: '系统日志', desc: '按级别筛选、手动刷新和复制当前显示日志', path: '/logs', icon: Document }
]

const loadStats = async () => {
  try {
    stats.value = await api.get('/config/stats')
  } catch (error) {
    console.error('加载统计数据失败:', error)
  }
}

const go = (path) => {
  router.push(path)
}

onMounted(() => {
  loadStats()
  statsTimer = setInterval(loadStats, 30000)
})

onUnmounted(() => {
  if (statsTimer) clearInterval(statsTimer)
})
</script>

<style scoped lang="scss">
.dashboard-container {
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--spacing-lg);

  h2 {
    font-size: var(--font-size-3xl);
    color: var(--text-color-primary);
    font-weight: var(--font-weight-bold);
    margin: 0 0 8px 0;
  }

  p {
    font-size: var(--font-size-base);
    color: var(--text-color-secondary);
    margin: 0;
  }
}

.quick-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--spacing-md);
  margin-top: var(--spacing-lg);
}

.quick-card {
  cursor: pointer;
  border-radius: 8px;

  :deep(.el-card__body) {
    display: flex;
    align-items: center;
    gap: var(--spacing-md);
    min-height: 108px;
  }
}

.quick-icon {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--bg-color-hover);
  color: var(--primary-color);
  font-size: 22px;
  flex: 0 0 auto;
}

.quick-body {
  min-width: 0;
}

.quick-title {
  font-weight: var(--font-weight-semibold);
  color: var(--text-color-primary);
  margin-bottom: 6px;
}

.quick-desc {
  color: var(--text-color-secondary);
  font-size: var(--font-size-sm);
  line-height: 1.5;
}
</style>
