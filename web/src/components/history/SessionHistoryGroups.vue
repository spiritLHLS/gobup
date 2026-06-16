<template>
  <div class="session-groups">
    <section v-for="group in sessionGroups" :key="group.key" class="session-group">
      <div class="session-group-header">
        <div class="session-group-title">
          <span>{{ group.title }}</span>
          <el-tag v-if="group.hasHighlight" size="small" type="warning">高能剪辑</el-tag>
        </div>
        <button class="session-id-button" type="button" @click="$emit('filterSession', group.sessionId)">
          <el-icon><Link /></el-icon>
          {{ group.sessionId || '无SessionID' }}
        </button>
      </div>

      <div class="card-grid session-card-grid">
        <div
          v-for="row in group.items"
          :key="row.id"
          class="history-card"
          :class="getCardClass(row)"
          @click="$emit('showActions', row)"
        >
          <div class="history-card-status">
            <el-tag v-if="row.isHighlight" type="warning" size="small" effect="plain">高光</el-tag>
            <el-tag v-if="row.bvId" type="success" size="small" effect="dark">已投稿</el-tag>
            <el-tag v-else-if="row.recording" type="danger" size="small" effect="dark">录制中</el-tag>
            <el-tag v-else-if="isUploading(row)" type="warning" size="small" effect="dark">上传中</el-tag>
            <el-tag v-else-if="row.uploadPartCount > 0" type="info" size="small">已上传{{ row.uploadPartCount }}P</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">未上传</el-tag>
          </div>

          <div class="history-card-title" :title="row.title">{{ row.title || '无标题' }}</div>
          <div class="history-card-meta">
            <span class="meta-item">
              <el-icon><User /></el-icon>
              {{ privacyMode ? '***' : (row.uname || '-') }}
            </span>
            <span class="meta-item">
              <el-icon><HomeFilled /></el-icon>
              {{ privacyMode ? '***' : row.roomId }}
            </span>
            <span class="meta-item part-count-meta" @click.stop="$emit('showParts', row)" title="查看分P详情">
              <el-icon><Film /></el-icon>
              {{ row.partCount || 0 }}P
            </span>
          </div>

          <div v-if="isUploading(row) && getHistoryProgress(row.id)" class="history-card-progress">
            <el-progress
              :percentage="getHistoryUploadPercent(row.id)"
              :stroke-width="6"
              :status="getHistoryUploadPercent(row.id) >= 100 ? 'success' : null"
            />
            <span class="progress-hint">{{ getHistoryProgress(row.id)?.activeCount || 0 }}P上传中</span>
          </div>

          <div v-if="row.bvId" class="history-card-bv">
            <a :href="`https://www.bilibili.com/video/${row.bvId}`" target="_blank" @click.stop>
              {{ row.bvId }}
            </a>
          </div>

          <div class="history-card-time">{{ formatTime(row.startTime) }}</div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Film, HomeFilled, Link, User } from '@element-plus/icons-vue'

const props = defineProps({
  histories: {
    type: Array,
    default: () => []
  },
  privacyMode: {
    type: Boolean,
    default: false
  },
  isUploading: {
    type: Function,
    required: true
  },
  getCardClass: {
    type: Function,
    required: true
  },
  getHistoryProgress: {
    type: Function,
    required: true
  },
  getHistoryUploadPercent: {
    type: Function,
    required: true
  },
  formatTime: {
    type: Function,
    required: true
  }
})

defineEmits(['showActions', 'showParts', 'filterSession'])

const sessionGroups = computed(() => {
  const groups = new Map()
  props.histories.forEach((history) => {
    const sessionId = history.sessionId || ''
    const key = sessionId || `history-${history.id}`
    if (!groups.has(key)) {
      groups.set(key, {
        key,
        sessionId,
        title: history.title || history.uname || '未命名场次',
        latestTime: history.endTime || history.startTime,
        hasHighlight: false,
        items: []
      })
    }
    const group = groups.get(key)
    group.items.push(history)
    group.hasHighlight = group.hasHighlight || !!history.isHighlight
    const historyTime = history.endTime || history.startTime
    if (historyTime && (!group.latestTime || new Date(historyTime) > new Date(group.latestTime))) {
      group.latestTime = historyTime
      if (history.title) group.title = history.title
    }
  })
  return Array.from(groups.values()).sort((a, b) => new Date(b.latestTime || 0) - new Date(a.latestTime || 0))
})
</script>

<style scoped lang="scss">
.session-groups {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
  margin-bottom: var(--spacing-lg);
}

.session-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.session-group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  min-height: 36px;
  padding: 0 2px;
}

.session-group-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 0;
  color: var(--text-color-primary);
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-semibold);

  span:first-child {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.session-id-button {
  border: none;
  background: transparent;
  color: var(--primary-color);
  cursor: pointer;
  font: inherit;
  padding: 0;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--font-size-sm);

  &:hover {
    text-decoration: underline;
  }
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--spacing-md);
}

.history-card {
  background: var(--bg-color-secondary);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--border-color);
  border-radius: var(--border-radius-large);
  padding: var(--spacing-lg);
  cursor: pointer;
  transition: all var(--transition-normal);
  position: relative;
  animation: fadeIn 0.2s ease;

  &:hover {
    box-shadow: var(--shadow-medium);
    transform: translateY(-2px);
  }

  &.status-success { border-left-color: var(--success-color); }
  &.status-uploading { border-left-color: var(--warning-color); }
  &.status-recording { border-left-color: var(--danger-color); }
  &.status-partial { border-left-color: var(--info-color); }
}

.history-card-status {
  position: absolute;
  top: var(--spacing-md);
  right: var(--spacing-md);
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: flex-end;
  max-width: 132px;
}

.history-card-title {
  font-weight: var(--font-weight-semibold);
  color: var(--text-color-primary);
  font-size: var(--font-size-base);
  margin-bottom: var(--spacing-sm);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding-right: 140px;
}

.history-card-meta {
  display: flex;
  gap: var(--spacing-md);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-sm);
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--font-size-sm);
  color: var(--text-color-secondary);
}

.part-count-meta {
  cursor: pointer;
  color: var(--primary-color);
  border-radius: var(--border-radius-small);
  padding: 0 4px;
  transition: background var(--transition-fast);

  &:hover {
    background: rgba(64, 158, 255, 0.1);
    color: var(--primary-color-dark, var(--primary-color));
  }
}

.history-card-progress {
  margin-bottom: var(--spacing-sm);

  .progress-hint {
    font-size: 11px;
    color: var(--text-color-secondary);
    margin-top: 2px;
    display: block;
  }
}

.history-card-bv {
  margin-bottom: 4px;

  a {
    font-size: var(--font-size-sm);
    color: var(--primary-color);
    text-decoration: none;

    &:hover { text-decoration: underline; }
  }
}

.history-card-time {
  font-size: var(--font-size-sm);
  color: var(--text-color-secondary);
}
</style>
