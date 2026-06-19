<template>
  <div class="history-container">
    <!-- 标签页 -->
    <div class="history-tabs-bar">
      <el-tabs v-model="activeTab" @tab-change="handleTabChange" class="history-tabs">
        <el-tab-pane name="working">
          <template #label>
            <span>
              进行中
              <el-badge v-if="workingCount > 0" :value="workingCount" :max="99" class="tab-badge" type="warning" />
            </span>
          </template>
        </el-tab-pane>
        <el-tab-pane name="archived">
          <template #label>
            <span>历史归档</span>
          </template>
        </el-tab-pane>
      </el-tabs>

      <!-- 视图切换 + 操作区 -->
      <div class="tabs-actions">
        <el-button-group class="view-toggle">
          <el-button :type="viewMode === 'card' ? 'primary' : ''" @click="viewMode = 'card'; saveViewMode()">
            <el-icon><Grid /></el-icon>
          </el-button>
          <el-button :type="viewMode === 'table' ? 'primary' : ''" @click="viewMode = 'table'; saveViewMode()">
            <el-icon><List /></el-icon>
          </el-button>
        </el-button-group>
        <el-button @click="filterPanelVisible = !filterPanelVisible">
          <el-icon><Filter /></el-icon>
          筛选
          <el-badge v-if="activeFilterCount > 0" :value="activeFilterCount" :max="9" />
        </el-button>
        <el-button @click="exportHistories">
          <el-icon><Download /></el-icon>
          导出
        </el-button>
        <el-button type="success" plain @click="handleSyncSessions">
          <el-icon><RefreshRight /></el-icon>
          同步
        </el-button>
        <el-button type="primary" @click="fetchHistories">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </div>

    <!-- 筛选面板 -->
    <el-collapse-transition>
      <div v-if="filterPanelVisible" class="filter-panel">
        <div class="filter-row">
          <el-input
            v-model="searchParams.roomId"
            placeholder="房间ID"
            clearable
            style="width: 140px"
            @input="debouncedSearch"
          />
          <el-input
            v-model="searchParams.bvId"
            placeholder="BV号"
            clearable
            style="width: 180px"
            @input="debouncedSearch"
          />
          <el-input
            v-model="searchParams.sessionId"
            placeholder="SessionID"
            clearable
            style="width: 200px"
            @input="debouncedSearch"
          />
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            :shortcuts="dateShortcuts"
            value-format="YYYY-MM-DD"
            style="width: 280px"
            @change="handleSearch"
          />
          <el-button @click="clearFilters">清空</el-button>
          <el-button type="primary" @click="handleSearch">搜索</el-button>
        </div>
        <div class="filter-tags">
          <span class="filter-label">快速筛选：</span>
          <el-check-tag
            v-for="tag in quickFilterTags"
            :key="tag.value"
            :checked="quickFilters.includes(tag.value)"
            @change="toggleQuickFilter(tag.value)"
            class="quick-filter-tag"
          >
            {{ tag.label }}
          </el-check-tag>
        </div>
      </div>
    </el-collapse-transition>

    <!-- 批量操作栏 -->
    <BatchActions
      :selected-histories="selectedHistories"
      @upload="handleBatchUpload"
      @publish="handleBatchPublish"
      @sync-sessions="handleBatchSyncSessions"
      @sync-video="handleBatchSyncVideo"
      @move-files="handleBatchMoveFiles"
      @reset-status="handleBatchResetStatus"
      @delete-only="handleBatchDeleteOnly"
      @delete-with-files="handleBatchDeleteWithFiles"
    />

    <!-- 卡片视图 -->
    <template v-if="viewMode === 'card'">
      <div v-if="loading" class="card-grid">
        <el-skeleton v-for="n in 6" :key="n" animated class="history-skeleton">
          <template #template>
            <div class="skeleton-card">
              <el-skeleton-item variant="text" style="width:60%;margin-bottom:12px" />
              <el-skeleton-item variant="text" style="width:40%;margin-bottom:8px" />
              <el-skeleton-item variant="text" style="width:80%;margin-bottom:16px" />
              <el-skeleton-item variant="button" style="width:100px;height:28px" />
            </div>
          </template>
        </el-skeleton>
      </div>

      <div v-else-if="histories.length === 0" class="empty-state">
        <el-empty description="暂无录制历史" />
      </div>

      <SessionHistoryGroups
        v-else
        :histories="histories"
        :privacy-mode="privacyMode.value"
        :is-uploading="isUploading"
        :get-card-class="getCardClass"
        :get-history-progress="getHistoryProgress"
        :get-history-upload-percent="getHistoryUploadPercent"
        :format-time="formatTime"
        @show-actions="showActionsDialog"
        @show-parts="showParts"
        @filter-session="filterBySession"
      />
    </template>

    <!-- 表格视图 -->
    <el-card v-else class="table-card">
      <el-table
        :data="histories"
        style="width: 100%"
        v-loading="loading"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="50" />
        <el-table-column prop="roomId" label="房间ID" width="90">
          <template #default="{ row }">{{ privacyMode.value ? '***' : row.roomId }}</template>
        </el-table-column>
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag v-if="row.isHighlight" type="warning" size="small">高光</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">录制</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="title" label="标题" min-width="180" show-overflow-tooltip />
        <el-table-column prop="sessionId" label="SessionID" width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <button class="table-session-button" type="button" @click="filterBySession(row.sessionId)">
              {{ row.sessionId || '-' }}
            </button>
          </template>
        </el-table-column>
        <el-table-column prop="uname" label="主播" width="110">
          <template #default="{ row }">{{ privacyMode.value ? '***' : (row.uname || '-') }}</template>
        </el-table-column>
        <el-table-column label="上传状态" width="200">
          <template #default="{ row }">
            <div v-if="isUploading(row) && getHistoryProgress(row.id)">
              <el-progress
                :percentage="getHistoryUploadPercent(row.id)"
                :status="getHistoryUploadPercent(row.id) >= 100 ? 'success' : null"
                :stroke-width="8"
              >
                <span style="font-size: 12px;">{{ getHistoryUploadPercent(row.id) }}%</span>
              </el-progress>
              <div style="font-size: 11px; color: var(--text-color-secondary); margin-top: 2px;">
                {{ getHistoryProgress(row.id)?.activeCount || 0 }} 个分P上传中
              </div>
            </div>
            <el-tag v-else-if="row.bvId" type="success" size="small">已发布</el-tag>
            <el-tag v-else-if="row.uploadPartCount > 0" type="warning" size="small">已上传{{ row.uploadPartCount }}P</el-tag>
            <el-tag v-else-if="isUploading(row)" type="info" size="small">上传中</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">未上传</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="bvId" label="BV号" width="140">
          <template #default="{ row }">
            <a
              v-if="row.bvId && row.bvId.startsWith('BV')"
              :href="`https://www.bilibili.com/video/${row.bvId}`"
              target="_blank"
              class="bv-link"
            >{{ row.bvId }}</a>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="分P" width="70">
          <template #default="{ row }">
            <span class="part-count-link" @click="showParts(row)">{{ row.partCount || 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column label="视频状态" width="100">
          <template #default="{ row }">
            <el-tooltip v-if="row.videoState >= 0" :content="row.videoStateDesc || ''" placement="top">
              <el-tag v-if="row.videoState === 1" type="success" size="small">已通过</el-tag>
              <el-tag v-else-if="row.videoState === 0" type="warning" size="small">审核中</el-tag>
              <el-tag v-else-if="row.videoState < 0" type="danger" size="small">未通过</el-tag>
            </el-tooltip>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="startTime" label="开始时间" width="160">
          <template #default="{ row }">{{ formatTime(row.startTime) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="showActionsDialog(row)">操作</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 分页 -->
    <div class="pagination">
      <el-pagination
        v-model:current-page="searchParams.page"
        v-model:page-size="searchParams.pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handleSizeChange"
        @current-change="handlePageChange"
        background
      />
    </div>

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
      @manual-publish="handleManualPublish"
      @sync-video="handleSyncVideoInDialog"
      @move-files="handleMoveFilesInDialog"
      @reset-status="handleResetStatus"
      @delete-only="handleDeleteOnly"
      @delete-with-files="handleDeleteWithFiles"
      @force-archive="handleForceArchive"
      @show-parts="handleShowPartsFromDialog"
    />

    <!-- 手动标记投稿对话框 -->
    <ManualPublishDialog
      v-model:visible="manualPublishDialogVisible"
      :history="currentHistory"
      @success="handleManualPublishSuccess"
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
import { ref, computed, inject, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Grid, List, Filter, Refresh, Download, RefreshRight
} from '@element-plus/icons-vue'
import { historyAPI } from '@/api'
import axios from 'axios'
import BatchActions from '@/components/history/BatchActions.vue'
import PartsDialog from '@/components/history/PartsDialog.vue'
import ActionsDialog from '@/components/history/ActionsDialog.vue'
import ManualPublishDialog from '@/components/history/ManualPublishDialog.vue'
import ResetStatusDialog from '@/components/history/ResetStatusDialog.vue'
import SessionHistoryGroups from '@/components/history/SessionHistoryGroups.vue'
import { useHistoryProgress, useHistoryOperations } from '@/composables/useHistory'

const privacyMode = inject('privacyMode', ref(false))

const histories = ref([])
const loading = ref(false)
const total = ref(0)
const workingCount = ref(0)
const selectedHistories = ref([])
const activeTab = ref('working')
const viewMode = ref(localStorage.getItem('historyViewMode') || 'table')
const filterPanelVisible = ref(false)
const dateRange = ref(null)

const saveViewMode = () => localStorage.setItem('historyViewMode', viewMode.value)

const searchParams = ref({
  page: 1,
  pageSize: 10,
  roomId: '',
  bvId: '',
  sessionId: '',
  viewType: 'working',
  from: '',
  to: '',
  recording: null,
  upload: null,
  publish: null,
  isHighlight: null
})

const quickFilterTags = [
  { label: '录制中', value: 'recording' },
  { label: '已投稿', value: 'published' },
  { label: '上传中', value: 'uploading' },
  { label: '未上传', value: 'noUpload' },
  { label: '高能剪辑', value: 'highlight' }
]
const quickFilters = ref([])

const activeFilterCount = computed(() => {
  let count = 0
  if (searchParams.value.roomId) count++
  if (searchParams.value.bvId) count++
  if (searchParams.value.sessionId) count++
  if (dateRange.value) count++
  count += quickFilters.value.length
  return count
})

const dateShortcuts = [
  { text: '今天', value: () => { const d = new Date(); return [d, d] } },
  { text: '最近24小时', value: () => { const d = new Date(); const y = new Date(d - 86400000); return [y, d] } },
  { text: '最近3天', value: () => { const d = new Date(); return [new Date(d - 3 * 86400000), d] } },
  { text: '最近7天', value: () => { const d = new Date(); return [new Date(d - 7 * 86400000), d] } },
  { text: '最近30天', value: () => { const d = new Date(); return [new Date(d - 30 * 86400000), d] } }
]

// dialogs
const partsDialogVisible = ref(false)
const parts = ref([])
const partsLoading = ref(false)
const currentHistoryId = ref(null)
const actionsDialogVisible = ref(false)
const manualPublishDialogVisible = ref(false)
const currentHistory = ref(null)
const resetDialogVisible = ref(false)
const resetOptions = ref({ upload: true, publish: true, danmaku: true, files: true })
const batchResetDialogVisible = ref(false)
const batchResetOptions = ref({ upload: true, publish: true, danmaku: true, files: true })

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
  handleSyncVideo,
  handleMoveFiles,
  handleResetStatus: resetHistoryStatus,
  handleDeleteOnly: deleteHistoryOnly,
  handleDeleteWithFiles: deleteHistoryWithFiles
} = useHistoryOperations()

const isUploading = (row) => row.uploadStatus === 1 && !row.bvId

const getCardClass = (row) => {
  if (row.bvId) return 'status-success'
  if (isUploading(row)) return 'status-uploading'
  if (row.recording) return 'status-recording'
  if (row.uploadPartCount > 0) return 'status-partial'
  return ''
}

const toggleQuickFilter = (value) => {
  const idx = quickFilters.value.indexOf(value)
  if (idx >= 0) quickFilters.value.splice(idx, 1)
  else quickFilters.value.push(value)
  applyQuickFilters()
  handleSearch()
}

const applyQuickFilters = () => {
  searchParams.value.recording = null
  searchParams.value.upload = null
  searchParams.value.publish = null
  searchParams.value.isHighlight = null
  for (const f of quickFilters.value) {
    if (f === 'recording') searchParams.value.recording = true
    if (f === 'published') searchParams.value.publish = true
    if (f === 'uploading') searchParams.value.upload = 1
    if (f === 'noUpload') searchParams.value.upload = 0
    if (f === 'highlight') searchParams.value.isHighlight = true
  }
}

const clearFilters = () => {
  searchParams.value.roomId = ''
  searchParams.value.bvId = ''
  searchParams.value.sessionId = ''
  searchParams.value.from = ''
  searchParams.value.to = ''
  searchParams.value.recording = null
  searchParams.value.upload = null
  searchParams.value.publish = null
  searchParams.value.isHighlight = null
  dateRange.value = null
  quickFilters.value = []
  handleSearch()
}

const filterBySession = (sessionId) => {
  if (!sessionId) return
  searchParams.value.sessionId = sessionId
  searchParams.value.page = 1
  filterPanelVisible.value = true
  fetchHistories()
}

let searchTimer = null
const debouncedSearch = () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => handleSearch(), 400)
}

