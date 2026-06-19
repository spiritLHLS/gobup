<template>
  <div class="line-selector">
    <el-select
      v-model="localLine"
      placeholder="选择线路"
      style="width: 100%"
      :fit-input-width="false"
      popper-class="line-select-dropdown"
    >
      <el-option
        v-for="line in uploadLines"
        :key="line.value"
        :label="line.label"
        :value="line.value"
      >
        <div class="line-option">
          <div class="line-option-main">
            <div class="line-option-title">
              <span class="line-label">{{ line.label }}</span>
              <el-tag v-if="line.recommended" size="small" type="success">推荐</el-tag>
              <el-tag v-if="line.provider" size="small" type="info">{{ line.provider }}</el-tag>
            </div>
            <div class="line-description">{{ line.description }}</div>
          </div>
          <div class="line-option-status" v-if="lineStats[line.value]">
            <i :class="getLineStatusIcon(lineStats[line.value])" :style="{color: getLineStatusColor(lineStats[line.value])}"></i>
            {{ lineStats[line.value] }}
            <span v-if="lineSpeeds[line.value]" class="line-speed">
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
      提示：线路检测采用全并发策略，通常在 10 秒内完成。深度测速将对可用线路逐一上传 2MB 测试数据以确认真实速度。
    </div>
  </div>
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
.line-selector {
  width: 100%;
}

.line-option {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) max-content;
  align-items: center;
  gap: 16px;
  min-width: 0;
  width: 100%;
}

.line-option-main {
  min-width: 0;
}

.line-option-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex-wrap: wrap;
}

.line-label {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
}

.line-description {
  font-size: 12px;
  color: #909399;
  line-height: 1.4;
  margin-top: 2px;
  white-space: normal;
}

.line-option-status {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  min-width: max-content;
  font-size: 12px;
  color: #8492a6;
  white-space: nowrap;
}

.line-speed {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-left: 5px;
  color: #409EFF;
}

.help-text {
  font-size: 12px;
  color: #999;
  margin-top: 5px;
}

.line-test-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

:global(.line-select-dropdown) {
  min-width: min(620px, calc(100vw - 32px)) !important;
}

:global(.line-select-dropdown .el-select-dropdown__item) {
  height: auto;
  min-height: 52px;
  line-height: 1.4;
  padding: 8px 14px;
}

@media (max-width: 640px) {
  .line-option {
    grid-template-columns: 1fr;
    gap: 6px;
  }

  .line-option-status {
    justify-content: flex-start;
    white-space: normal;
  }
}
</style>
