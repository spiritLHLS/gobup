<template>
  <div class="history-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>录制历史</span>
          <div class="header-actions">
            <el-button type="danger" size="small" plain @click="showCleanDialog = true">清理旧记录</el-button>
            <div class="search-box">
              <el-input
                v-model="searchParams.roomId"
                placeholder="房间ID"
                clearable
                style="width: 150px; margin-right: 10px"
              />
              <el-input
                v-model="searchParams.bvId"
                placeholder="BV号"
                clearable
                style="width: 200px; margin-right: 10px"
              />
              <el-button type="primary" @click="handleSearch">搜索</el-button>
            </div>
          </div>
        </div>
      </template>

      <el-table :data="histories" style="width: 100%" v-loading="loading" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="55" />
        <el-table-column prop="roomId" label="房间ID" width="100" />
        <el-table-column prop="title" label="标题" min-width="200" />
        <el-table-column prop="name" label="主播" width="120" />
        <el-table-column label="上传状态" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.bvId" type="success">已发布</el-tag>
            <el-tag v-else-if="row.uploadStatus === 2" type="warning">已上传</el-tag>
            <el-tag v-else-if="row.uploadStatus === 1" type="info">上传中</el-tag>
            <el-tag v-else type="info">未上传</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="bvId" label="BV号" width="150">
          <template #default="{ row }">
            <a
              v-if="row.bvId"
              :href="`https://www.bilibili.com/video/${row.bvId}`"
              target="_blank"
              style="color: #1890ff"
            >
              {{ row.bvId }}
            </a>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="分P数量" width="100">
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              @click="showParts(row)"
            >
              {{ row.partCount || 0 }} P
            </el-button>
          </template>
        </el-table-column>
        <el-table-column label="视频状态" width="120">
          <template #default="{ row }">
            <el-tooltip v-if="row.videoState >= 0" :content="row.videoStateDesc || ''" placement="top">
              <el-tag v-if="row.videoState === 1" type="success">已通过</el-tag>
              <el-tag v-else-if="row.videoState === 0" type="warning">审核中</el-tag>
              <el-tag v-else-if="row.videoState < 0" type="danger">未通过</el-tag>
              <el-tag v-else type="info">未知</el-tag>
            </el-tooltip>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="弹幕" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.danmakuSent" type="success">{{ row.danmakuCount || 0 }}</el-tag>
            <el-tag v-else-if="row.bvId" type="info">未发送</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="startTime" label="开始时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.startTime) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="320" fixed="right">
          <template #default="{ row }">
            <el-dropdown size="small">
              <el-button size="small" type="primary">
                操作 <el-icon class="el-icon--right"><arrow-down /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item :disabled="!!row.bvId || row.uploadStatus !== 2" @click="handlePublish(row)">
                    发布视频
                  </el-dropdown-item>
                  <el-dropdown-item :disabled="!row.bvId || row.danmakuSent" @click="handleSendDanmaku(row)">
                    发送弹幕
                  </el-dropdown-item>
                  <el-dropdown-item :disabled="!row.bvId" @click="handleSyncVideo(row)">
                    同步视频信息
                  </el-dropdown-item>
                  <el-dropdown-item :disabled="!row.publish || row.filesMoved" @click="handleMoveFiles(row)">
                    移动文件
                  </el-dropdown-item>
                  <el-dropdown-item divided @click="handleDelete(row)">
                    删除记录
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>

      <!-- 批量操作栏 -->
      <div v-if="selectedHistories.length > 0" class="batch-actions">
        <span class="batch-info">已选择 {{ selectedHistories.length }} 项</span>
        <el-button size="small" @click="handleBatchUpdate('publish')">批量标记已发布</el-button>
        <el-button size="small" @click="handleBatchUpdate('unpublish')">批量取消发布</el-button>
        <el-button size="small" type="danger" @click="handleBatchDelete">批量删除</el-button>
      </div>

      <div class="pagination">
        <el-pagination
          v-model:current-page="searchParams.page"
          v-model:page-size="searchParams.pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSearch"
          @current-change="handleSearch"
        />
      </div>
    </el-card>

    <!-- 分P详情对话框 -->
    <el-dialog
      v-model="partsDialogVisible"
      title="分P详情"
      width="900px"
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

      <el-table :data="parts" v-loading="partsLoading" style="margin-top: 15px;">
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
              <el-tag v-if="row.uploadStatus === 2" type="success">已上传</el-tag>
              <el-tag v-else-if="row.uploadStatus === 1" type="warning">上传中</el-tag>
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
            <span v-else-if="row.uploadStatus === 2">-</span>
            <span v-else>等待上传</span>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 清理旧记录对话框 -->
    <el-dialog v-model="showCleanDialog" title="清理旧记录" width="400px">
      <el-form>
        <el-form-item label="保留天数">
          <el-input-number v-model="cleanDays" :min="7" :max="365" />
          <div style="margin-top: 8px; font-size: 12px; color: #999;">
            将删除{{ cleanDays }}天前的未上传、未发布记录
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCleanDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCleanOld">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage, ElMessageBox, ElLoading } from 'element-plus'
import { ArrowDown } from '@element-plus/icons-vue'
import { historyAPI } from '@/api'
import axios from 'axios'