const fetchHistories = async () => {
  loading.value = true
  if (dateRange.value) {
    searchParams.value.from = dateRange.value[0] || ''
    searchParams.value.to = dateRange.value[1] || ''
  } else {
    searchParams.value.from = ''
    searchParams.value.to = ''
  }

  try {
    const data = await historyAPI.list(searchParams.value)
    histories.value = data?.list || []
    total.value = data?.total || 0
    workingCount.value = data?.workingCount ?? workingCount.value

    const hasUploading = histories.value.some(h => isUploading(h))
    if (hasUploading) {
      startHistoryProgressPolling(() => histories.value)
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

const exportHistories = async () => {
  if (dateRange.value) {
    searchParams.value.from = dateRange.value[0] || ''
    searchParams.value.to = dateRange.value[1] || ''
  } else {
    searchParams.value.from = ''
    searchParams.value.to = ''
  }
  try {
    const blob = await historyAPI.export(searchParams.value)
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `gobup-history-${Date.now()}.csv`
    a.click()
    window.URL.revokeObjectURL(url)
    ElMessage.success('导出成功')
  } catch (error) {
    console.error('导出失败:', error)
    ElMessage.error('导出失败')
  }
}

const handleTabChange = (tab) => {
  searchParams.value.viewType = tab
  searchParams.value.page = 1
  fetchHistories()
}

const handleSearch = () => {
  searchParams.value.page = 1
  fetchHistories()
}

const handlePageChange = () => fetchHistories()
const handleSizeChange = () => { searchParams.value.page = 1; fetchHistories() }
const handleSelectionChange = (sel) => { selectedHistories.value = sel }

const showActionsDialog = (row) => {
  currentHistory.value = row
  actionsDialogVisible.value = true
}

const handleUploadInDialog = async () => {
  const historyId = currentHistory.value.id
  await handleUpload(currentHistory.value, async () => {
    await fetchHistories()
    startHistoryProgressPolling(() => histories.value)
    const up = histories.value.find(h => h.id === historyId)
    if (up) currentHistory.value = up
  })
}

const handlePublishInDialog = async () => {
  const historyId = currentHistory.value.id
  await handlePublish(currentHistory.value, async () => {
    await fetchHistories()
    const up = histories.value.find(h => h.id === historyId)
    if (up) currentHistory.value = up
  })
}

const handleManualPublish = () => { manualPublishDialogVisible.value = true }

const handleManualPublishSuccess = async () => {
  const historyId = currentHistory.value.id
  await fetchHistories()
  const up = histories.value.find(h => h.id === historyId)
  if (up) currentHistory.value = up
  ElMessage.success('投稿信息已更新')
}

const handleSyncVideoInDialog = async () => {
  const historyId = currentHistory.value.id
  await handleSyncVideo(currentHistory.value, async () => {
    await fetchHistories()
    const up = histories.value.find(h => h.id === historyId)
    if (up) currentHistory.value = up
  })
}

const handleMoveFilesInDialog = async () => {
  const historyId = currentHistory.value.id
  await handleMoveFiles(currentHistory.value, async () => {
    await fetchHistories()
    const up = histories.value.find(h => h.id === historyId)
    if (up) currentHistory.value = up
  })
}

const handleResetStatus = () => {
  resetOptions.value = { upload: true, publish: true, danmaku: true, files: true }
  resetDialogVisible.value = true
}

const confirmReset = async (options) => {
  const historyId = currentHistory.value.id
  await resetHistoryStatus(historyId, options, async () => {
    resetDialogVisible.value = false
    await fetchHistories()
    const up = histories.value.find(h => h.id === historyId)
    if (up) currentHistory.value = up
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

const handleForceArchive = async () => {
  try {
    await historyAPI.forceArchive(currentHistory.value.id)
    ElMessage.success('已强制归档')
    actionsDialogVisible.value = false
    fetchHistories()
  } catch (error) {
    console.error('强制归档失败:', error)
    ElMessage.error('强制归档失败')
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

const handleShowPartsFromDialog = () => {
  if (!currentHistory.value) return
  actionsDialogVisible.value = false
  showParts(currentHistory.value)
}

// 批量操作
const handleBatchUpload = async () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  try {
    await ElMessageBox.confirm(`确定要批量上传选中的 ${selectedHistories.value.length} 项吗？`, '批量上传', { type: 'warning' })
    const userResponse = await axios.get('/api/biliUser/list')
    const users = userResponse.data || []
    if (!users.length) return ElMessage.warning('请先添加B站用户')
    const historyIds = selectedHistories.value.map(h => h.id)
    const response = await axios.post('/api/history/batchUpload', { historyIds, userId: users[0].id })
    ElMessage.success(response.data.msg || '批量上传任务已启动')
    startHistoryProgressPolling(() => histories.value)
    fetchHistories()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '批量上传失败') }
}

const handleBatchPublish = async () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  try {
    await ElMessageBox.confirm(`确定要批量投稿选中的 ${selectedHistories.value.length} 项吗？`, '批量投稿', { type: 'warning' })
    const userResponse = await axios.get('/api/biliUser/list')
    const users = userResponse.data || []
    if (!users.length) return ElMessage.warning('请先添加B站用户')
    const historyIds = selectedHistories.value.map(h => h.id)
    const response = await axios.post('/api/history/batchPublish', { historyIds, userId: users[0].id })
    ElMessage.success(response.data.msg || '批量投稿任务已提交')
    fetchHistories()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '批量投稿失败') }
}

const handleBatchSyncVideo = async () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  try {
    await ElMessageBox.confirm(`确定要批量同步 ${selectedHistories.value.length} 项视频信息吗？`, '批量同步', { type: 'warning' })
    const response = await axios.post('/api/history/batchSyncVideo', { historyIds: selectedHistories.value.map(h => h.id) })
    ElMessage.success(response.data.msg || '批量同步成功')
    fetchHistories()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '批量同步失败') }
}

const handleSyncSessions = async () => {
  try {
    await ElMessageBox.confirm('将按同房间、完全一致标题重整同场直播分组。确定同步吗？', '同步', { type: 'info' })
    const response = await historyAPI.syncSessions([])
    ElMessage.success(response.msg || '同步完成')
    fetchHistories()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '同步失败')
  }
}

const handleBatchSyncSessions = async () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  try {
    await ElMessageBox.confirm(`将同步并重整选中的 ${selectedHistories.value.length} 项同场直播分组。确定继续吗？`, '同步', { type: 'info' })
    const response = await historyAPI.syncSessions(selectedHistories.value.map(h => h.id))
    ElMessage.success(response.msg || '同步完成')
    fetchHistories()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '同步失败')
  }
}

