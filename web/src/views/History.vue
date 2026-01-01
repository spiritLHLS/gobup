<template>
  <div class="history-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>录制历史</span>
          <div class="header-actions">
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
        <el-table-column label="上传状态" width="200">
          <template #default="{ row }">
            <div v-if="row.uploadStatus === 1 && !row.bvId && getHistoryProgress(row.id)">
              <el-progress
                :percentage="getHistoryUploadPercent(row.id)"
                :status="getHistoryUploadPercent(row.id) >= 100 ? 'success' : null"
                :stroke-width="8"
              >
                <span style="font-size: 12px;">{{ getHistoryUploadPercent(row.id) }}%</span>
              </el-progress>
              <div style="font-size: 11px; color: #999; margin-top: 2px;">
                {{ getHistoryProgress(row.id).activeCount || 0 }} 个分P上传中
              </div>
            </div>
            <el-tag v-else-if="row.bvId" type="success">已发布</el-tag>
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
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="showActionsDialog(row)">
              操作
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 批量操作栏 -->
      <BatchActions
        :selected-histories="selectedHistories"
        @upload="handleBatchUpload"
        @publish="handleBatchPublish"
        @send-danmaku="handleBatchSendDanmaku"
        @sync-video="handleBatchSyncVideo"
        @move-files="handleBatchMoveFiles"
        @reset-status="handleBatchResetStatus"
        @delete-only="handleBatchDeleteOnly"
        @delete-with-files="handleBatchDeleteWithFiles"
      />

      <div class="pagination">
        <el-pagination
          v-model:current-page="searchParams.page"
          v-model:page-size="searchParams.pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>

    <!-- 分P详情对话框 -->
    <PartsDialog
      v-model:visible="partsDialogVisible"
      :parts="parts"
      :loading="partsLoading"
      :upload-progress="uploadProgress"
      :speed-tracking="speedTracking"
    />

    <!-- 操作对话框 -->
    <ActionsDialog
      v-model:visible="actionsDialogVisible"
      :history="currentHistory"
      @upload="handleUploadInDialog"
      @publish="handlePublishInDialog"
      @send-danmaku="handleSendDanmakuInDialog"
      @sync-video="handleSyncVideoInDialog"
      @move-files="handleMoveFilesInDialog"
      @reset-status="handleResetStatus"
      @delete-only="handleDeleteOnly"
      @delete-with-files="handleDeleteWithFiles"
    />

    <!-- 重置状态对话框 -->
    <ResetStatusDialog
      v-model:visible="resetDialogVisible"
      :options="resetOptions"
      :is-batch="false"
      @confirm="confirmReset"
    />

    <!-- 批量重置状态对话框 -->
    <ResetStatusDialog
      v-model:visible="batchResetDialogVisible"
      :options="batchResetOptions"
      :is-batch="true"
      @confirm="confirmBatchReset"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox, ElLoading } from 'element-plus'
import { historyAPI } from '@/api'
import axios from 'axios'
import BatchActions from '@/components/history/BatchActions.vue'
import PartsDialog from '@/components/history/PartsDialog.vue'
import ActionsDialog from '@/components/history/ActionsDialog.vue'
import ResetStatusDialog from '@/components/history/ResetStatusDialog.vue'
import { useHistoryProgress, useHistoryOperations } from '@/composables/useHistory'

const histories = ref([])
const loading = ref(false)
const total = ref(0)
const selectedHistories = ref([])

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
const actionsDialogVisible = ref(false)
const currentHistory = ref(null)
const resetDialogVisible = ref(false)
const resetOptions = ref({
  upload: true,
  publish: true,
  danmaku: true,
  files: true
})
const batchResetDialogVisible = ref(false)
const batchResetOptions = ref({
  upload: true,
  publish: true,
  danmaku: true,
  files: true
})

// 使用composables
const {
  uploadProgress,
  speedTracking,
  startProgressPolling,
  stopProgressPolling,
  getHistoryProgress,
  getHistoryUploadPercent,
  fetchHistoryProgress,
  startHistoryProgressPolling,
  stopHistoryProgressPolling
} = useHistoryProgress()

const {
  handleUpload,
  handlePublish,
  handleSendDanmaku,
  handleSyncVideo,
  handleMoveFiles,
  handleResetStatus: resetHistoryStatus,
  handleDeleteOnly: deleteHistoryOnly,
  handleDeleteWithFiles: deleteHistoryWithFiles
} = useHistoryOperations()

