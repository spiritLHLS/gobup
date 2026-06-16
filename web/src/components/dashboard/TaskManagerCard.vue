<template>
  <el-card class="task-card" v-loading="loading">
    <template #header>
      <div class="card-header">
        <span>任务管理</span>
        <el-button size="default" :icon="Refresh" @click="$emit('refresh')">刷新</el-button>
      </div>
    </template>

    <div class="task-summary">
      <div class="task-summary-item">
        <span class="task-number">{{ uploadCounts.pending || 0 }}</span>
        <span class="task-label">上传待处理</span>
      </div>
      <div class="task-summary-item">
        <span class="task-number">{{ uploadCounts.running || 0 }}</span>
        <span class="task-label">上传执行中</span>
      </div>
      <div class="task-summary-item">
        <span class="task-number">{{ status.parse?.queueLength || 0 }}</span>
        <span class="task-label">弹幕解析队列</span>
      </div>
      <div class="task-summary-item">
        <span class="task-number">{{ syncCounts.pending || 0 }}</span>
        <span class="task-label">视频同步待处理</span>
      </div>
      <div class="task-summary-item">
        <span class="task-number">{{ publishCounts.cooldown || 0 }}</span>
        <span class="task-label">投稿冷却</span>
      </div>
      <div class="task-summary-item">
        <span class="task-number">{{ syncCounts.failed || 0 }}</span>
        <span class="task-label">同步失败</span>
      </div>
    </div>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="上传任务" name="upload">
        <el-table :data="uploadRows" height="260" empty-text="暂无上传任务">
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="uploadStatusTag(row.status)" size="small">{{ uploadStatusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="fileName" label="文件" min-width="220" show-overflow-tooltip />
          <el-table-column label="大小" width="100">
            <template #default="{ row }">{{ formatBytes(row.fileSize) }}</template>
          </el-table-column>
          <el-table-column prop="uploadErrorMsg" label="最近信息" min-width="180" show-overflow-tooltip />
          <el-table-column label="更新时间" width="170">
            <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="弹幕任务" name="danmaku">
        <div class="task-inline-status">
          <el-tag :type="status.parse?.processing ? 'warning' : 'info'">
            解析队列 {{ status.parse?.processing ? '执行中' : '空闲' }}
          </el-tag>
          <span>队列长度：{{ status.parse?.queueLength || 0 }}</span>
          <span>发送队列：{{ danmakuQueueText }}</span>
        </div>
      </el-tab-pane>

      <el-tab-pane label="投稿后处理" name="publish">
        <el-table :data="status.publish?.cooldown || []" height="260" empty-text="暂无投稿冷却任务">
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="title" label="标题" min-width="180" show-overflow-tooltip />
          <el-table-column prop="publishErrorType" label="错误类型" width="100">
            <template #default="{ row }">
              <el-tag type="warning" size="small">{{ row.publishErrorType || 'rate_limit' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="message" label="最近信息" min-width="180" show-overflow-tooltip />
          <el-table-column label="下次尝试" width="170">
            <template #default="{ row }">{{ formatTime(row.publishCooldownAt) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="视频同步" name="sync">
        <el-table :data="status.sync?.tasks || []" height="260" empty-text="暂无同步任务">
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="syncStatusTag(row.status)" size="small">{{ syncStatusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="historyId" label="历史ID" width="90" />
          <el-table-column prop="bvid" label="BV号" width="140" show-overflow-tooltip />
          <el-table-column prop="message" label="信息" min-width="180" show-overflow-tooltip />
          <el-table-column prop="lastError" label="最近错误" min-width="180" show-overflow-tooltip />
          <el-table-column label="下次执行" width="170">
            <template #default="{ row }">{{ formatTime(row.nextRunAte || row.nextRunAt) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </el-card>
</template>

<script setup>
import { computed, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'

const props = defineProps({
  status: {
    type: Object,
    default: () => ({})
  },
  loading: {
    type: Boolean,
    default: false
  }
})

defineEmits(['refresh'])

const activeTab = ref('upload')

const uploadCounts = computed(() => props.status.upload?.counts || {})
const syncCounts = computed(() => props.status.sync?.counts || {})
const publishCounts = computed(() => props.status.publish?.counts || {})

const uploadRows = computed(() => {
  const upload = props.status.upload || {}
  return [
    ...(upload.running || []),
    ...(upload.pending || []),
    ...(upload.paused || []),
    ...(upload.cancelled || [])
  ].slice(0, 80)
})

const danmakuQueueText = computed(() => {
  const queues = props.status.danmaku?.queues || {}
  const values = Object.values(queues)
  if (values.length === 0) return '0'
  return values.reduce((sum, val) => sum + Number(val || 0), 0)
})

const formatBytes = (bytes) => {
  if (!bytes) return '-'
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${Math.round(bytes / 1024)} KB`
}

const formatTime = (value) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN')
}

const uploadStatusLabel = (status) => {
  const labels = {
    pending: '待上传',
    running: '上传中',
    paused: '已暂停',
    cancelled: '已取消',
    cooldown: '冷却中'
  }
  return labels[status] || status || '-'
}

const uploadStatusTag = (status) => {
  const types = {
    running: 'primary',
    paused: 'warning',
    cancelled: 'danger',
    cooldown: 'warning'
  }
  return types[status] || 'info'
}

const syncStatusLabel = (status) => {
  const labels = {
    pending: '待处理',
    running: '执行中',
    completed: '已完成',
    failed: '失败'
  }
  return labels[status] || status || '-'
}

const syncStatusTag = (status) => {
  const types = {
    running: 'primary',
    completed: 'success',
    failed: 'danger'
  }
  return types[status] || 'info'
}
</script>

<style scoped lang="scss">
.task-card {
  margin-bottom: var(--spacing-xl);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;

  > span {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--text-color-primary);
  }
}

.task-summary {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);

  @media (max-width: 960px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  @media (max-width: 520px) {
    grid-template-columns: 1fr;
  }
}

.task-summary-item {
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-medium);
  padding: var(--spacing-md);
  background: var(--bg-color-secondary);
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.task-number {
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-bold);
  color: var(--text-color-primary);
}

.task-label {
  font-size: var(--font-size-sm);
  color: var(--text-color-secondary);
}

.task-inline-status {
  min-height: 160px;
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-md);
  flex-wrap: wrap;
  color: var(--text-color-secondary);
}
</style>
