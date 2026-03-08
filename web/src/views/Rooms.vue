<template>
  <div class="rooms-container">
    <!-- 统计头部 -->
    <div class="stats-header">
      <div class="stat-card">
        <div class="stat-icon total"><el-icon><HomeFilled /></el-icon></div>
        <div class="stat-content">
          <div class="stat-value">{{ rooms.length }}</div>
          <div class="stat-label">总房间数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon streaming"><el-icon><VideoPlay /></el-icon></div>
        <div class="stat-content">
          <div class="stat-value">{{ streamingCount }}</div>
          <div class="stat-label">直播中</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon recording"><el-icon><Promotion /></el-icon></div>
        <div class="stat-content">
          <div class="stat-value">{{ recordingCount }}</div>
          <div class="stat-label">录制中</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon upload"><el-icon><Upload /></el-icon></div>
        <div class="stat-content">
          <div class="stat-value">{{ uploadEnabledCount }}</div>
          <div class="stat-label">启用上传</div>
        </div>
      </div>
    </div>

    <!-- 操作栏 -->
    <div class="action-bar">
      <div class="action-bar-left">
        <el-input
          v-model="searchKeyword"
          placeholder="搜索房间ID或主播名"
          clearable
          style="width: 220px"
          @input="handleSearch"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
      </div>
      <div class="action-bar-right">
        <el-button-group class="view-toggle">
          <el-button :type="viewMode === 'card' ? 'primary' : ''" @click="viewMode = 'card'">
            <el-icon><Grid /></el-icon>
          </el-button>
          <el-button :type="viewMode === 'table' ? 'primary' : ''" @click="viewMode = 'table'">
            <el-icon><List /></el-icon>
          </el-button>
        </el-button-group>
        <el-button @click="handleExport">
          <el-icon><Download /></el-icon>
          导出配置
        </el-button>
        <el-button @click="handleImport">
          <el-icon><FolderOpened /></el-icon>
          导入配置
        </el-button>
        <el-button type="primary" @click="handleAdd">
          <el-icon><Plus /></el-icon>
          添加房间
        </el-button>
      </div>
    </div>

    <!-- 卡片模式 -->
    <template v-if="viewMode === 'card'">
      <!-- 骨架屏 -->
      <div v-if="loading" class="card-grid">
        <el-skeleton v-for="n in 6" :key="n" class="room-skeleton" animated>
          <template #template>
            <div class="skeleton-card">
              <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px">
                <el-skeleton-item variant="circle" style="width:36px;height:36px" />
                <div style="flex:1">
                  <el-skeleton-item variant="text" style="width:60%;margin-bottom:8px" />
                  <el-skeleton-item variant="text" style="width:40%" />
                </div>
              </div>
              <el-skeleton-item variant="text" style="width:100%;margin-bottom:8px" />
              <el-skeleton-item variant="text" style="width:80%;margin-bottom:16px" />
              <el-skeleton-item variant="button" style="width:80px;height:28px" />
            </div>
          </template>
        </el-skeleton>
      </div>

      <!-- 空状态 -->
      <div v-else-if="filteredRooms.length === 0" class="empty-state">
        <el-empty description="暂无房间，点击添加房间开始配置">
          <el-button type="primary" @click="handleAdd">
            <el-icon><Plus /></el-icon>
            添加房间
          </el-button>
        </el-empty>
      </div>

      <!-- 卡片列表 -->
      <div v-else class="card-grid">
        <div
          v-for="room in filteredRooms"
          :key="room.id"
          class="room-card"
          :class="{
            'is-streaming': room.recording,
            'is-uploading': room.uploading
          }"
        >
          <!-- 状态角标 -->
          <div class="room-card-badges">
            <el-tag v-if="room.recording" type="danger" size="small" effect="dark">录制中</el-tag>
            <el-tag v-if="room.uploading" type="warning" size="small" effect="dark">上传中</el-tag>
            <el-tag v-if="!room.upload" type="info" size="small" effect="plain">未启用上传</el-tag>
          </div>

          <!-- 卡片头 -->
          <div class="room-card-header">
            <div class="room-avatar">
              {{ room.uname ? room.uname.charAt(0) : '?' }}
            </div>
            <div class="room-info">
              <div class="room-name">{{ privacyMode ? '***' : (room.uname || '未知主播') }}</div>
              <div class="room-id">房间 #{{ privacyMode ? '***' : room.roomId }}</div>
            </div>
          </div>

          <!-- 标题 -->
          <div class="room-title" :title="room.title">
            {{ room.title || '暂无标题' }}
          </div>

          <!-- 分区标签 -->
          <div class="room-meta" v-if="room.areaName">
            <el-tag size="small" type="info" effect="plain">{{ room.areaName }}</el-tag>
          </div>

          <!-- 上传线路 -->
          <div class="room-line">
            <el-icon><Connection /></el-icon>
            {{ formatLine(room.line) }}
          </div>

          <!-- 操作按钮 -->
          <div class="room-card-actions">
            <el-button size="small" type="primary" plain @click="handleEdit(room)">
              <el-icon><Edit /></el-icon>
              编辑
            </el-button>
            <el-button size="small" @click="handleCopyRoom(room)">
              <el-icon><CopyDocument /></el-icon>
              复制
            </el-button>
            <el-button size="small" type="danger" plain @click="handleDelete(room)">
              <el-icon><Delete /></el-icon>
              删除
            </el-button>
          </div>
        </div>
      </div>
    </template>

    <!-- 表格模式 -->
    <el-card v-else class="table-card">
      <el-table :data="filteredRooms" style="width: 100%" v-loading="loading">
        <el-table-column prop="roomId" label="房间ID" width="100">
          <template #default="{ row }">
            {{ privacyMode ? '***' : row.roomId }}
          </template>
        </el-table-column>
        <el-table-column prop="uname" label="主播" width="140">
          <template #default="{ row }">
            {{ privacyMode ? '***' : (row.uname || '-') }}
          </template>
        </el-table-column>
        <el-table-column prop="title" label="房间标题" min-width="150" show-overflow-tooltip />
        <el-table-column label="状态" width="140">
          <template #default="{ row }">
            <el-tag v-if="row.recording" type="danger" size="small" style="margin-right:4px">录制中</el-tag>
            <el-tag v-if="row.uploading" type="warning" size="small">上传中</el-tag>
            <el-tag v-if="!row.recording && !row.uploading" type="info" size="small">待机</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用上传" width="90">
          <template #default="{ row }">
            <el-tag :type="row.upload ? 'success' : 'info'" size="small">
              {{ row.upload ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="line" label="上传线路" width="160">
          <template #default="{ row }">{{ formatLine(row.line) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="handleEdit(row)">编辑</el-button>
            <el-button size="small" @click="handleCopyRoom(row)">复制</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 编辑对话框 -->
    <RoomEditDialog
      v-model:visible="dialogVisible"
      :title="dialogTitle"
      :form="form"
      :users="users"
      :upload-lines="uploadLines"
      :line-stats="lineStats"
      :line-speeds="lineSpeeds"
      :testing-lines="testingLines"
      :testing-deep-speed="testingDeepSpeed"
      :saving="saving"
      @save="handleSave"
      @test-lines="testLines"
      @test-deep-speed="testDeepSpeed"
      @preview-template="previewTemplate"
    />

    <!-- 导出配置对话框 -->
    <el-dialog v-model="exportDialogVisible" title="导出配置" width="360px">
      <p style="margin-bottom:16px;color:var(--text-color-secondary)">选择要导出的数据：</p>
      <el-checkbox v-model="exportOptions.rooms" label="房间配置" />
      <el-checkbox v-model="exportOptions.users" label="用户信息" />
      <el-checkbox v-model="exportOptions.histories" label="历史记录" />
      <template #footer>
        <el-button @click="exportDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="doExport">导出</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, inject, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Download, Upload, Plus, Search, Grid, List,
  Edit, Delete, CopyDocument, FolderOpened,
  HomeFilled, VideoPlay, Promotion, Connection
} from '@element-plus/icons-vue'
import { roomAPI, userAPI, configAPI } from '@/api'
import RoomEditDialog from '@/components/rooms/RoomEditDialog.vue'
import { useLineTest, formatLine } from '@/composables/useRooms'

const privacyMode = inject('privacyMode', ref(false))

const rooms = ref([])
const users = ref([])
const uploadLines = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const dialogTitle = ref('添加房间')
const saving = ref(false)
const viewMode = ref(localStorage.getItem('roomViewMode') || 'card')
const searchKeyword = ref('')
const exportDialogVisible = ref(false)
const exportOptions = ref({ rooms: true, users: true, histories: false })

// 保存视图模式
const updateViewMode = (mode) => {
  viewMode.value = mode
  localStorage.setItem('roomViewMode', mode)
}

const filteredRooms = computed(() => {
  if (!searchKeyword.value) return rooms.value
  const kw = searchKeyword.value.toLowerCase()
  return rooms.value.filter(r =>
    String(r.roomId).includes(kw) ||
    (r.uname && r.uname.toLowerCase().includes(kw))
  )
})

const streamingCount = computed(() => rooms.value.filter(r => r.liveStatus === 1 || r.streaming).length)
const recordingCount = computed(() => rooms.value.filter(r => r.recording).length)
const uploadEnabledCount = computed(() => rooms.value.filter(r => r.upload).length)

const handleSearch = () => { /* reactive */ }

const form = ref({
  roomId: '',
  upload: true,
  autoUpload: true,
  autoPublish: true,
  mergeBySession: true,
  autoParseDanmaku: true,
  autoSyncInfo: true,
  autoUpdatePublished: false,
  uploadUserId: null,
  titleTemplate: '【直播回放】【${uname}】${title} ${yyyy年MM月dd日HH点mm分}',
  descTemplate: '直播录像\\n${uname}直播间：https://live.bilibili.com/${roomId}',
  tags: '直播回放,${uname},${areaName}',
  tid: 21,
  copyright: 1,
  line: 'CS_UPOS',
  fileOpTrigger: 3,
  fileOpAction: 1,
  fileOpScope: 1,
  fileOpDelay: 0,
  partTitleTemplate: 'P${index}-${areaName}-${MM月dd日HH点mm分}',
  wxuid: '',
  pushMsgTags: '开播,上传,投稿',
  coverType: 'default',
  coverUrl: '',
  highEnergyCut: false,
  windowSize: 60,
  percentileRank: 75,
  minSegmentDuration: 10,
  dmDistinct: true,
  dmUlLevel: 0,
  dmMedalLevel: 0,
  dmKeywordBlacklist: '',
  enableDanmakuBurn: false,
  danmakuBurnStyle: 'default',
  dynamicTemplate: '',
  moveDir: '',
  isOnlySelf: false,
  noDisturbance: false,
  fileSizeLimit: 0,
  durationLimit: 60
})

const getDefaultForm = () => ({
  roomId: '',
  upload: true,
  autoUpload: true,
  autoPublish: true,
  mergeBySession: true,
  autoParseDanmaku: true,
  autoSyncInfo: true,
  autoUpdatePublished: false,
  uploadUserId: users.value[0]?.id || null,
  titleTemplate: '【直播回放】【${uname}】${title} ${yyyy年MM月dd日HH点mm分}',
  descTemplate: '直播录像\\n${uname}直播间：https://live.bilibili.com/${roomId}',
  tags: '直播回放,${uname},${areaName}',
  tid: 21,
  copyright: 1,
  line: 'CS_UPOS',
  deleteType: 9,
  partTitleTemplate: 'P${index}-${areaName}-${MM月dd日HH点mm分}',
  wxuid: '',
  pushMsgTags: '开播,上传,投稿',
  coverType: 'default',
  coverUrl: '',
  highEnergyCut: false,
  windowSize: 60,
  percentileRank: 75,
  minSegmentDuration: 10,
  dmDistinct: true,
  dmUlLevel: 0,
  dmMedalLevel: 0,
  dmKeywordBlacklist: '',
  enableDanmakuBurn: false,
  danmakuBurnStyle: 'default',
  dynamicTemplate: '',
  moveDir: '',
  isOnlySelf: false,
  noDisturbance: false,
  fileSizeLimit: 0,
  durationLimit: 60
})
const {
  lineStats,
  lineSpeeds,
  testingLines,
  testingDeepSpeed,
  testLines,
  testDeepSpeed
} = useLineTest()

const fetchRooms = async () => {
  loading.value = true
  try {
    const data = await roomAPI.list()
    rooms.value = data || []
  } catch (error) {
    console.error('获取房间列表失败:', error)
  } finally {
    loading.value = false
  }
}

const fetchUsers = async () => {
  try {
    const data = await userAPI.list()
    users.value = (data || []).filter(user => user.uid !== -1)
  } catch (error) {
    console.error('获取用户列表失败:', error)
  }
}

const fetchUploadLines = async () => {
  try {
    const data = await roomAPI.getLines()
    uploadLines.value = data || []
  } catch (error) {
    console.error('获取上传线路失败:', error)
  }
}

const handleAdd = () => {
  dialogTitle.value = '添加房间'
  form.value = getDefaultForm()
  dialogVisible.value = true
}

const handleEdit = (row) => {
  dialogTitle.value = '编辑房间'
  form.value = { ...row }
  dialogVisible.value = true
}

const handleCopyRoom = async (row) => {
  const config = JSON.stringify({ ...row, id: undefined, roomId: '' }, null, 2)
  try {
    await navigator.clipboard.writeText(config)
    ElMessage.success('房间配置已复制到剪贴板')
  } catch {
    ElMessage.info('请手动复制：' + config.substring(0, 50) + '...')
  }
}

const handleSave = async (formData) => {
  saving.value = true
  try {
    await roomAPI.update(formData)
    ElMessage.success('保存成功')
    dialogVisible.value = false
    fetchRooms()
  } catch (error) {
    console.error('保存失败:', error)
  } finally {
    saving.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这个房间吗？', '提示', { type: 'warning' })
    await roomAPI.delete(row.id)
    ElMessage.success('删除成功')
    fetchRooms()
  } catch (error) {
    if (error !== 'cancel') console.error('删除失败:', error)
  }
}

const handleExport = () => {
  exportDialogVisible.value = true
}

const doExport = async () => {
  try {
    const blob = await configAPI.export(exportOptions.value)
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `gobup-config-${Date.now()}.json`
    a.click()
    window.URL.revokeObjectURL(url)
    ElMessage.success('导出成功')
    exportDialogVisible.value = false
  } catch (error) {
    console.error('导出失败:', error)
    ElMessage.error('导出失败')
  }
}

const handleImport = () => {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = '.json'
  input.onchange = async (e) => {
    const file = e.target.files[0]
    if (!file) return
    try {
      await configAPI.import(file)
      ElMessage.success('导入成功')
      fetchRooms()
      fetchUsers()
    } catch (error) {
      console.error('导入失败:', error)
      ElMessage.error('导入失败')
    }
  }
  input.click()
}

const previewTemplate = async (formData) => {
  if (!formData.dynamicTemplate) {
    ElMessage.warning('请先输入动态模板')
    return
  }
  try {
    const result = await roomAPI.verifyTemplate({
      roomId: formData.roomId,
      template: formData.dynamicTemplate
    })
    ElMessageBox.alert(result.result, '模板预览', { confirmButtonText: '确定' })
  } catch (error) {
    console.error('模板预览失败:', error)
    ElMessage.error('模板预览失败')
  }
}

onMounted(() => {
  fetchRooms()
  fetchUsers()
  fetchUploadLines()
})
</script>

<style scoped lang="scss">
.rooms-container {
  padding: 0;
  animation: fadeIn 0.3s ease;
}

/* ===== 统计卡片 ===== */
.stats-header {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);

  @media (max-width: 1024px) {
    grid-template-columns: repeat(2, 1fr);
  }

  @media (max-width: 480px) {
    grid-template-columns: repeat(2, 1fr);
    gap: var(--spacing-sm);
  }
}

