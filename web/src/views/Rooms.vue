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
        <el-table-column prop="line" label="上传线路" width="120" />
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="handleEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="800px"
    >
      <el-form :model="form" label-width="120px">
        <el-form-item label="房间ID" required>
          <el-input v-model="form.roomId" />
        </el-form-item>
        <el-form-item label="是否上传">
          <el-switch v-model="form.upload" />
        </el-form-item>
        <el-form-item label="上传用户">
          <el-select v-model="form.uploadUserId" placeholder="请选择用户">
            <el-option
              v-for="user in users"
              :key="user.id"
              :label="user.name"
              :value="user.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="标题模板">
          <el-input v-model="form.titleTemplate" type="textarea" :rows="2" />
          <div class="help-text">支持变量: ${uname} ${title} ${yyyy年MM月dd日HH点mm分} ${roomId} ${areaName}</div>
        </el-form-item>
        <el-form-item label="简介模板">
          <el-input v-model="form.descTemplate" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="form.tags" placeholder="多个标签用逗号分隔" />
        </el-form-item>
        <el-form-item label="分区ID">
          <el-input-number v-model="form.tid" :min="1" />
        </el-form-item>
        <el-form-item label="版权">
          <el-radio-group v-model="form.copyright">
            <el-radio :label="1">自制</el-radio>
            <el-radio :label="2">转载</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="上传线路">
          <el-select v-model="form.line" placeholder="选择线路" style="width: 100%">
            <el-option
              v-for="line in uploadLines"
              :key="line.value"
              :label="line.label"
              :value="line.value"
            >
              <span style="float: left">{{ line.label }}</span>
              <span style="float: right; font-size: 12px; color: #8492a6" v-if="lineStats[line.value]">
                <i :class="getLineStatusIcon(lineStats[line.value])" :style="{color: getLineStatusColor(lineStats[line.value])}"></i>
                {{ lineStats[line.value] }}
                <span v-if="lineSpeeds[line.value]" style="margin-left: 5px; color: #409EFF">
                  <el-icon><Upload /></el-icon> {{ lineSpeeds[line.value] }}
                </span>
              </span>
            </el-option>
          </el-select>
          <div class="line-test-actions" style="margin-top: 10px;">
            <el-button size="small" @click="testLines" :loading="testingLines">
              <el-icon><Connection /></el-icon>
              {{ testingLines ? '测速中...' : '检测线路' }}
            </el-button>
            <el-button size="small" @click="testDeepSpeed" :loading="testingDeepSpeed" :disabled="testingLines">
              <el-icon><Odometer /></el-icon>
              {{ testingDeepSpeed ? '深度测速中...' : '深度测速' }}
            </el-button>
          </div>
          <div class="help-text" style="margin-top: 5px;">
            提示：线路检测采用分批限流策略，避免触发风控。深度测速将逐条测试，耗时较长。
          </div>
        </el-form-item>
        <el-form-item label="文件处理">
          <el-radio-group v-model="form.deleteType">
            <el-radio :label="0">不删除</el-radio>
            <el-radio :label="1">上传后删除</el-radio>
            <el-radio :label="2">审核后删除</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="分P标题模板">
          <el-input v-model="form.partTitleTemplate" />
          <div class="help-text">支持变量: ${index} ${MM月dd日HH点mm分} ${areaName}</div>
        </el-form-item>
        <el-form-item label="WxPusher UID">
          <el-input v-model="form.wxuid" placeholder="填写后将推送通知" />
          <div class="help-text">获取地址: https://wxpusher.zjiecode.com/</div>
        </el-form-item>
        <el-form-item label="推送消息类型">
          <el-checkbox-group v-model="pushTags">
            <el-checkbox label="开播">开播通知</el-checkbox>
            <el-checkbox label="上传">上传通知</el-checkbox>
            <el-checkbox label="投稿">投稿通知</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSave" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { roomAPI, userAPI, configAPI } from '@/api'

