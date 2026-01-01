<template>
  <el-dialog
    :model-value="visible"
    title="分P详情"
    width="900px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <!-- 整体上传进度 -->
    <div v-if="uploadProgress && uploadProgress.activeCount > 0" class="upload-progress-section">
      <div class="progress-title">整体上传进度</div>
      <el-progress
        :percentage="uploadProgress.overallPercent"
        :status="getProgressStatus(uploadProgress.overallPercent)"
        :stroke-width="16"
      >
        <span class="progress-text">{{ uploadProgress.activeCount }} 个分P上传中</span>
      </el-progress>
    </div>

    <el-table :data="parts" v-loading="loading" style="margin-top: 15px;">
      <el-table-column prop="partIndex" label="分P序号" width="100" />
      <el-table-column prop="title" label="标题" min-width="200" />
      <el-table-column label="上传状态" width="120">
        <template #default="{ row }">
          <template v-if="getPartProgress(row.id)">
            <el-tag v-if="getPartProgress(row.id).state === 'SUCCESS'" type="success">已上传</el-tag>
            <el-tag v-else-if="getPartProgress(row.id).state === 'UPLOADING'" type="warning">上传中</el-tag>
            <el-tag v-else-if="getPartProgress(row.id).state === 'FAILED'" type="danger">失败</el-tag>
            <el-tag v-else-if="getPartProgress(row.id).state === 'RETRY_WAIT'" type="info">等待重试</el-tag>
            <el-tag v-else type="info">{{ getPartProgress(row.id).state }}</el-tag>
          </template>
          <template v-else>
            <el-popover
              v-if="row.uploadErrorMsg"
              placement="top"
              :width="400"
              trigger="hover"
            >
              <template #reference>
                <el-tag type="danger">失败</el-tag>
              </template>
              <div>
                <div style="font-weight: bold; margin-bottom: 8px;">上传错误信息：</div>
                <div style="color: #e6a23c;">{{ row.uploadErrorMsg }}</div>
                <div v-if="row.uploadRetryCount" style="margin-top: 8px; font-size: 12px; color: #999;">
                  已重试: {{ row.uploadRetryCount }} 次
                </div>
              </div>
            </el-popover>
            <el-tag v-else-if="row.upload" type="success">已上传</el-tag>
            <el-tag v-else-if="row.uploading" type="warning">上传中</el-tag>
            <el-tag v-else type="info">未上传</el-tag>
          </template>
        </template>
      </el-table-column>
      <el-table-column label="上传进度" width="280">
        <template #default="{ row }">
          <div v-if="getPartProgress(row.id)" class="part-progress">
            <el-progress
              :percentage="getPartProgress(row.id).percent"
              :status="getPartProgressStatus(getPartProgress(row.id).state)"
              :stroke-width="10"
            />
            <div class="progress-info">
              <span class="progress-chunks">{{ getPartProgress(row.id).chunkDone }}/{{ getPartProgress(row.id).chunkTotal }}</span>
              <span v-if="getRemainingTime(row.id)" class="remaining-time">{{ getRemainingTime(row.id) }}</span>
              <span v-if="getPartProgress(row.id).stateMsg" class="state-msg">{{ getPartProgress(row.id).stateMsg }}</span>
            </div>
          </div>
          <span v-else-if="row.upload">-</span>
          <span v-else>等待上传</span>
        </template>
      </el-table-column>
      <el-table-column prop="uploadLine" label="上传线路" width="140">
        <template #default="{ row }">
          <el-tag v-if="row.uploadLine" size="small" type="info">{{ row.uploadLine }}</el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
    </el-table>
  </el-dialog>
</template>

<script setup>
const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  parts: {
    type: Array,
    default: () => []
  },
  loading: {
    type: Boolean,
    default: false
  },
  uploadProgress: {
    type: Object,
    default: null
  },
  speedTracking: {
    type: Object,
    default: () => ({})
  }
})

defineEmits(['update:visible'])

const getPartProgress = (partId) => {
  if (!props.uploadProgress || !props.uploadProgress.items) return null
  return props.uploadProgress.items.find(item => item.partId === partId)
}

const getProgressStatus = (percent) => {
  if (percent >= 100) return 'success'
  if (percent >= 50) return 'warning'
  return null
}

const getPartProgressStatus = (state) => {
  if (state === 'FAILED') return 'exception'
  if (state === 'SUCCESS') return 'success'
  if (state === 'RETRY_WAIT') return 'warning'
  return null
}

const getRemainingTime = (partId) => {
  const progress = getPartProgress(partId)
  if (!progress || progress.state !== 'UPLOADING') return null
  
  const track = props.speedTracking[partId]
  if (!track || !track.samples || track.samples.length < 2) {
    return '正在计算...'
  }
  
  const now = Date.now()
  const recentSamples = track.samples.filter(s => (now - s.time) < 30000)
  
  if (recentSamples.length === 0) return null
  
  const weights = recentSamples.map((s, i) => i + 1)
  const totalWeight = weights.reduce((a, b) => a + b, 0)
  const avgSpeed = recentSamples.reduce((sum, s, i) => sum + s.speed * weights[i], 0) / totalWeight
  
  if (avgSpeed <= 0) return null
  
  const remainingChunks = progress.chunkTotal - progress.chunkDone
  const remainingSeconds = remainingChunks / avgSpeed
  
  const speedMBps = (avgSpeed * 5).toFixed(1)
  
  return formatRemainingTime(remainingSeconds, speedMBps)
}

const formatRemainingTime = (seconds, speedMBps) => {
  if (!isFinite(seconds) || seconds <= 0) return null
  
  let timeStr = ''
  if (seconds > 3600) {
    const hours = Math.floor(seconds / 3600)
    const mins = Math.floor((seconds % 3600) / 60)
    timeStr = `约${hours}小时${mins}分钟`
  } else if (seconds > 60) {
    const mins = Math.ceil(seconds / 60)
    timeStr = `约${mins}分钟`
  } else {
    const secs = Math.ceil(seconds)
    timeStr = `约${secs}秒`
  }
  
  if (speedMBps && Number(speedMBps) > 0) {
    return `${timeStr} (${speedMBps}MB/s)`
  }
  return timeStr
}
</script>

<style scoped>
.upload-progress-section {
  margin-bottom: 20px;
  padding: 15px;
  background: #f5f7fa;
  border-radius: 8px;
}

.progress-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 12px;
  color: #303133;
}

.part-progress {
  width: 100%;
}

.progress-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 4px;
  font-size: 12px;
  color: #909399;
}

.progress-chunks {
  font-family: 'Courier New', monospace;
}

.remaining-time {
  color: #67c23a;
  font-weight: 500;
}

.state-msg {
  color: #909399;
  font-size: 11px;
}
</style>