const fetchHistories = async () => {
  loading.value = true
  try {
    const data = await historyAPI.list(searchParams.value)
    histories.value = data?.list || []
    total.value = data?.total || 0
    
    const hasUploading = histories.value.some(h => h.uploadStatus === 1 && !h.bvId)
    if (hasUploading) {
      startHistoryProgressPolling()
      await fetchHistoryProgress(histories.value)
    } else {
      stopHistoryProgressPolling()
    }
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

const handlePageChange = () => {
  fetchHistories()
}

const handleSizeChange = () => {
  searchParams.value.page = 1
  fetchHistories()
}

const showActionsDialog = (row) => {
  currentHistory.value = row
  actionsDialogVisible.value = true
}

const handleUploadInDialog = async () => {
  await handleUpload(currentHistory.value, () => {
    fetchHistories()
    startHistoryProgressPolling()
  })
}

const handlePublishInDialog = async () => {
  await handlePublish(currentHistory.value, () => {
    actionsDialogVisible.value = false
    fetchHistories()
  })
}

const handleSendDanmakuInDialog = async () => {
  await handleSendDanmaku(currentHistory.value, () => {
    actionsDialogVisible.value = false
    fetchHistories()
  })
}

const handleSyncVideoInDialog = async () => {
  await handleSyncVideo(currentHistory.value, () => {
    actionsDialogVisible.value = false
    fetchHistories()
  })
}

const handleMoveFilesInDialog = async () => {
  await handleMoveFiles(currentHistory.value, () => {
    actionsDialogVisible.value = false
    fetchHistories()
  })
}

const handleResetStatus = () => {
  resetOptions.value = {
    upload: true,
    publish: true,
    danmaku: true,
    files: true
  }
  resetDialogVisible.value = true
}

const confirmReset = async (options) => {
  await resetHistoryStatus(currentHistory.value.id, options, () => {
    resetDialogVisible.value = false
    actionsDialogVisible.value = false
    fetchHistories()
  })
}

const handleDeleteOnly = async () => {
  await deleteHistoryOnly(currentHistory.value.id, () => {
    actionsDialogVisible.value = false
    fetchHistories()
  })
}

const handleDeleteWithFiles = async () => {
  await deleteHistoryWithFiles(currentHistory.value.id, () => {
    actionsDialogVisible.value = false
    fetchHistories()
  })
}

const handleSelectionChange = (selection) => {
  selectedHistories.value = selection
}

// 批量操作函数
const handleBatchUpload = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(`确定要批量上传选中的 ${selectedHistories.value.length} 项吗？`, '批量上传', {
      type: 'warning'
    })

    const userResponse = await axios.get('/api/biliUser/list')
    const users = userResponse.data || []
    
    if (users.length === 0) {
      ElMessage.warning('请先添加B站用户')
      return
    }
    
    const userId = users[0].id

    const loadingInstance = ElLoading.service({ text: '批量上传中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.post(`/api/history/upload/${history.id}`, { userId })
        } catch (error) {
          console.error(`上传历史记录${history.id}失败:`, error)
        }
      }
      ElMessage.success('批量上传任务已启动')
      startHistoryProgressPolling()
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量上传失败:', error)
      ElMessage.error('批量上传失败')
    }
  }
}

const handleBatchPublish = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(`确定要批量投稿选中的 ${selectedHistories.value.length} 项吗？`, '批量投稿', {
      type: 'warning'
    })

    const userResponse = await axios.get('/api/biliUser/list')
    const users = userResponse.data || []
    
    if (users.length === 0) {
      ElMessage.warning('请先添加B站用户')
      return
    }
    
    const userId = users[0].id

    const loadingInstance = ElLoading.service({ text: '批量投稿中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.post(`/api/history/publish/${history.id}`, { userId })
        } catch (error) {
          console.error(`投稿历史记录${history.id}失败:`, error)
        }
      }
      ElMessage.success('批量投稿任务已提交')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量投稿失败:', error)
      ElMessage.error('批量投稿失败')
    }
  }
}

const handleBatchSendDanmaku = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(`确定要批量发送弹幕到选中的 ${selectedHistories.value.length} 项吗？此操作可能需要较长时间。`, '批量发送弹幕', {
      type: 'warning'
    })

    const userResponse = await axios.get('/api/biliUser/list')
    const users = userResponse.data || []
    
    if (users.length === 0) {
      ElMessage.warning('请先添加B站用户')
      return
    }
    
    const userId = users[0].id

    const loadingInstance = ElLoading.service({ text: '批量发送弹幕中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.post(`/api/history/sendDanmaku/${history.id}`, { userId })
        } catch (error) {
          console.error(`发送弹幕到历史记录${history.id}失败:`, error)
        }
      }
      ElMessage.success('批量发送弹幕成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量发送弹幕失败:', error)
      ElMessage.error('批量发送弹幕失败')
    }
  }
}

