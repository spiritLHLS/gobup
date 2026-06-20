<template>
  <div class="log-container">
    <el-card class="log-card">
      <template #header>
        <div class="log-header">
          <div class="log-title">
            <el-icon><Document /></el-icon>
            系统日志
            <span class="log-count">显示 {{ filteredLogs.length }} / {{ logs.length }} 行</span>
          </div>
          <div class="log-controls">
            <el-input
              v-model="searchKeyword"
              placeholder="搜索日志..."
              size="small"
              clearable
              class="control-search"
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
            <el-select
              v-model="levelFilter"
              multiple
              collapse-tags
              placeholder="日志级别"
              size="small"
              class="control-level"
            >
              <el-option label="INFO" value="INFO" />
              <el-option label="WARN" value="WARN" />
              <el-option label="ERROR" value="ERROR" />
              <el-option label="DEBUG" value="DEBUG" />
            </el-select>
            <el-select v-model="lineLimit" size="small" class="control-limit" @change="fetchLogs">
              <el-option label="最新 100 行" :value="100" />
              <el-option label="最新 500 行" :value="500" />
              <el-option label="最新 1000 行" :value="1000" />
              <el-option label="最新 3000 行" :value="3000" />
              <el-option label="最新 10000 行" :value="10000" />
            </el-select>
            <el-switch v-model="autoRefresh" size="small" active-text="自动刷新" inactive-text="暂停" />
            <el-button size="small" plain :icon="Refresh" :loading="loading" @click="fetchLogs">
              刷新
            </el-button>
            <el-button size="small" plain :icon="DocumentCopy" :disabled="filteredLogs.length === 0" @click="copyDisplayedLogs">
              复制
            </el-button>
          </div>
        </div>
      </template>

      <div class="log-console" ref="consoleRef">
        <div
          v-for="(log, index) in filteredLogs"
          :key="index"
          class="log-line"
          :class="`log-${String(log.level || '').toLowerCase()}`"
        >
          <span class="log-time">{{ log.timestamp }}</span>
          <span class="log-level" :class="`level-${log.level}`">
            {{ log.level }}
          </span>
          <span class="log-message">{{ log.message }}</span>
        </div>
        <div v-if="filteredLogs.length === 0" class="log-empty">
          暂无日志
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Document, DocumentCopy, Refresh, Search } from '@element-plus/icons-vue'
import api from '@/api'

const logs = ref([])
const searchKeyword = ref('')
const levelFilter = ref(['INFO', 'WARN', 'ERROR'])
const lineLimit = ref(1000)
const autoRefresh = ref(true)
const loading = ref(false)
const consoleRef = ref(null)
let refreshTimer = null

const filteredLogs = computed(() => {
  let result = logs.value

  // 级别过滤
  if (levelFilter.value.length > 0) {
    result = result.filter(log => levelFilter.value.includes(log.level))
  }

  // 关键词搜索
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    result = result.filter(log =>
      String(log.message || '').toLowerCase().includes(keyword) ||
      String(log.timestamp || '').toLowerCase().includes(keyword)
    )
  }

  return result
})

const fetchLogs = async () => {
  loading.value = true
  try {
    const response = await api.get('/logs', { params: { limit: lineLimit.value } })
    logs.value = response.logs || response.data?.logs || []
    
    // 自动滚动到底部
    nextTick(() => {
      if (consoleRef.value) {
        consoleRef.value.scrollTop = consoleRef.value.scrollHeight
      }
    })
  } catch (error) {
    console.error('获取日志失败:', error)
  } finally {
    loading.value = false
  }
}

const stopAutoRefresh = () => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

const startAutoRefresh = () => {
  stopAutoRefresh()
  if (!autoRefresh.value) return
  // 每10秒刷新一次
  refreshTimer = setInterval(() => {
    fetchLogs()
  }, 10000)
}

const copyDisplayedLogs = async () => {
  const text = filteredLogs.value
    .map(log => `${log.timestamp || ''} ${log.level || ''} ${log.message || ''}`.trim())
    .join('\n')
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`已复制 ${filteredLogs.value.length} 行日志`)
  } catch (error) {
    ElMessage.error('复制失败，请手动选择日志')
  }
}

watch(autoRefresh, startAutoRefresh)

onMounted(() => {
  fetchLogs()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
})
</script>

<style scoped>
.log-container {
  padding: 20px;
  height: calc(100vh - 100px);
}

.log-card {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.log-card :deep(.el-card__body) {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.log-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
}

.log-title .el-icon {
  color: #409eff;
}

.log-count {
  color: var(--text-color-secondary);
  font-size: 12px;
  font-weight: 400;
}

.log-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.control-search {
  width: 220px;
}

.control-level {
  width: 160px;
}

.control-limit {
  width: 140px;
}

.log-console {
  flex: 1;
  overflow-y: auto;
  background: var(--log-console-bg, #1e1e1e);
  border-radius: 4px;
  padding: 12px;
  font-family: 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.6;
}

.log-line {
  margin-bottom: 4px;
  white-space: pre-wrap;
  word-break: break-all;
}

.log-time {
  color: #569cd6;
  margin-right: 8px;
}

.log-level {
  display: inline-block;
  padding: 2px 6px;
  border-radius: 3px;
  margin-right: 8px;
  font-weight: bold;
  font-size: 11px;
}

.level-INFO {
  background: rgba(76, 175, 80, 0.2);
  color: #4caf50;
}

.level-WARN {
  background: rgba(255, 152, 0, 0.2);
  color: #ff9800;
}

.level-ERROR {
  background: rgba(244, 67, 54, 0.2);
  color: #f44336;
}

.level-DEBUG {
  background: rgba(158, 158, 158, 0.2);
  color: #9e9e9e;
}

.log-message {
  color: #d4d4d4;
}

.log-empty {
  color: #909399;
  text-align: center;
  padding: 40px;
}

/* 滚动条美化 */
.log-console::-webkit-scrollbar {
  width: 8px;
}

.log-console::-webkit-scrollbar-track {
  background: var(--log-console-scrollbar-track);
}

.log-console::-webkit-scrollbar-thumb {
  background: var(--log-console-scrollbar-thumb);
  border-radius: 4px;
}

.log-console::-webkit-scrollbar-thumb:hover {
  background: #4e4e52;
}
</style>
