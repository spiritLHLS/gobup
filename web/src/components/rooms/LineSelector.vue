<template>
  <el-form-item label="上传线路">
    <el-select v-model="localLine" placeholder="选择线路" style="width: 100%">
      <el-option
        v-for="line in uploadLines"
        :key="line.value"
        :label="line.label"
        :value="line.value"
      >
        <div style="display: flex; align-items: center; justify-content: space-between;">
          <div style="flex: 1; overflow: hidden;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <span style="font-weight: 500;">{{ line.label }}</span>
              <el-tag v-if="line.recommended" size="small" type="success">推荐</el-tag>
              <el-tag v-if="line.provider" size="small" type="info">{{ line.provider }}</el-tag>
            </div>
            <div style="font-size: 12px; color: #909399; margin-top: 2px;">{{ line.description }}</div>
          </div>
          <div style="flex-shrink: 0; margin-left: 10px; font-size: 12px; color: #8492a6;" v-if="lineStats[line.value]">
            <i :class="getLineStatusIcon(lineStats[line.value])" :style="{color: getLineStatusColor(lineStats[line.value])}"></i>
            {{ lineStats[line.value] }}
            <span v-if="lineSpeeds[line.value]" style="margin-left: 5px; color: #409EFF">
              <el-icon><Upload /></el-icon> {{ lineSpeeds[line.value] }}
            </span>
          </div>
        </div>
      </el-option>
    </el-select>
    <div class="line-test-actions" style="margin-top: 10px;">
      <el-button size="small" @click="$emit('test-lines')" :loading="testingLines">
        <el-icon><Connection /></el-icon>
        {{ testingLines ? '测速中...' : '检测线路' }}
      </el-button>
      <el-button size="small" @click="$emit('test-deep-speed')" :loading="testingDeepSpeed" :disabled="testingLines">
        <el-icon><Odometer /></el-icon>
        {{ testingDeepSpeed ? '深度测速中...' : '深度测速' }}
      </el-button>
    </div>
    <div class="help-text" style="margin-top: 5px;">
      提示：线路检测采用分批限流策略，避免触发风控。深度测速将逐条测试，耗时较长。
    </div>
  </el-form-item>
</template>

<script setup>
import { ref, watch } from 'vue'
import { Upload, Connection, Odometer } from '@element-plus/icons-vue'

const props = defineProps({
  line: {
    type: String,
    default: 'CS_UPOS'
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

const emit = defineEmits(['update:line', 'test-lines', 'test-deep-speed'])

const localLine = ref(props.line)

watch(localLine, (val) => {
  emit('update:line', val)
})

watch(() => props.line, (val) => {
  localLine.value = val
})

const getLineStatusColor = (status) => {
  if (!status) return ''
  if (status.includes('ms')) {
    const ms = parseInt(status)
    if (ms < 200) return '#67C23A'
    if (ms < 500) return '#E6A23C'
    return '#F56C6C'
  }
  return '#F56C6C'
}

const getLineStatusIcon = (status) => {
  if (!status) return ''
  if (status.includes('ms')) return 'el-icon-success'
  return 'el-icon-error'
}
</script>

<style scoped>
.help-text {
  font-size: 12px;
  color: #999;
  margin-top: 5px;
}

.line-test-actions {
  display: flex;
  gap: 10px;
}
</style>