const handleBatchSyncVideo = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(`确定要批量同步选中的 ${selectedHistories.value.length} 项的视频信息吗？`, '批量同步', {
      type: 'warning'
    })

    const loadingInstance = ElLoading.service({ text: '批量同步中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.post(`/api/history/syncVideo/${history.id}`)
        } catch (error) {
          console.error(`同步历史记录${history.id}失败:`, error)
        }
      }
      ElMessage.success('批量同步成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量同步失败:', error)
      ElMessage.error('批量同步失败')
    }
  }
}

const handleBatchMoveFiles = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(`确定要批量移动选中的 ${selectedHistories.value.length} 项的文件吗？`, '批量移动文件', {
      type: 'warning'
    })

    const loadingInstance = ElLoading.service({ text: '批量移动文件中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.post(`/api/history/moveFiles/${history.id}`)
        } catch (error) {
          console.error(`移动历史记录${history.id}文件失败:`, error)
        }
      }
      ElMessage.success('批量移动文件成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量移动文件失败:', error)
      ElMessage.error('批量移动文件失败')
    }
  }
}

const handleBatchResetStatus = () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  batchResetOptions.value = {
    upload: true,
    publish: true,
    danmaku: true,
    files: true
  }
  batchResetDialogVisible.value = true
}

const confirmBatchReset = async (options) => {
  try {
    const loadingInstance = ElLoading.service({ text: '批量重置中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.post(`/api/history/resetStatus/${history.id}`, options)
        } catch (error) {
          console.error(`重置历史记录${history.id}失败:`, error)
        }
      }
      ElMessage.success('批量重置成功')
      batchResetDialogVisible.value = false
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    console.error('批量重置失败:', error)
    ElMessage.error(error.response?.data?.msg || '批量重置失败')
  }
}

const handleBatchDeleteOnly = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(
      `此操作将仅删除选中的 ${selectedHistories.value.length} 项数据库记录，不会删除文件。确定要删除吗？`,
      '批量删除记录',
      { type: 'warning' }
    )
    
    const loadingInstance = ElLoading.service({ text: '批量删除中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.get(`/api/history/delete/${history.id}`)
        } catch (error) {
          console.error(`删除历史记录${history.id}失败:`, error)
        }
      }
      ElMessage.success('批量删除记录成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量删除失败:', error)
      ElMessage.error(error.response?.data?.msg || '批量删除失败')
    }
  }
}

const handleBatchDeleteWithFiles = async () => {
  if (selectedHistories.value.length === 0) {
    ElMessage.warning('请先选择记录')
    return
  }

  try {
    await ElMessageBox.confirm(
      `此操作将删除选中的 ${selectedHistories.value.length} 项数据库记录和所有相关文件，不可恢复。确定要删除吗？`,
      '批量删除记录和文件',
      { type: 'error', confirmButtonText: '确定删除' }
    )
    
    const loadingInstance = ElLoading.service({ text: '批量删除中...' })
    try {
      for (const history of selectedHistories.value) {
        try {
          await axios.post(`/api/history/deleteWithFiles/${history.id}`)
        } catch (error) {
          console.error(`删除历史记录${history.id}和文件失败:`, error)
        }
      }
      ElMessage.success('批量删除记录和文件成功')
      fetchHistories()
    } finally {
      loadingInstance.close()
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('批量删除失败:', error)
      ElMessage.error(error.response?.data?.msg || '批量删除失败')
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
    
    await startProgressPolling(row.id)
  } catch (error) {
    console.error('获取分P详情失败:', error)
  } finally {
    partsLoading.value = false
  }
}

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString('zh-CN')
}

watch(partsDialogVisible, (newVal) => {
  if (!newVal) {
    stopProgressPolling()
    currentHistoryId.value = null
  }
})

onMounted(() => {
  fetchHistories()
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

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

:deep(.el-table__header th) {
  background: #f5f7fa;
  color: #606266;
  font-weight: 600;
}

:deep(.el-table__row):hover {
  background: rgba(24, 144, 255, 0.04);
}
</style>
