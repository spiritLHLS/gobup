<template>
  <el-dialog 
    :model-value="visible"
    :title="`操作 - ${history?.title || ''}`" 
    width="600px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <div class="actions-container">
      <!-- 状态信息 -->
      <div class="status-section">
        <h4 class="section-title">状态信息</h4>
        <div class="status-grid">
          <div class="status-item">
            <span class="status-label">上传状态：</span>
            <el-tag v-if="history?.bvId" type="success">已发布</el-tag>
            <el-tag v-else-if="history?.uploadPartCount > 0 && history?.uploadPartCount === history?.partCount" type="success">已上传{{ history.uploadPartCount }}P</el-tag>
            <el-tag v-else-if="history?.uploadPartCount > 0" type="warning">已上传{{ history.uploadPartCount }}P</el-tag>
            <el-tag v-else-if="history?.uploadStatus === 1" type="info">上传中</el-tag>
            <el-tag v-else type="info">未上传</el-tag>
          </div>
          <div class="status-item">
            <span class="status-label">视频状态：</span>
            <el-tag v-if="history?.videoState === 1" type="success">已通过</el-tag>
            <el-tag v-else-if="history?.videoState === 0" type="warning">审核中</el-tag>
            <el-tag v-else-if="history?.videoState === -2" type="danger">未通过</el-tag>
            <el-tag v-else type="info">未知</el-tag>
          </div>
          <div class="status-item">
            <span class="status-label">弹幕状态：</span>
            <el-tag v-if="history?.danmakuSent" type="success">已发送({{ history.danmakuCount }})</el-tag>
            <el-tag v-else type="info">未发送</el-tag>
          </div>
          <div class="status-item">
            <span class="status-label">文件状态：</span>
            <el-tag v-if="history?.filesMoved" type="success">已移动</el-tag>
            <el-tag v-else type="info">未移动</el-tag>
          </div>
        </div>
        <div v-if="history?.bvId" class="bv-link">
          <a :href="`https://www.bilibili.com/video/${history.bvId}`" target="_blank">
            {{ history.bvId }}
          </a>
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="actions-section">
        <h4 class="section-title">可用操作</h4>
        <div class="actions-grid">
          <el-button 
            type="warning"
            :disabled="!hasUnuploadedParts || history?.publish"
            @click="$emit('upload')"
          >
            <el-icon><Upload /></el-icon>
            上传视频
          </el-button>

          <el-button 
            type="primary"
            :disabled="!history?.uploadPartCount || !!history?.bvId"
            @click="$emit('publish')"
          >
            <el-icon><Promotion /></el-icon>
            投稿视频
          </el-button>

          <el-button 
            type="primary"
            plain
            @click="$emit('manualPublish')"
          >
            <el-icon><Edit /></el-icon>
            手动标记投稿
          </el-button>

          <el-button 
            type="primary"
            plain
            @click="$emit('parseDanmaku')"
          >
            <el-icon><Document /></el-icon>
            解析弹幕
          </el-button>

          <el-button 
            type="success"
            :disabled="!history?.bvId || history?.danmakuSent"
            @click="$emit('sendDanmaku')"
          >
            <el-icon><ChatDotRound /></el-icon>
            发送弹幕
          </el-button>

          <el-button 
            type="info"
            :disabled="!history?.bvId"
            @click="$emit('syncVideo')"
          >
            <el-icon><Refresh /></el-icon>
            同步信息
          </el-button>

          <el-button 
            type="warning"
            :disabled="!history?.publish || history?.filesMoved"
            @click="$emit('moveFiles')"
          >
            <el-icon><FolderOpened /></el-icon>
            移动文件
          </el-button>

          <el-button 
            plain
            @click="$emit('resetStatus')"
          >
            <el-icon><RefreshLeft /></el-icon>
            重置状态
          </el-button>

          <el-button 
            type="danger"
            plain
            @click="$emit('deleteOnly')"
          >
            <el-icon><Delete /></el-icon>
            仅删除记录
          </el-button>

          <el-button 
            type="danger"
            @click="$emit('deleteWithFiles')"
          >
            <el-icon><DeleteFilled /></el-icon>
            删除记录和文件
          </el-button>
        </div>
      </div>
    </div>
    
    <template #footer>
      <el-button @click="$emit('update:visible', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed } from 'vue'
import { 
  Upload, 
  ChatDotRound, 
  Document,
  Refresh, 
  FolderOpened, 
  RefreshLeft, 
  Delete, 
  DeleteFilled,
  Promotion,
  Edit
} from '@element-plus/icons-vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  history: {
    type: Object,
    default: null
  }
})

defineEmits([
  'update:visible',
  'upload',
  'publish',
  'manualPublish',
  'parseDanmaku',
  'sendDanmaku',
  'syncVideo',
  'moveFiles',
  'resetStatus',
  'deleteOnly',
  'deleteWithFiles'
])

const hasUnuploadedParts = computed(() => {
  if (!props.history) return false
  const partCount = props.history.partCount || 0
  const uploadPartCount = props.history.uploadPartCount || 0
  return partCount > uploadPartCount
})
</script>

<style scoped>
.actions-container {
  padding: 10px 0;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 15px;
  padding-bottom: 8px;
  border-bottom: 2px solid #e4e7ed;
}

.status-section {
  margin-bottom: 25px;
}

.status-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  margin-bottom: 12px;
}

.status-item {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  background: #f5f7fa;
  border-radius: 4px;
}

.status-label {
  font-size: 13px;
  color: #606266;
  margin-right: 8px;
}

.bv-link {
  margin-top: 12px;
  padding: 10px;
  background: #ecf5ff;
  border-radius: 4px;
  text-align: center;
}

.bv-link a {
  color: #409eff;
  text-decoration: none;
  font-weight: 500;
}

.bv-link a:hover {
  text-decoration: underline;
}

.actions-section {
  margin-bottom: 10px;
}

.actions-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.actions-grid .el-button {
  width: 100%;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 15px;
  box-sizing: border-box;
}

.actions-grid .el-button .el-icon {
  margin-right: 5px;
}

@media (max-width: 768px) {
  .status-grid,
  .actions-grid {
    grid-template-columns: 1fr;
  }
}
</style>