const handleBatchMoveFiles = async () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  try {
    await ElMessageBox.confirm(`确定要批量移动 ${selectedHistories.value.length} 项文件吗？`, '批量移动', { type: 'warning' })
    const response = await axios.post('/api/history/batchMoveFiles', { historyIds: selectedHistories.value.map(h => h.id) })
    ElMessage.success(response.data.msg || '批量移动成功')
    fetchHistories()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '批量移动失败') }
}

const handleBatchResetStatus = () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  batchResetOptions.value = { upload: true, publish: true, danmaku: true, files: true }
  batchResetDialogVisible.value = true
}

const confirmBatchReset = async (options) => {
  try {
    const response = await axios.post('/api/history/batchResetStatus', {
      historyIds: selectedHistories.value.map(h => h.id), ...options
    })
    ElMessage.success(response.data.msg || '批量重置成功')
    batchResetDialogVisible.value = false
    fetchHistories()
  } catch (e) { ElMessage.error(e.response?.data?.msg || '批量重置失败') }
}

const handleBatchDeleteOnly = async () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  try {
    await ElMessageBox.confirm(`将仅删除 ${selectedHistories.value.length} 条数据库记录，不删除文件。确定吗？`, '批量删除', { type: 'warning' })
    const response = await axios.post('/api/history/batchDelete', { ids: selectedHistories.value.map(h => h.id) })
    ElMessage.success(response.data.msg || '批量删除成功')
    fetchHistories()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '批量删除失败') }
}