.stat-card {
  background: var(--bg-color-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-large);
  padding: var(--spacing-lg);
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  transition: all var(--transition-normal);
  
  &:hover {
    box-shadow: var(--shadow-medium);
    transform: translateY(-2px);
  }
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  
  &.total { background: rgba(64, 158, 255, 0.15); color: #409eff; }
  &.streaming { background: rgba(245, 108, 108, 0.15); color: #f56c6c; }
  &.recording { background: rgba(230, 162, 60, 0.15); color: #e6a23c; }
  &.upload { background: rgba(103, 194, 58, 0.15); color: #67c23a; }
}

.stat-content {
  .stat-value {
    font-size: 24px;
    font-weight: var(--font-weight-bold);
    color: var(--text-color-primary);
    line-height: 1;
    margin-bottom: 4px;
  }
  .stat-label {
    font-size: var(--font-size-sm);
    color: var(--text-color-secondary);
  }
}

/* ===== 操作栏 ===== */
.action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
  flex-wrap: wrap;
}

.action-bar-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.view-toggle {
  .el-button {
    padding: 8px 12px;
  }
}

/* ===== 卡片网格 ===== */
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--spacing-lg);
}

/* ===== 骨架屏 ===== */
.room-skeleton {
  .skeleton-card {
    background: var(--bg-color-secondary);
    border: 1px solid var(--border-color);
    border-radius: var(--border-radius-large);
    padding: var(--spacing-lg);
    height: 180px;
  }
}

/* ===== 房间卡片 ===== */
.room-card {
  background: var(--bg-color-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-large);
  padding: var(--spacing-lg);
  position: relative;
  transition: all var(--transition-normal);
  animation: fadeIn 0.3s ease;
  
  &:hover {
    box-shadow: var(--shadow-medium);
    transform: translateY(-2px);
    border-color: var(--primary-color);
  }
  
  &.is-streaming {
    border-left: 3px solid var(--danger-color);
  }
  
  &.is-uploading {
    border-left: 3px solid var(--warning-color);
  }
}

.room-card-badges {
  position: absolute;
  top: var(--spacing-md);
  right: var(--spacing-md);
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: flex-end;
  max-width: 50%;
}

.room-card-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);
}

