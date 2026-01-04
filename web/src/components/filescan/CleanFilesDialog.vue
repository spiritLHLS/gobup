<template>
  <el-dialog
    v-model="dialogVisible"
    title="清理已完成文件"
    width="900px"
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <div v-loading="loading">
      <el-alert
        type="info"
        :closable="false"
        style="margin-bottom: 15px"
      >
        <template #default>
          <div>以下是所有已上传投稿成功且解析弹幕完成且已发送弹幕的历史记录的xml和jpg文件</div>
          <div style="margin-top: 5px">请勾选需要删除的文件，未勾选的文件将保留</div>
        </template>
      </el-alert>

      <div class="stats-bar">
        <el-tag type="info">总计: {{ filesToClean.length }} 个文件</el-tag>
        <el-tag type="primary">已选择: {{ selectedFiles.length }} 个文件</el-tag>
        <el-tag type="warning">总大小: {{ formatSize(totalSize) }}</el-tag>
        <el-tag type="success">选中大小: {{ formatSize(selectedSize) }}</el-tag>
      </div>

      <div class="filter-bar">
        <el-input
          v-model="filterText"
          placeholder="搜索房间名称、标题或文件路径"
          clearable
          style="width: 300px"
          :prefix-icon="Search"
        />
        <el-select
          v-model="filterType"
          placeholder="文件类型"
          clearable
          style="width: 120px; margin-left: 10px"
        >
          <el-option label="全部" value="" />
          <el-option label="XML" value="xml" />
          <el-option label="JPG" value="jpg" />
        </el-select>
        <div style="flex: 1"></div>
        <el-button @click="selectAll" :icon="Select">全选</el-button>
        <el-button @click="selectNone" :icon="CloseBold">取消全选</el-button>
      </div>

      <el-table
        ref="tableRef"
        :data="filteredFiles"
        style="width: 100%"
        max-height="500px"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="55" />
        <el-table-column label="文件类型" width="80">
          <template #default="{ row }">
            <el-tag :type="row.fileType === 'xml' ? 'primary' : 'success'" size="small">
              {{ row.fileType.toUpperCase() }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="房间名称" width="120" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.roomName }}
          </template>
        </el-table-column>
        <el-table-column label="标题" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.title }}
          </template>
        </el-table-column>
        <el-table-column label="录制时间" width="160">
          <template #default="{ row }">
            {{ row.recordTime }}
          </template>
        </el-table-column>
        <el-table-column label="文件大小" width="100" align="right">
          <template #default="{ row }">
            {{ formatSize(row.fileSize) }}
          </template>
        </el-table-column>
        <el-table-column label="文件路径" min-width="250" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="file-path">{{ row.filePath }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <template #footer>
      <span class="dialog-footer">
        <el-button @click="handleClose">取消</el-button>
        <el-button
          type="danger"
          @click="handleDelete"
          :disabled="selectedFiles.length === 0"
          :loading="deleting"
        >
          删除选中的文件 ({{ selectedFiles.length }})
        </el-button>
      </span>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, Select, CloseBold } from '@element-plus/icons-vue'
import { filescanAPI } from '@/api'

const dialogVisible = ref(false)
const loading = ref(false)
const deleting = ref(false)
const filesToClean = ref([])
const selectedFiles = ref([])
const filterText = ref('')
const filterType = ref('')
const tableRef = ref(null)

const emit = defineEmits(['success'])

// 过滤后的文件列表
const filteredFiles = computed(() => {
  let files = filesToClean.value

  // 按文件类型过滤
  if (filterType.value) {
    files = files.filter(f => f.fileType === filterType.value)
  }

  // 按文本过滤
  if (filterText.value) {
    const text = filterText.value.toLowerCase()
    files = files.filter(f => 
      f.roomName.toLowerCase().includes(text) ||
      f.title.toLowerCase().includes(text) ||
      f.filePath.toLowerCase().includes(text)
    )
  }

  return files
})

// 总大小
const totalSize = computed(() => {
  return filesToClean.value.reduce((sum, file) => sum + file.fileSize, 0)
})

// 选中文件的总大小
const selectedSize = computed(() => {
  return selectedFiles.value.reduce((sum, file) => sum + file.fileSize, 0)
})

// 格式化文件大小
const formatSize = (bytes) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

// 打开对话框
const open = async () => {
  dialogVisible.value = true
  loading.value = true
  
  try {
    const response = await filescanAPI.cleanPreview()
    
    if (response.type === 'success') {
      filesToClean.value = response.filesToClean || []
      
      if (filesToClean.value.length === 0) {
        ElMessage.info('没有找到可清理的文件')
        dialogVisible.value = false
      }
    } else {
      ElMessage.error(response.msg || '获取文件列表失败')
      dialogVisible.value = false
    }
  } catch (error) {
    console.error('获取文件列表失败:', error)
    ElMessage.error('获取文件列表失败')
    dialogVisible.value = false
  } finally {
    loading.value = false
  }
}

// 处理选择变化
const handleSelectionChange = (selection) => {
  selectedFiles.value = selection
}

// 全选
const selectAll = () => {
  filteredFiles.value.forEach(row => {
    tableRef.value.toggleRowSelection(row, true)
  })
}

// 取消全选
const selectNone = () => {
  tableRef.value.clearSelection()
}

// 删除选中的文件
const handleDelete = async () => {
  try {
    await ElMessageBox.confirm(
      `确定要删除选中的 ${selectedFiles.value.length} 个文件吗？\n\n` +
      `XML文件: ${selectedFiles.value.filter(f => f.fileType === 'xml').length} 个\n` +
      `JPG文件: ${selectedFiles.value.filter(f => f.fileType === 'jpg').length} 个\n` +
      `总大小: ${formatSize(selectedSize.value)}\n\n` +
      `注意：此操作不可恢复！`,
      '确认删除',
      {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'warning',
        distinguishCancelAndClose: true
      }
    )

    deleting.value = true

    const filePaths = selectedFiles.value.map(f => f.filePath)
    const response = await filescanAPI.cleanSelected(filePaths)

    if (response.type === 'success') {
      let message = `清理完成！\n\n`
      message += `删除XML文件: ${response.deletedXMLFiles} 个\n`
      message += `删除JPG文件: ${response.deletedJPGFiles} 个\n`

      if (response.errors && response.errors.length > 0) {
        message += `\n错误信息：\n` + response.errors.join('\n')
        ElMessageBox.alert(message, '清理结果', {
          type: 'warning',
          confirmButtonText: '知道了'
        })
      } else {
        ElMessage.success(message)
      }

      emit('success')
      dialogVisible.value = false
    } else {
      ElMessage.error(response.msg || '删除失败')
    }
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      console.error('删除失败:', error)
      ElMessage.error('删除失败')
    }
  } finally {
    deleting.value = false
  }
}

// 关闭对话框
const handleClose = () => {
  dialogVisible.value = false
  filesToClean.value = []
  selectedFiles.value = []
  filterText.value = ''
  filterType.value = ''
}

// 监听过滤条件变化，重置选择
watch([filterText, filterType], () => {
  tableRef.value?.clearSelection()
})

defineExpose({
  open
})
</script>

<style scoped>
.stats-bar {
  display: flex;
  gap: 10px;
  margin-bottom: 15px;
  flex-wrap: wrap;
}

.filter-bar {
  display: flex;
  align-items: center;
  margin-bottom: 15px;
}

.file-path {
  font-family: monospace;
  font-size: 12px;
  color: #606266;
}

:deep(.el-table) {
  font-size: 13px;
}

:deep(.el-table__header th) {
  background-color: #f5f7fa;
}
</style>