const handleBatchDeleteWithFiles = async () => {
  if (!selectedHistories.value.length) return ElMessage.warning('请先选择记录')
  try {
    await ElMessageBox.confirm(`将删除 ${selectedHistories.value.length} 条记录及所有相关文件，不可恢复！`, '批量删除', { type: 'error', confirmButtonText: '确定删除' })
    const response = await axios.post('/api/history/batchDeleteWithFiles', {
      historyIds: selectedHistories.value.map(h => h.id),
      confirmDeleteFiles: true,
      confirmText: 'DELETE_FILES'
    })
    ElMessage.success(response.data.msg || '批量删除成功')
    fetchHistories()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.msg || '批量删除失败') }
}

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString('zh-CN')
}

watch(partsDialogVisible, (newVal) => {
  if (!newVal) { stopProgressPolling(); currentHistoryId.value = null }
})

onMounted(() => fetchHistories())
</script>

<style scoped lang="scss">
.history-container {
  animation: fadeIn 0.3s ease;
}

/* ===== 标签栏 ===== */
.history-tabs-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-color-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-large);
  padding: 0 var(--spacing-lg);
  margin-bottom: var(--spacing-md);

  .history-tabs {
    flex: 1;
    :deep(.el-tabs__header) {
      margin: 0;
      border-bottom: none;
    }
    :deep(.el-tabs__nav-wrap::after) {
      display: none;
    }
  }
}