.room-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--primary-color);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: var(--font-weight-bold);
  font-size: var(--font-size-lg);
  flex-shrink: 0;
}

.room-info {
  flex: 1;
  min-width: 0;
  
  .room-name {
    font-weight: var(--font-weight-semibold);
    color: var(--text-color-primary);
    font-size: var(--font-size-base);
  }
  
  .room-id {
    font-size: var(--font-size-sm);
    color: var(--text-color-secondary);
    margin-top: 2px;
  }
}

.room-title {
  color: var(--text-color-secondary);
  font-size: var(--font-size-sm);
  margin-bottom: var(--spacing-sm);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  padding-right: calc(50% + var(--spacing-sm));
}

.room-meta {
  margin-bottom: var(--spacing-sm);
}

.room-line {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--font-size-sm);
  color: var(--text-color-secondary);
  margin-bottom: var(--spacing-md);
}

.room-card-actions {
  display: flex;
  gap: var(--spacing-sm);
  margin-top: auto;
}

/* ===== 表格卡片 ===== */
.table-card {
  background: var(--bg-color-secondary);
}

/* ===== 空状态 ===== */
.empty-state {
  background: var(--bg-color-secondary);
  border: 1px dashed var(--border-color);
  border-radius: var(--border-radius-large);
  padding: 60px 20px;
  text-align: center;
}
</style>
