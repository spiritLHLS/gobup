<template>
  <el-card class="queue-card" v-loading="loading">
    <template #header>
      <div class="card-header">
        <span>上传队列</span>
        <el-button size="default" @click="$emit('refresh')" :icon="Refresh">刷新</el-button>
      </div>
    </template>

    <div class="queue-summary">
      <div class="queue-summary-item">
        <span class="queue-number">{{ status.counts?.pending || 0 }}</span>
        <span class="queue-label">待上传</span>
      </div>
      <div class="queue-summary-item">
        <span class="queue-number">{{ status.counts?.running || 0 }}</span>
        <span class="queue-label">上传中</span>
      </div>
      <div class="queue-summary-item">
        <span class="queue-number">{{ status.counts?.completed || 0 }}</span>
        <span class="queue-label">已完成</span>
      </div>
    </div>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="待上传" name="pending">
        <el-table :data="status.pending || []" height="260" empty-text="暂无待上传任务">
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="fileName" label="文件" min-width="220" show-overflow-tooltip />
          <el-table-column label="大小" width="100">
            <template #default="{ row }">{{ formatBytes(row.fileSize) }}</template>
          </el-table-column>
          <el-table-column prop="uploadErrorMsg" label="最近错误" min-width="180" show-overflow-tooltip />
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatQueueTime(row.createdAt) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="上传中" name="running">
        <el-table :data="status.running || []" height="260" empty-text="暂无上传中任务">
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="fileName" label="文件" min-width="220" show-overflow-tooltip />
          <el-table-column label="大小" width="100">
            <template #default="{ row }">{{ formatBytes(row.fileSize) }}</template>
          </el-table-column>
          <el-table-column prop="uploadLine" label="线路" width="120" />
          <el-table-column label="重试" width="80">
            <template #default="{ row }">{{ row.uploadRetryCount || 0 }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="已完成" name="completed">
        <el-table :data="status.completed || []" height="260" empty-text="暂无完成任务">
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="fileName" label="文件" min-width="220" show-overflow-tooltip />
          <el-table-column label="大小" width="100">
            <template #default="{ row }">{{ formatBytes(row.fileSize) }}</template>
          </el-table-column>
          <el-table-column prop="uploadLine" label="线路" width="120" />
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatQueueTime(row.createdAt) }}</template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </el-card>
</template>

<script setup>
import { ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'

defineEmits(['refresh'])

defineProps({
  status: {
    type: Object,
    default: () => ({
      counts: { pending: 0, running: 0, completed: 0 },
      pending: [],
      running: [],
      completed: []
    })
  },
  loading: {
    type: Boolean,
    default: false
  }
})

const activeTab = ref('pending')

const formatBytes = (bytes) => {
  if (!bytes) return '-'
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${Math.round(bytes / 1024)} KB`
}

const formatQueueTime = (value) => {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN')
}
</script>

<style scoped lang="scss">
.queue-card {
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

.queue-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
}

.queue-summary-item {
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-medium);
  padding: var(--spacing-md);
  background: var(--bg-color-secondary);
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.queue-number {
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-bold);
  color: var(--text-color-primary);
}

.queue-label {
  font-size: var(--font-size-sm);
  color: var(--text-color-secondary);
}

@media (max-width: 480px) {
  .card-header {
    flex-direction: column;
    gap: var(--spacing-sm);
    align-items: flex-start;

    :deep(.el-button) {
      width: 100%;
    }
  }
}
</style>
