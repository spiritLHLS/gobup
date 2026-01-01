<template>
  <el-dialog 
    :model-value="visible"
    title="上传限速配置" 
    width="400px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <el-form label-width="100px">
      <el-form-item label="启用限速">
        <el-switch v-model="localConfig.enabled" />
      </el-form-item>
      <el-form-item label="限速(MB/s)" v-if="localConfig.enabled">
        <el-input-number
          v-model="localConfig.speedMBps"
          :min="1"
          :max="100"
          :step="0.5"
        />
        <div style="margin-top: 8px; font-size: 12px; color: #999;">
          设置上传速度上限，避免占用过多带宽
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button type="primary" @click="handleSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch } from 'vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  config: {
    type: Object,
    default: () => ({
      enabled: false,
      speedMBps: 10
    })
  }
})

const emit = defineEmits(['update:visible', 'save'])

const localConfig = ref({ ...props.config })

watch(() => props.config, (val) => {
  localConfig.value = { ...val }
}, { deep: true })

const handleSave = () => {
  emit('save', localConfig.value)
}
</script>
