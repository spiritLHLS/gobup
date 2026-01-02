<template>
  <div class="upload-settings-tab">
    <el-form :model="localForm" label-width="120px">
      <el-form-item label="上传线路">
        <LineSelector
          v-model:line="localForm.line"
          :line-stats="lineStats"
          :line-speeds="lineSpeeds"
          :upload-lines="uploadLines"
          :testing-lines="testingLines"
          :testing-deep-speed="testingDeepSpeed"
          @test-lines="$emit('test-lines')"
          @test-deep-speed="$emit('test-deep-speed')"
        />
      </el-form-item>
      
      <el-divider content-position="left">封面设置</el-divider>
      
      <el-form-item label="封面配置">
        <el-radio-group v-model="localForm.coverType">
          <el-radio label="default">不使用封面</el-radio>
          <el-radio label="live">使用直播首帧</el-radio>
          <el-radio label="diy">自定义封面</el-radio>
        </el-radio-group>
      </el-form-item>
      
      <el-form-item v-if="localForm.coverType === 'diy'" label="封面地址">
        <div class="upload-area">
          <el-input
            v-model="localForm.coverUrl"
            placeholder="请输入图片URL地址，例如：https://example.com/cover.jpg"
            clearable
          />
          <div class="help-text">输入图片URL地址，建议尺寸：960x600</div>
          <div v-if="localForm.coverUrl" class="cover-preview">
            <img :src="localForm.coverUrl" alt="封面预览" @error="handleImageError" />
          </div>
        </div>
      </el-form-item>
      
      <el-divider content-position="left">动态设置</el-divider>
      
      <el-form-item label="动态模板">
        <el-input 
          v-model="localForm.dynamicTemplate" 
          type="textarea" 
          :rows="3"
          placeholder="投稿成功后将发送的动态内容"
        />
        <div class="help-text">
          支持变量: ${uname} ${title} ${roomId} ${bvid}
        </div>
        <el-button 
          size="small" 
          @click="$emit('preview-template', localForm)" 
          style="margin-top: 10px;"
        >
          <el-icon><View /></el-icon>
          预览效果
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { View } from '@element-plus/icons-vue'
import LineSelector from '../LineSelector.vue'

const props = defineProps({
  modelValue: {
    type: Object,
    required: true
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
  }
})

const emit = defineEmits([
  'update:modelValue',
  'test-lines',
  'test-deep-speed',
  'preview-template'
])

const localForm = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const handleImageError = (e) => {
  console.error('封面图片加载失败:', e)
  ElMessage.warning('封面图片加载失败，请检查URL是否正确')
}
</script>

<style scoped>
.upload-settings-tab {
  padding: 20px 0;
}

.help-text {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.5;
}

.upload-area {
  width: 100%;
}

.cover-preview {
  margin-top: 15px;
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  padding: 10px;
  background-color: #f5f7fa;
}

.cover-preview img {
  max-width: 100%;
  max-height: 300px;
  display: block;
  border-radius: 4px;
}

:deep(.el-divider__text) {
  font-weight: 500;
  color: #303133;
}
</style>
