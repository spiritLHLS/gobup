<template>
  <div class="log-container">
    <el-card class="log-card">
      <template #header>
        <div class="log-header">
          <span class="log-title">
            <i class="el-icon-document"></i>
            实时日志
          </span>
          <div class="log-controls">
            <el-input
              v-model="searchKeyword"
              placeholder="搜索日志..."
              size="small"
              clearable
              style="width: 200px; margin-right: 10px"
            >
              <i slot="prefix" class="el-icon-search"></i>
            </el-input>
            <el-select
              v-model="levelFilter"
              multiple
              collapse-tags
              placeholder="日志级别"
              size="small"
              style="width: 150px; margin-right: 10px"
            >
              <el-option label="INFO" value="INFO" />
              <el-option label="WARN" value="WARN" />
              <el-option label="ERROR" value="ERROR" />
              <el-option label="DEBUG" value="DEBUG" />
            </el-select>
            <el-button
              :type="realtime ? 'success' : 'info'"
              size="small"
              @click="toggleRealtime"
            >
              {{ realtime ? '实时推送' : '已暂停' }}
            </el-button>
            <el-button size="small" @click="clearLogs">
              清空
            </el-button>
          </div>
        </div>
      </template>

      <div class="log-console" ref="console">
        <div
          v-for="(log, index) in filteredLogs"
          :key="index"
          class="log-line"
          :class="`log-${log.level.toLowerCase()}`"
        >
          <span class="log-time">{{ log.timestamp }}</span>
          <span class="log-level" :class="`level-${log.level}`">
            {{ log.level }}
          </span>
          <span class="log-message">{{ log.message }}</span>
        </div>
        <div v-if="filteredLogs.length === 0" class="log-empty">
          {{ realtime ? '等待日志...' : '暂无日志' }}
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'

const logs = ref([])
const searchKeyword = ref('')
const levelFilter = ref(['INFO', 'WARN', 'ERROR'])
const realtime = ref(true)
const ws = ref(null)
const console = ref(null)

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
      log.message.toLowerCase().includes(keyword) ||
      log.timestamp.toLowerCase().includes(keyword)
    )
  }

  return result
})

const connectWebSocket = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  ws.value = new WebSocket(`${protocol}//${host}/ws/log`)

  ws.value.onopen = () => {
    console.log('WebSocket连接已建立')
  }

  ws.value.onmessage = (event) => {
    try {
      const log = JSON.parse(event.data)
      addLog(log)
    } catch (e) {
      console.error('解析日志失败:', e)
    }
  }

  ws.value.onerror = (error) => {
    console.error('WebSocket错误:', error)
  }

  ws.value.onclose = () => {
    console.log('WebSocket连接已关闭')
    if (realtime.value) {
      // 3秒后重连
      setTimeout(() => {
        if (realtime.value) {
          connectWebSocket()
        }
      }, 3000)
    }
  }
}

const addLog = (log) => {
  logs.value.push(log)
  
  // 限制日志数量，避免内存溢出
  if (logs.value.length > 1000) {
    logs.value.shift()
  }

  // 自动滚动到底部
  nextTick(() => {
    if (console.value) {
      console.value.scrollTop = console.value.scrollHeight
    }
  })
}

const toggleRealtime = () => {
  realtime.value = !realtime.value
  if (realtime.value) {
    connectWebSocket()
  } else if (ws.value) {
    ws.value.close()
  }
}

const clearLogs = () => {
  logs.value = []
}

onMounted(() => {
  if (realtime.value) {
    connectWebSocket()
  }
})

onUnmounted(() => {
  if (ws.value) {
    ws.value.close()
  }
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
}

.log-title {
  font-size: 16px;
  font-weight: 600;
}

.log-title i {
  margin-right: 8px;
  color: #409eff;
}

.log-controls {
  display: flex;
  align-items: center;
}

.log-console {
  flex: 1;
  overflow-y: auto;
  background: #1e1e1e;
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
  background: #2d2d30;
}

.log-console::-webkit-scrollbar-thumb {
  background: #3e3e42;
  border-radius: 4px;
}

.log-console::-webkit-scrollbar-thumb:hover {
  background: #4e4e52;
}
</style>