const rooms = ref([])
const users = ref([])
const uploadLines = ref([])
const lineStats = ref({})
const lineSpeeds = ref({})
const loading = ref(false)
const dialogVisible = ref(false)
const dialogTitle = ref('添加房间')
const saving = ref(false)
const testingLines = ref(false)
const testingDeepSpeed = ref(false)
const pushTags = ref(['开播', '上传', '投稿'])

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
  pushMsgTags: '开播,上传,投稿'
})

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
    users.value = data || []
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

const testLines = async () => {
  try {
    await ElMessageBox.confirm(
      '线路检测将分批测试30+条线路，为避免触发风控，测试过程约需30-45秒，是否继续？',
      '提示',
      {
        confirmButtonText: '开始检测',
        cancelButtonText: '取消',
        type: 'info'
      }
    )
  } catch {
    return
  }

  testingLines.value = true
  lineStats.value = {}
  lineSpeeds.value = {}
  
  ElMessage.info('开始检测线路，请耐心等待约30-45秒...')
  
  try {
    const data = await roomAPI.testLines()
    lineStats.value = data || {}
    ElMessage.success('线路检测完成')
  } catch (error) {
    console.error('线路检测失败:', error)
    ElMessage.error('线路检测失败')
  } finally {
    testingLines.value = false
  }
}

const testDeepSpeed = async () => {
  if (Object.keys(lineStats.value).length === 0) {
    ElMessage.warning('请先进行普通线路检测')
    return
  }
  
  // 筛选出可用的线路（非 Error/Unknown/Timeout）
  const availableLines = Object.keys(lineStats.value).filter(line => {
    const status = lineStats.value[line]
    return status && status.includes('ms')
  })
  
  if (availableLines.length === 0) {
    ElMessage.warning('没有可用的线路进行深度测速')
    return
  }

  try {
    await ElMessageBox.confirm(
      `将对${availableLines.length}条可用线路进行真实上传测速（1MB数据），为避免风控每条线路间隔2秒，预计耗时${Math.ceil(availableLines.length * 2 / 60)}分钟，是否继续？`,
      '深度测速确认',
      {
        confirmButtonText: '开始测速',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    return
  }
  
  testingDeepSpeed.value = true
  lineSpeeds.value = {}
  
  ElMessage.info(`开始深度测速，将测试${availableLines.length}条线路，请耐心等待...`)
  
  for (let i = 0; i < availableLines.length; i++) {
    const line = availableLines[i]
    lineSpeeds.value[line] = `测速中(${i+1}/${availableLines.length})...`
    
    try {
      const result = await roomAPI.testSpeed(line)
      if (result.success) {
        lineSpeeds.value[line] = result.speed
      } else {
        lineSpeeds.value[line] = '失败'
      }
    } catch (error) {
      lineSpeeds.value[line] = '失败'
    }

    // 间隔2秒，避免风控
    if (i < availableLines.length - 1) {
      await new Promise(resolve => setTimeout(resolve, 2000))
    }
  }
  
  testingDeepSpeed.value = false
  ElMessage.success('深度测速完成')
}

const getLineStatusColor = (status) => {
  if (!status) return ''
  if (status.includes('ms')) {
    const ms = parseInt(status)
    if (ms < 200) return '#67C23A' // Green
    if (ms < 500) return '#E6A23C' // Yellow
    return '#F56C6C' // Red
  }
  return '#F56C6C' // Error
}

const getLineStatusIcon = (status) => {
  if (!status) return ''
  if (status.includes('ms')) return 'el-icon-success'
  return 'el-icon-error'
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
    partTitleTemplate: 'P${index}-${areaName}-${MM月dd日HH点mm分}'
  }
  dialogVisible.value = true
}

const handleEdit = (row) => {
  dialogTitle.value = '编辑房间'
  form.value = { ...row }
  pushTags.value = row.pushMsgTags ? row.pushMsgTags.split(',') : []
  dialogVisible.value = true
}

const handleSave = async () => {
  if (!form.value.roomId) {
    ElMessage.warning('请输入房间ID')
    return
  }
  
  // 组装推送标签
  form.value.pushMsgTags = pushTags.value.join(',')
  
  saving.value = true
  try {
    await roomAPI.update(form.value)
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

.help-text {
  font-size: 12px;
  color: #999;
  margin-top: 5px;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.line-test-actions {
  display: flex;
  gap: 10px;
}
</style>
