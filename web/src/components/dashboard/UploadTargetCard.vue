<template>
  <el-card class="upload-target-card" v-loading="loading">
    <template #header>
      <div class="card-header">
        <span>上传执行目标</span>
        <div class="header-actions">
          <el-button plain :icon="Refresh" :loading="refreshing" @click="loadTargets(true)">
            刷新状态
          </el-button>
        </div>
      </div>
    </template>

    <div class="target-list">
      <div
        v-for="target in targets"
        :key="targetKey(target)"
        class="target-row"
        :class="{ current: target.current, disabled: !target.available }"
      >
        <div class="target-main">
          <div class="target-title">
            <el-icon class="target-icon">
              <component :is="target.targetType === 'local' ? Monitor : Connection" />
            </el-icon>
            <span>{{ target.name || target.endpoint || '未命名 Agent' }}</span>
            <el-tag v-if="target.current" type="success" size="small">当前</el-tag>
            <el-tag v-if="target.targetType === 'agent'" type="info" size="small" effect="plain">
              优先级 {{ target.priority ?? 50 }}
            </el-tag>
          </div>
          <div class="target-meta">
            <span>{{ target.targetType === 'local' ? '本地' : target.endpoint }}</span>
            <el-tag :type="statusTagType(target)" size="small" effect="plain">
              {{ statusLabel(target) }}
            </el-tag>
            <span v-if="target.message" class="target-message">{{ target.message }}</span>
          </div>
        </div>
        <el-button
          type="primary"
          plain
          :disabled="target.current || !target.available"
          :loading="selectingKey === targetKey(target)"
          @click="selectTarget(target)"
        >
          使用
        </el-button>
      </div>
    </div>
  </el-card>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, Monitor, Refresh } from '@element-plus/icons-vue'
import { agentAPI } from '@/api'

const emit = defineEmits(['changed'])

const targets = ref([])
const loading = ref(false)
const refreshing = ref(false)
const selectingKey = ref('')

const targetKey = (target) => `${target.targetType}:${target.id || 0}`

const loadTargets = async (refresh = false) => {
  if (refresh) {
    refreshing.value = true
  } else {
    loading.value = true
  }
  try {
    const response = await agentAPI.uploadTargets(refresh)
    if (response.type === 'success') {
      targets.value = Array.isArray(response.data?.targets) ? response.data.targets : []
    } else {
      ElMessage.error(response.msg || '加载上传目标失败')
    }
  } catch (error) {
    console.error('加载上传目标失败:', error)
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

const selectTarget = async (target) => {
  if (!target?.available || target.current) return
  selectingKey.value = targetKey(target)
  try {
    const payload = target.targetType === 'local'
      ? { targetType: 'local' }
      : { targetType: 'agent', agentId: target.id }
    const response = await agentAPI.selectUploadTarget(payload)
    if (response.type === 'success') {
      ElMessage.success(response.msg || '上传目标已切换')
      await loadTargets(false)
      emit('changed')
    } else {
      ElMessage.error(response.msg || '切换上传目标失败')
      await loadTargets(false)
    }
  } catch (error) {
    console.error('切换上传目标失败:', error)
  } finally {
    selectingKey.value = ''
  }
}

const statusLabel = (target) => {
  if (target.available) return target.targetType === 'local' ? '可用' : '在线'
  return target.disabledReason || target.message || '不可用'
}

const statusTagType = (target) => {
  if (target.available) return 'success'
  if (target.status === 'error') return 'danger'
  return 'info'
}

onMounted(() => loadTargets(true))
</script>

<style scoped lang="scss">
.upload-target-card {
  margin: var(--spacing-lg) 0;
}

.card-header,
.header-actions,
.target-title,
.target-meta,
.target-row {
  display: flex;
  align-items: center;
}

.card-header {
  justify-content: space-between;
  gap: var(--spacing-md);

  > span {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--text-color-primary);
  }
}

.target-list {
  display: grid;
  gap: var(--spacing-sm);
}

.target-row {
  min-height: 76px;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--bg-color-secondary);
}

.target-row.current {
  border-color: var(--primary-color);
  background: var(--bg-color-hover);
}

.target-row.disabled {
  opacity: 0.72;
}

.target-main {
  min-width: 0;
  flex: 1;
}

.target-title {
  gap: var(--spacing-sm);
  font-weight: var(--font-weight-semibold);
  color: var(--text-color-primary);
  min-width: 0;

  span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.target-icon {
  color: var(--primary-color);
}

.target-meta {
  gap: var(--spacing-sm);
  margin-top: 6px;
  color: var(--text-color-secondary);
  font-size: var(--font-size-sm);
  min-width: 0;
  flex-wrap: wrap;

  > span:first-child {
    max-width: 360px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.target-message {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .card-header,
  .target-row {
    align-items: stretch;
    flex-direction: column;
  }

  .target-row :deep(.el-button),
  .header-actions :deep(.el-button) {
    width: 100%;
  }
}
</style>