const histories = ref([])
const loading = ref(false)
const total = ref(0)
const selectedHistories = ref([])
const showCleanDialog = ref(false)
const cleanDays = ref(30)


const searchParams = ref({
  page: 1,
  pageSize: 10,
  roomId: '',
  bvId: ''
})

const partsDialogVisible = ref(false)
const parts = ref([])
const partsLoading = ref(false)
const currentHistoryId = ref(null)
const uploadProgress = ref(null)
const progressTimer = ref(null)
const speedTracking = ref({})

const fetchHistories = async () => {
  loading.value = true
  try {
    const data = await historyAPI.list(searchParams.value)
    histories.value = data?.list || []
    total.value = data?.total || 0
  } catch (error) {
    console.error('获取历史记录失败:', error)
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  searchParams.value.page = 1
  fetchHistories()
}

const handlePublish = async (row) => {
  try {
    await ElMessageBox.confirm('确定要发布这个视频到B站吗？', '提示', {
      type: 'warning'
    })
    await historyAPI.publish(row.id)
    ElMessage.success('发布任务已提交')
    fetchHistories()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('发布失败:', error)
    }
  }
}

// 发送弹幕
const handleSendDanmaku = async (row) => {
  try {
    await ElMessageBox.confirm('确定要将直播弹幕转移到视频吗？此操作可能需要较长时间。', '发送弹幕', {
      type: 'warning'
    })
    
    // 获取用户列表
    const userResponse = await axios.get('/api/biliUser/list')
    const users = userResponse.data || []
    
    if (users.length === 0) {
      ElMessage.warning('请先添加B站用户')
      return
    }
    
    // 使用房间配置的用户ID，或第一个用户
    const userId = users[0].id
    
    const loadingInstance = ElLoading.service({ text: '弹幕发送中，请稍候...' })
    try {
      await axios.post(`/api/history/sendDanmaku/${row.id}`, { userId })
      ElMessage.success('弹幕发送成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('发送弹幕失败:', error)
      ElMessage.error(error.response?.data?.msg || '发送弹幕失败')
    }
  }
}

// 同步视频信息
const handleSyncVideo = async (row) => {
  try {
    const loadingInstance = ElLoading.service({ text: '同步中...' })
    try {
      await axios.post(`/api/history/syncVideo/${row.id}`)
      ElMessage.success('视频信息同步成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    console.error('同步视频信息失败:', error)
    ElMessage.error(error.response?.data?.msg || '同步失败')
  }
}

// 移动文件
const handleMoveFiles = async (row) => {
  try {
    await ElMessageBox.confirm('确定要移动此历史记录的所有相关文件吗？', '移动文件', {
      type: 'warning'
    })
    
    const loadingInstance = ElLoading.service({ text: '文件移动中...' })
    try {
      await axios.post(`/api/history/moveFiles/${row.id}`)
      ElMessage.success('文件移动成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('移动文件失败:', error)
      ElMessage.error(error.response?.data?.msg || '移动失败')
    }
  }
}

// 批量操作
const handleSelectionChange = (selection) => {
  selectedHistories.value = selection
}

const handleBatchUpdate = async (status) => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  const statusText = {
    'publish': '标记已发布',
    'unpublish': '取消发布',
    'upload': '标记待上传',
    'cancel': '取消上传'
  }

  try {
    await ElMessageBox.confirm(`确定要${statusText[status]}选中的 ${selectedHistories.value.length} 项吗？`, '批量操作', {
      type: 'warning'
    })

    const ids = selectedHistories.value.map(h => h.id)
    await axios.post('/api/history/batchUpdate', { ids, status })
    
    ElMessage.success('批量操作成功')
    fetchHistories()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量操作失败:', error)
      ElMessage.error('操作失败')
    }
  }
}

const handleBatchDelete = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(`确定要删除选中的 ${selectedHistories.value.length} 项吗？`, '批量删除', {
      type: 'warning'
    })

    const ids = selectedHistories.value.map(h => h.id)
    await axios.post('/api/history/batchDelete', { ids })
    
    ElMessage.success('批量删除成功')
    fetchHistories()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量删除失败:', error)
      ElMessage.error('删除失败')
    }
  }
}

const handleCleanOld = async () => {
  try {
    const result = await axios.post('/api/history/cleanOld', { days: cleanDays.value })
    ElMessage.success(`已清理 ${result.data.deletedCount} 条旧记录`)
    showCleanDialog.value = false
    fetchHistories()
  } catch (error) {
    console.error('清理失败:', error)
    ElMessage.error('清理失败')
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这条记录吗？', '提示', {
      type: 'warning'
    })
    await historyAPI.delete(row.id)
    ElMessage.success('删除成功')
    fetchHistories()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('删除失败:', error)
    }
  }
}

