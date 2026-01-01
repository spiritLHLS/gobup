<template>
  <el-dialog
    :model-value="visible"
    :title="isBatch ? '批量重置状态选项' : '重置状态选项'"
    width="500px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <div style="margin-bottom: 20px; color: #666;">
      <el-icon><InfoFilled /></el-icon>
      请选择要重置的状态项：
    </div>
    <el-form label-width="120px">
      <el-form-item label="上传状态">
        <el-checkbox v-model="localOptions.upload">
          将所有分P标记为未上传，清除CID等上传信息
        </el-checkbox>
      </el-form-item>
      <el-form-item label="投稿状态">
        <el-checkbox v-model="localOptions.publish">
          标记为未投稿，清除BV号、AV号等投稿信息
        </el-checkbox>
      </el-form-item>
      <el-form-item label="弹幕状态">
        <el-checkbox v-model="localOptions.danmaku">
          标记为未发送弹幕
        </el-checkbox>
      </el-form-item>
      <el-form-item label="文件状态">
        <el-checkbox v-model="localOptions.files">
          标记为未移动文件
        </el-checkbox>
      </el-form-item>
    </el-form>
    <div style="margin-top: 20px; padding: 12px; background: #fff3cd; border-radius: 4px; color: #856404;">
      <el-icon><Warning /></el-icon>
      <span style="margin-left: 8px;">提示：重置后需要重新执行相应的操作</span>
    </div>
    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button 
        type="primary" 
        @click="handleConfirm"
        :disabled="!localOptions.upload && !localOptions.publish && !localOptions.danmaku && !localOptions.files"
      >
        确定重置
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch } from 'vue'
import { InfoFilled, Warning } from '@element-plus/icons-vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  options: {
    type: Object,
    default: () => ({
      upload: true,
      publish: true,
      danmaku: true,
      files: true
    })
  },
  isBatch: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:visible', 'confirm'])

const localOptions = ref({ ...props.options })

watch(() => props.options, (newVal) => {
  localOptions.value = { ...newVal }
}, { deep: true })

const handleConfirm = () => {
  emit('confirm', localOptions.value)
}
</script>
