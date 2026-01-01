<template>
  <div class="rooms-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>房间列表</span>
          <div class="header-actions">
            <el-button @click="handleExport">
              <el-icon><Download /></el-icon>
              导出配置
            </el-button>
            <el-button @click="handleImport">
              <el-icon><Upload /></el-icon>
              导入配置
            </el-button>
            <el-button type="primary" @click="handleAdd">
              <el-icon><Plus /></el-icon>
              添加房间
            </el-button>
          </div>
        </div>
      </template>

      <el-table :data="rooms" style="width: 100%" v-loading="loading">
        <el-table-column prop="roomId" label="房间ID" width="100" />
        <el-table-column prop="uname" label="主播" width="120" />
        <el-table-column prop="title" label="房间标题" min-width="200" />
        <el-table-column label="是否上传" width="100">
          <template #default="{ row }">
            <el-tag :type="row.upload ? 'success' : 'info'">
              {{ row.upload ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="line" label="上传线路" width="180">
          <template #default="{ row }">
            {{ formatLine(row.line) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="handleEdit(row)">编辑</el-button>
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download, Upload, Plus } from '@element-plus/icons-vue'
import { roomAPI, userAPI, configAPI } from '@/api'
import RoomEditDialog from '@/components/rooms/RoomEditDialog.vue'
import { useLineTest, formatLine } from '@/composables/useRooms'

const rooms = ref([])
const users = ref([])
const uploadLines = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const dialogTitle = ref('添加房间')
const saving = ref(false)

const form = ref({
  roomId: '',
  upload: true,
  uploadUserId: null,
  titleTemplate: '【直播回放】【${uname}】${title} ${yyyy年MM月dd日HH点mm分}',
  descTemplate: '直播录像\\n${uname}直播间：https://live.bilibili.com/${roomId}',
  tags: '直播回放,${uname},${areaName}',
  tid: 21,
  copyright: 1,
  line: 'CS_UPOS',
  deleteType: 1,
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
  dynamicTemplate: '',
  moveDir: ''
})

// 使用composable
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
  form.value = {
    roomId: '',
    upload: true,
    uploadUserId: users.value[0]?.id || null,
    titleTemplate: '【直播回放】【${uname}】${title} ${yyyy年MM月dd日HH点mm分}',
    descTemplate: '直播录像\\n${uname}直播间：https://live.bilibili.com/${roomId}',
    tags: '直播回放,${uname},${areaName}',
    tid: 21,
    copyright: 1,
    line: 'CS_UPOS',
    deleteType: 1,
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
    dynamicTemplate: '',
    moveDir: ''
  }
  dialogVisible.value = true
}

const handleEdit = (row) => {
  dialogTitle.value = '编辑房间'
  form.value = { ...row }
  dialogVisible.value = true
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
    await ElMessageBox.confirm('确定要删除这个房间吗？', '提示', {
      type: 'warning'
    })
    await roomAPI.delete(row.id)
    ElMessage.success('删除成功')
    fetchRooms()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('删除失败:', error)
    }
  }
}

const handleExport = async () => {
  try {
    const blob = await configAPI.export({
      rooms: true,
      users: true,
      histories: false
    })
    
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `gobup-config-${Date.now()}.json`
    a.click()
    window.URL.revokeObjectURL(url)
    
    ElMessage.success('导出成功')
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
    ElMessageBox.alert(result.result, '模板预览', {
      confirmButtonText: '确定'
    })
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

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 10px;
}
</style>