const showParts = async (row) => {
  partsDialogVisible.value = true
  partsLoading.value = true
  currentHistoryId.value = row.id
  
  try {
    const data = await historyAPI.parts(row.id)
    parts.value = data || []
    
    // 开始轮询进度
    startProgressPolling(row.id)
  } catch (error) {
    console.error('获取分P详情失败:', error)
  } finally {
    partsLoading.value = false
  }
}

// 开始轮询上传进度
const startProgressPolling = async (historyId) => {
  // 清除之前的定时器
  stopProgressPolling()
  
  // 立即获取一次进度
  await fetchProgress(historyId)
  
  // 如果有活跃的上传，启动定时轮询
  if (uploadProgress.value && uploadProgress.value.activeCount > 0) {
    progressTimer.value = setInterval(() => {
      fetchProgress(historyId)
    }, 1500)
  }
}

// 停止轮询
const stopProgressPolling = () => {
  if (progressTimer.value) {
    clearInterval(progressTimer.value)
    progressTimer.value = null
  }
  speedTracking.value = {}
}

// 获取进度
const fetchProgress = async (historyId) => {
  try {
    const response = await axios.get(`/api/progress/history/${historyId}`)
    uploadProgress.value = response.data
    
    // 更新速度追踪
    updateSpeedTracking(uploadProgress.value)
    
    // 如果没有活跃上传，停止轮询
    if (!uploadProgress.value || uploadProgress.value.activeCount === 0) {
      stopProgressPolling()
    }
  } catch (error) {
    console.error('获取进度失败:', error)
  }
}

// 更新速度追踪
const updateSpeedTracking = (progress) => {
  if (!progress || !progress.items) return
  
  const now = Date.now()
  progress.items.forEach(item => {
    if (item.state !== 'UPLOADING' || !item.chunkTotal) return
    
    const partId = item.partId
    if (!speedTracking.value[partId]) {
      speedTracking.value[partId] = {
        samples: [],
        lastChunkDone: item.chunkDone,
        lastTime: now,
        chunkTotal: item.chunkTotal
      }
    } else {
      const track = speedTracking.value[partId]
      const timeDiff = (now - track.lastTime) / 1000
      const chunkDiff = item.chunkDone - track.lastChunkDone
      
      if (timeDiff > 0 && chunkDiff > 0) {
        const speed = chunkDiff / timeDiff
        track.samples.push({ speed, time: now })
        
        if (track.samples.length > 10) {
          track.samples.shift()
        }
        
        track.lastChunkDone = item.chunkDone
        track.lastTime = now
        track.chunkTotal = item.chunkTotal
      }
    }
  })
}

// 获取分P进度
const getPartProgress = (partId) => {
  if (!uploadProgress.value || !uploadProgress.value.items) return null
  return uploadProgress.value.items.find(item => item.partId === partId)
}

// 获取进度状态
const getProgressStatus = (percent) => {
  if (percent >= 100) return 'success'
  if (percent >= 50) return 'warning'
  return null
}

// 获取分P进度状态
const getPartProgressStatus = (state) => {
  if (state === 'FAILED') return 'exception'
  if (state === 'SUCCESS') return 'success'
  if (state === 'RETRY_WAIT') return 'warning'
  return null
}

// 计算剩余时间
const getRemainingTime = (partId) => {
  const progress = getPartProgress(partId)
  if (!progress || progress.state !== 'UPLOADING') return null
  
  const track = speedTracking.value[partId]
  if (!track || !track.samples || track.samples.length < 2) {
    return '正在计算...'
  }
  
  const now = Date.now()
  const recentSamples = track.samples.filter(s => (now - s.time) < 30000)
  
  if (recentSamples.length === 0) return null
  
  // 加权平均速度
  const weights = recentSamples.map((s, i) => i + 1)
  const totalWeight = weights.reduce((a, b) => a + b, 0)
  const avgSpeed = recentSamples.reduce((sum, s, i) => sum + s.speed * weights[i], 0) / totalWeight
  
  if (avgSpeed <= 0) return null
  
  const remainingChunks = progress.chunkTotal - progress.chunkDone
  const remainingSeconds = remainingChunks / avgSpeed
  
  const speedMBps = (avgSpeed * 5).toFixed(1)
  
  return formatRemainingTime(remainingSeconds, speedMBps)
}

// 格式化剩余时间
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

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString('zh-CN')
}

// 监听对话框关闭
watch(partsDialogVisible, (newVal) => {
  if (!newVal) {
    stopProgressPolling()
    uploadProgress.value = null
    currentHistoryId.value = null
  }
})

onMounted(() => {
  fetchHistories()
})

onUnmounted(() => {
  stopProgressPolling()
})
</script>

<style scoped>
.history-container {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-box {
  display: flex;
  align-items: center;
}

.batch-actions {
  padding: 12px;
  background: #f5f7fa;
  border-radius: 4px;
  margin-top: 15px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.batch-info {
  font-size: 14px;
  color: #606266;
  margin-right: 10px;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

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

/* Table styles */
:deep(.el-table__header th) {
  background: #f5f7fa;
  color: #606266;
  font-weight: 600;
}

:deep(.el-table__row):hover {
  background: rgba(24, 144, 255, 0.04);
}
</style>