.tabs-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) 0;
}

.tab-badge {
  margin-left: 6px;
  :deep(.el-badge__content) {
    position: static;
    transform: none;
    vertical-align: middle;
    margin-left: 4px;
  }
}

/* ===== 筛选面板 ===== */
.filter-panel {
  background: var(--bg-color-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-large);
  padding: var(--spacing-lg);
  margin-bottom: var(--spacing-md);
}

.filter-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md);
}

.filter-tags {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.filter-label {
  font-size: var(--font-size-sm);
  color: var(--text-color-secondary);
  flex-shrink: 0;
}

.quick-filter-tag {
  cursor: pointer;
  border-radius: var(--border-radius-medium);
  font-size: var(--font-size-sm);
  
  &:deep(.el-check-tag) {
    border-radius: var(--border-radius-medium);
  }
}

/* ===== 卡片视图 ===== */
.table-session-button {
  border: none;
  background: transparent;
  color: var(--primary-color);
  cursor: pointer;
  font: inherit;
  padding: 0;
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}

.table-session-button:hover {
  text-decoration: underline;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.history-skeleton .skeleton-card {
  background: var(--bg-color-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-large);
  padding: var(--spacing-lg);
  height: 164px;
}

/* ===== 表格 ===== */
.table-card {
  background: var(--bg-color-secondary);
  margin-bottom: var(--spacing-md);
}

.bv-link {
  color: var(--primary-color);
  text-decoration: none;
  
  &:hover { text-decoration: underline; }
}

.part-count-link {
  cursor: pointer;
  color: var(--primary-color);
  font-weight: var(--font-weight-medium);

  &:hover { text-decoration: underline; }
}

/* ===== 分页 ===== */
.pagination {
  margin-top: var(--spacing-lg);
  display: flex;
  justify-content: flex-end;
}

/* ===== 空状态 ===== */
.empty-state {
  background: var(--bg-color-secondary);
  border: 1px dashed var(--border-color);
  border-radius: var(--border-radius-large);
  padding: 60px 20px;
  margin-bottom: var(--spacing-lg);
}

/* ===== 视图切换 ===== */
.view-toggle {
  .el-button {
    padding: 8px 12px;
  }
}
</style>
