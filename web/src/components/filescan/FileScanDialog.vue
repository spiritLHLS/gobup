<template>
  <el-dialog
    v-model="visible"
    title="强制扫盘 - 选择要入库的文件"
    width="90%"
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <div v-loading="loading" class="file-scan-dialog">
      <!-- 工具栏 -->
      <div class="toolbar">
        <div class="info">
          <span>共 {{ totalFiles }} 个文件</span>
          <span class="divider">|</span>
          <span>已选择 {{ selectedFiles.length }} 个</span>
        </div>
        <div class="actions">
          <el-button 
            size="small" 
            @click="selectAll"
            :disabled="files.length === 0"
          >
            全选
          </el-button>
          <el-button 
            size="small" 
            @click="selectNone"
            :disabled="selectedFiles.length === 0"
          >
            取消全选
          </el-button>
          <el-button 
            size="small" 
            type="primary"
            @click="refreshPreview"
            :icon="Refresh"
          >
            刷新
          </el-button>
        </div>
      </div>

      <!-- 文件列表 -->
      <div class="file-list">
        <el-empty v-if="!loading && files.length === 0" description="没有发现新文件" />
        
        <el-checkbox-group v-else v-model="selectedFiles" class="file-checkbox-group">
          <div 
            v-for="file in files" 
            :key="file.filePath"
            class="file-item"
          >
            <el-checkbox :label="file.filePath" class="file-checkbox">
              <div class="file-info">
                <div class="file-header">
                  <span class="file-name" :title="file.fileName">{{ file.fileName }}</span>
                  <span class="file-size">{{ formatFileSize(file.fileSize) }}</span>
                </div>
                <div class="file-details">
                  <el-tag size="small" type="info">{{ file.roomId }}</el-tag>
                  <span class="uname">{{ file.uname }}</span>
                  <span class="divider">•</span>
                  <span class="mod-time">{{ formatDateTime(file.modTime) }}</span>
                </div>
                <div class="file-path" :title="file.filePath">
                  <el-icon><Folder /></el-icon>
                  <span>{{ file.filePath }}</span>
                </div>
              </div>
            </el-checkbox>
          </div>
        </el-checkbox-group>
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">取消</el-button>
        <el-button 
          type="primary" 
          @click="handleImport"
          :disabled="selectedFiles.length === 0"
          :loading="importing"
        >
          导入选中的文件 ({{ selectedFiles.length }})
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Folder } from '@element-plus/icons-vue'
import { filescanAPI } from '@/api'

const visible = ref(false)
const loading = ref(false)
const importing = ref(false)
const files = ref([])
const selectedFiles = ref([])

const totalFiles = computed(() => files.value.length)

// 打开对话框
const open = async () => {
  visible.value = true
  await loadPreview()
}

// 加载预览
const loadPreview = async () => {
  loading.value = true
  try {
    const response = await filescanAPI.preview()
    if (response.type === 'success') {
      files.value = response.files || []
      selectedFiles.value = [] // 清空选择
      
      if (files.value.length === 0) {
        ElMessage.info('没有发现新文件')
      }
    } else {
      ElMessage.error(response.msg || '加载失败')
    }
  } catch (error) {
    console.error('加载预览失败:', error)
    ElMessage.error('加载失败: ' + (error.message || '网络错误'))
  } finally {
    loading.value = false
  }
}

// 刷新预览
const refreshPreview = () => {
  loadPreview()
}

// 全选
const selectAll = () => {
  selectedFiles.value = files.value.map(f => f.filePath)
}

// 取消全选
const selectNone = () => {
  selectedFiles.value = []
}

// 导入选中的文件
const handleImport = async () => {
  if (selectedFiles.value.length === 0) {
    ElMessage.warning('请至少选择一个文件')
    return
  }

  try {
    await ElMessageBox.confirm(
      `确定要导入选中的 ${selectedFiles.value.length} 个文件吗？`,
      '确认导入',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'info'
      }
    )

    importing.value = true
    const response = await filescanAPI.import(selectedFiles.value)

    if (response.type === 'success') {
      const message = `导入完成！成功: ${response.newFiles}, 失败: ${response.failedFiles}`
      
      if (response.failedFiles > 0 && response.errors && response.errors.length > 0) {
        ElMessageBox.alert(
          message + '\n\n失败文件：\n' + response.errors.join('\n'),
          '导入结果',
          { type: 'warning' }
        )
      } else {
        ElMessage.success(message)
      }
      
      // 导入成功后关闭对话框
      visible.value = false
      // 触发刷新事件
      emit('imported')
    } else {
      ElMessage.error(response.msg || '导入失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('导入失败:', error)
      ElMessage.error('导入失败: ' + (error.message || '网络错误'))
    }
  } finally {
    importing.value = false
  }
}

// 关闭对话框
const handleClose = () => {
  visible.value = false
  files.value = []
  selectedFiles.value = []
}

// 格式化文件大小
const formatFileSize = (bytes) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
}

// 格式化日期时间
const formatDateTime = (dateStr) => {
  const date = new Date(dateStr)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

const emit = defineEmits(['imported'])

// 暴露方法给父组件
defineExpose({
  open
})
</script>

<style scoped lang="scss">
.file-scan-dialog {
  min-height: 400px;
  max-height: 60vh;
  display: flex;
  flex-direction: column;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background-color: var(--bg-color-tertiary);
  border-radius: var(--border-radius-base);
  margin-bottom: var(--spacing-md);

  .info {
    display: flex;
    gap: var(--spacing-sm);
    color: var(--text-color-secondary);
    font-size: var(--font-size-sm);

    .divider {
      color: var(--border-color-base);
    }
  }

  .actions {
    display: flex;
    gap: var(--spacing-sm);
  }
}

.file-list {
  flex: 1;
  overflow-y: auto;
  border: 1px solid var(--border-color-base);
  border-radius: var(--border-radius-base);
  padding: var(--spacing-md);
}

.file-checkbox-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  width: 100%;
}

.file-item {
  padding: 12px;
  border: 1px solid var(--border-color-light);
  border-radius: var(--border-radius-base);
  transition: all 0.3s;

  &:hover {
    border-color: var(--primary-color);
    background-color: var(--bg-color-tertiary);
  }
}

.file-checkbox {
  width: 100%;

  :deep(.el-checkbox__label) {
    width: 100%;
    display: block;
  }
}

.file-info {
  width: 100%;
}

.file-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;

  .file-name {
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-medium);
    color: var(--text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 70%;
  }

  .file-size {
    font-size: var(--font-size-sm);
    color: var(--text-color-secondary);
    font-weight: var(--font-weight-medium);
  }
}

.file-details {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: 6px;
  font-size: var(--font-size-sm);

  .uname {
    color: var(--text-color-primary);
  }

  .divider {
    color: var(--border-color-base);
  }

  .mod-time {
    color: var(--text-color-secondary);
  }
}

.file-path {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--font-size-xs);
  color: var(--text-color-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  .el-icon {
    flex-shrink: 0;
  }

  span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
}

:deep(.el-dialog__body) {
  padding: var(--spacing-lg);
}

@media (max-width: 768px) {
  .file-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;

    .file-name {
      max-width: 100%;
    }
  }

  .toolbar {
    flex-direction: column;
    gap: var(--spacing-sm);
    align-items: stretch;

    .actions {
      justify-content: flex-end;
    }
  }
}
</style>
