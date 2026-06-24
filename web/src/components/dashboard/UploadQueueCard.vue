<template>
  <el-card class="queue-card" v-loading="loading">
    <template #header>
      <div class="card-header">
        <span>上传队列</span>
        <div class="queue-actions">
          <el-button size="default" @click="$emit('refresh')" :icon="Refresh">刷新</el-button>
          <el-button size="default" @click="handleBatch('pause')" :icon="VideoPause">全部暂停</el-button>
          <el-button size="default" @click="handleBatch('resume')" :icon="VideoPlay">全部恢复</el-button>
          <el-button size="default" type="danger" plain @click="handleBatch('cancel')" :icon="Close">全部取消</el-button>
        </div>
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
        <span class="queue-number">{{ status.counts?.cooldown || 0 }}</span>
        <span class="queue-label">冷却中</span>
      </div>
      <div class="queue-summary-item">
        <span class="queue-number">{{ status.counts?.paused || 0 }}</span>
        <span class="queue-label">已暂停</span>
      </div>
      <div class="queue-summary-item">
        <span class="queue-number">{{ status.counts?.cancelled || 0 }}</span>
        <span class="queue-label">已取消</span>
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
          <el-table-column label="类型" width="110">
            <template #default="{ row }">
              <el-tag v-if="row.uploadErrorType" size="small" :type="errorTagType(row.uploadErrorType)">
                {{ errorTypeLabel(row.uploadErrorType) }}
              </el-tag>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column prop="uploadErrorMsg" label="最近错误" min-width="180" show-overflow-tooltip />
          <el-table-column label="创建时间" width="170">
            <template #default="{ row }">{{ formatQueueTime(row.createdAt) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <el-button size="small" :icon="InfoFilled" title="详情" aria-label="查看任务详情" @click="showDetail(row)" />
              <el-button size="small" :icon="VideoPause" title="暂停" aria-label="暂停任务" @click="handleAction('pause', row)" />
              <el-button size="small" type="danger" plain :icon="Close" title="取消" aria-label="取消任务" @click="handleAction('cancel', row)" />
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="冷却中" name="cooldown">
        <el-table :data="status.cooldown || []" height="260" empty-text="暂无冷却任务">
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="fileName" label="文件" min-width="220" show-overflow-tooltip />
          <el-table-column label="类型" width="110">
            <template #default="{ row }">
              <el-tag v-if="row.uploadErrorType" size="small" :type="errorTagType(row.uploadErrorType)">
                {{ errorTypeLabel(row.uploadErrorType) }}
              </el-tag>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column label="冷却至" width="170">
            <template #default="{ row }">{{ formatQueueTime(row.rateLimitCooldownAt) }}</template>
          </el-table-column>
          <el-table-column prop="uploadErrorMsg" label="最近错误" min-width="220" show-overflow-tooltip />
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <el-button size="small" :icon="InfoFilled" title="详情" aria-label="查看任务详情" @click="showDetail(row)" />
              <el-button size="small" type="primary" :icon="RefreshRight" title="立即重试" aria-label="立即重试任务" @click="handleAction('retry', row)" />
              <el-button size="small" :icon="VideoPause" title="暂停" aria-label="暂停任务" @click="handleAction('pause', row)" />
              <el-button size="small" type="danger" plain :icon="Close" title="取消" aria-label="取消任务" @click="handleAction('cancel', row)" />
            </template>
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
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button size="small" :icon="InfoFilled" title="详情" aria-label="查看任务详情" @click="showDetail(row)" />
              <el-button size="small" :icon="VideoPause" title="暂停" aria-label="暂停任务" @click="handleAction('pause', row)" />
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="已暂停" name="paused">
        <el-table :data="status.paused || []" height="260" empty-text="暂无暂停任务">
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="fileName" label="文件" min-width="220" show-overflow-tooltip />
          <el-table-column label="大小" width="100">
            <template #default="{ row }">{{ formatBytes(row.fileSize) }}</template>
          </el-table-column>
          <el-table-column prop="uploadErrorMsg" label="状态" min-width="180" show-overflow-tooltip />
          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <el-button size="small" :icon="InfoFilled" title="详情" aria-label="查看任务详情" @click="showDetail(row)" />
              <el-button size="small" type="primary" :icon="VideoPlay" title="恢复" aria-label="恢复任务" @click="handleAction('resume', row)" />
              <el-button size="small" type="danger" plain :icon="Close" title="取消" aria-label="取消任务" @click="handleAction('cancel', row)" />
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="已取消" name="cancelled">
        <el-table :data="status.cancelled || []" height="260" empty-text="暂无取消任务">
          <el-table-column prop="roomId" label="房间" width="100" />
          <el-table-column prop="fileName" label="文件" min-width="220" show-overflow-tooltip />
          <el-table-column label="大小" width="100">
            <template #default="{ row }">{{ formatBytes(row.fileSize) }}</template>
          </el-table-column>
          <el-table-column prop="uploadErrorMsg" label="状态" min-width="180" show-overflow-tooltip />
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button size="small" :icon="InfoFilled" title="详情" aria-label="查看任务详情" @click="showDetail(row)" />
              <el-button size="small" type="primary" :icon="RefreshRight" title="重试" aria-label="重试任务" @click="handleAction('retry', row)" />
            </template>
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
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <el-button size="small" :icon="InfoFilled" title="详情" aria-label="查看任务详情" @click="showDetail(row)" />
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="detailVisible" title="任务详情" width="680px" class="queue-detail-dialog">
      <el-descriptions v-if="selectedPart" :column="2" border size="small">
        <el-descriptions-item label="状态">
          <el-tag :type="statusTagType(selectedPart.status)">
            {{ statusLabel(selectedPart.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="房间">{{ selectedPart.roomId || '-' }}</el-descriptions-item>
        <el-descriptions-item label="标题" :span="2">{{ selectedPart.title || selectedPart.liveTitle || '-' }}</el-descriptions-item>
        <el-descriptions-item label="文件" :span="2">{{ selectedPart.fileName || '-' }}</el-descriptions-item>
        <el-descriptions-item label="路径" :span="2">{{ selectedPart.filePath || '-' }}</el-descriptions-item>
        <el-descriptions-item label="大小">{{ formatBytes(selectedPart.fileSize) }}</el-descriptions-item>
        <el-descriptions-item label="时长">{{ formatDuration(selectedPart.duration) }}</el-descriptions-item>
        <el-descriptions-item label="分P">{{ selectedPart.page || '-' }}</el-descriptions-item>
        <el-descriptions-item label="CID">{{ selectedPart.cid || '-' }}</el-descriptions-item>
        <el-descriptions-item label="开始">{{ formatQueueTime(selectedPart.startTime) }}</el-descriptions-item>
        <el-descriptions-item label="结束">{{ formatQueueTime(selectedPart.endTime) }}</el-descriptions-item>
        <el-descriptions-item label="线路">{{ selectedPart.uploadLine || '-' }}</el-descriptions-item>
        <el-descriptions-item label="重试">{{ selectedPart.uploadRetryCount || 0 }}</el-descriptions-item>
        <el-descriptions-item label="错误类型">
          <el-tag v-if="selectedPart.uploadErrorType" size="small" :type="errorTagType(selectedPart.uploadErrorType)">
            {{ errorTypeLabel(selectedPart.uploadErrorType) }}
          </el-tag>
          <span v-else>-</span>
        </el-descriptions-item>
        <el-descriptions-item label="冷却至">{{ formatQueueTime(selectedPart.rateLimitCooldownAt) }}</el-descriptions-item>
        <el-descriptions-item label="临时文件">{{ formatBool(selectedPart.isTempFile) }}</el-descriptions-item>
        <el-descriptions-item label="来源分P">{{ selectedPart.sourcePartId || '-' }}</el-descriptions-item>
        <el-descriptions-item label="临时类型">{{ selectedPart.tempFileType || '-' }}</el-descriptions-item>
        <el-descriptions-item label="已移动">{{ formatBool(selectedPart.fileMoved) }}</el-descriptions-item>
        <el-descriptions-item label="已删除">{{ formatBool(selectedPart.fileDelete) }}</el-descriptions-item>
        <el-descriptions-item label="最近错误" :span="2">{{ selectedPart.uploadErrorMsg || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, InfoFilled, Refresh, RefreshRight, VideoPause, VideoPlay } from '@element-plus/icons-vue'
import { queueAPI } from '@/api'

const emit = defineEmits(['refresh'])

defineProps({
  status: {
    type: Object,
    default: () => ({
      counts: { pending: 0, running: 0, cooldown: 0, paused: 0, cancelled: 0, completed: 0 },
      pending: [],
      running: [],
      cooldown: [],
      paused: [],
      cancelled: [],
      completed: []
    })
  },
  loading: {
    type: Boolean,
    default: false
  }
})

const activeTab = ref('pending')
const detailVisible = ref(false)
const selectedPart = ref(null)

const formatBytes = (bytes) => {
  if (!bytes) return '-'
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${Math.round(bytes / 1024)} KB`
}

const formatQueueTime = (value) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN')
}

const formatDuration = (seconds) => {
  if (!seconds) return '-'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

const formatBool = (value) => (value ? '是' : '否')

const errorTypeLabel = (type) => {
  const labels = {
    network: '网络',
    rate_limit: '限流',
    auth: '鉴权',
    file: '文件',
    transcode: '转码',
    window: '窗口',
    user: '用户',
    unknown: '未知'
  }
  return labels[type] || type || '-'
}

const errorTagType = (type) => {
  const types = {
    rate_limit: 'danger',
    auth: 'danger',
    file: 'warning',
    transcode: 'warning',
    network: 'info',
    window: 'warning',
    user: 'info'
  }
  return types[type] || 'info'
}

const statusLabel = (status) => {
  const labels = {
    pending: '待上传',
    running: '上传中',
    paused: '已暂停',
    cancelled: '已取消',
    completed: '已完成',
    cooldown: '冷却中'
  }
  return labels[status] || status || '-'
}

const statusTagType = (status) => {
  const types = {
    running: 'primary',
    paused: 'warning',
    cancelled: 'danger',
    completed: 'success',
    cooldown: 'warning'
  }
  return types[status] || 'info'
}

const showDetail = (row) => {
  selectedPart.value = row
  detailVisible.value = true
}

const handleAction = async (action, row) => {
  const calls = {
    pause: queueAPI.pauseUploadPart,
    resume: queueAPI.resumeUploadPart,
    cancel: queueAPI.cancelUploadPart,
    retry: queueAPI.retryUploadPart
  }
  const call = calls[action]
  if (!call || !row?.id) return

  if (action === 'cancel') {
    try {
      await ElMessageBox.confirm('确定取消这个上传任务吗？', '确认取消', { type: 'warning' })
    } catch {
      return
    }
  }

  const result = await call(row.id)
  if (result?.type === 'error') {
    ElMessage.error(result.msg || '操作失败')
  } else if (result?.type === 'warning') {
    ElMessage.warning(result.msg || '操作已提交')
  } else {
    ElMessage.success(result?.msg || '操作成功')
  }
  emit('refresh')
}

const handleBatch = async (action) => {
  const calls = {
    pause: queueAPI.pauseAllUploads,
    resume: queueAPI.resumeAllUploads,
    cancel: queueAPI.cancelAllUploads
  }
  const call = calls[action]
  if (!call) return

  if (action === 'cancel') {
    try {
      await ElMessageBox.confirm('确定取消所有待上传任务吗？', '确认批量取消', { type: 'warning' })
    } catch {
      return
    }
  }

  const result = await call()
  if (result?.type === 'error') {
    ElMessage.error(result.msg || '操作失败')
  } else {
    ElMessage.success(result?.msg || '操作成功')
  }
  emit('refresh')
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

.queue-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  justify-content: flex-end;
}

.queue-summary {
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

:deep(.queue-detail-dialog) {
  max-width: calc(100vw - 32px);
}

:deep(.queue-detail-dialog .el-dialog__body) {
  overflow-x: auto;
}

@media (max-width: 480px) {
  .card-header {
    flex-direction: column;
    gap: var(--spacing-sm);
    align-items: flex-start;

    .queue-actions,
    :deep(.el-button) {
      width: 100%;
    }
  }
}
</style>
