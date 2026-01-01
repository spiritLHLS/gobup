<template>
  <el-dialog
    :model-value="visible"
    :title="title"
    width="900px"
    @update:model-value="handleClose"
    class="room-edit-dialog"
  >
    <el-tabs v-model="activeTab" class="dialog-tabs">
      <el-tab-pane label="基本信息" name="basic">
        <BasicInfoTab
          v-model="localForm"
          :users="users"
        />
      </el-tab-pane>
      
      <el-tab-pane label="上传设置" name="upload">
        <UploadSettingsTab
          v-model="localForm"
          :upload-lines="uploadLines"
          :line-stats="lineStats"
          :line-speeds="lineSpeeds"
          :testing-lines="testingLines"
          :testing-deep-speed="testingDeepSpeed"
          @test-lines="$emit('test-lines')"
          @test-deep-speed="$emit('test-deep-speed')"
          @preview-template="$emit('preview-template', localForm)"
        />
      </el-tab-pane>
      
      <el-tab-pane label="视频处理" name="processing">
        <VideoProcessingTab
          v-model="localForm"
        />
      </el-tab-pane>
      
      <el-tab-pane label="通知设置" name="notification">
        <NotificationSettingsTab
          v-model="localForm"
        />
      </el-tab-pane>
    </el-tabs>
    
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">取消</el-button>
        <el-button type="primary" @click="handleSave" :loading="saving">
          保存
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import BasicInfoTab from './tabs/BasicInfoTab.vue'
import UploadSettingsTab from './tabs/UploadSettingsTab.vue'
import VideoProcessingTab from './tabs/VideoProcessingTab.vue'
import NotificationSettingsTab from './tabs/NotificationSettingsTab.vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  title: {
    type: String,
    default: '编辑房间'
  },
  form: {
    type: Object,
    required: true
  },
  users: {
    type: Array,
    default: () => []
  },
  uploadLines: {
    type: Array,
    default: () => []
  },
  lineStats: {
    type: Object,
    default: () => ({})
  },
  lineSpeeds: {
    type: Object,
    default: () => ({})
  },
  testingLines: {
    type: Boolean,
    default: false
  },
  testingDeepSpeed: {
    type: Boolean,
    default: false
  },
  saving: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits([
  'update:visible',
  'save',
  'test-lines',
  'test-deep-speed',
  'preview-template'
])

const activeTab = ref('basic')
const localForm = ref({ ...props.form })

// 监听props.form变化，更新localForm，但保持引用
watch(() => props.form, (val) => {
  // 不直接替换对象，而是更新属性，这样各个tab组件的v-model能正常工作
  Object.keys(val).forEach(key => {
    localForm.value[key] = val[key]
  })
}, { deep: true })

// 监听visible变化，重置tab
watch(() => props.visible, (val) => {
  if (val) {
    activeTab.value = 'basic'
    localForm.value = { ...props.form }
  }
})

const handleClose = () => {
  emit('update:visible', false)
}

const handleSave = () => {
  if (!localForm.value.roomId) {
    ElMessage.warning('请输入房间ID')
    return
  }
  
  emit('save', localForm.value)
}
</script>

<style scoped>
.room-edit-dialog :deep(.el-dialog__body) {
  padding: 10px 20px 20px;
  max-height: 70vh;
  overflow: hidden;
}

.dialog-tabs {
  height: 100%;
}

.dialog-tabs :deep(.el-tabs__content) {
  max-height: calc(70vh - 100px);
  overflow-y: auto;
  padding: 0 10px;
}

.dialog-tabs :deep(.el-tabs__header) {
  margin-bottom: 15px;
}

.dialog-tabs :deep(.el-tabs__nav-wrap::after) {
  height: 1px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

/* 滚动条样式优化 */
.dialog-tabs :deep(.el-tabs__content)::-webkit-scrollbar {
  width: 6px;
}

.dialog-tabs :deep(.el-tabs__content)::-webkit-scrollbar-track {
  background: #f1f1f1;
  border-radius: 3px;
}

.dialog-tabs :deep(.el-tabs__content)::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 3px;
}

.dialog-tabs :deep(.el-tabs__content)::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8;
}
</style>
