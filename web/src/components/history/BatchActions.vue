<template>
  <div v-if="selectedHistories.length > 0" class="batch-actions">
    <div class="batch-header">
      <span class="batch-info">已选择 {{ selectedHistories.length }} 项</span>
    </div>
    <div class="batch-buttons">
      <el-button size="small" type="warning" @click="$emit('upload')">
        <el-icon><Upload /></el-icon>
        上传视频
      </el-button>
      <el-button size="small" type="primary" @click="$emit('publish')">
        <el-icon><Promotion /></el-icon>
        投稿视频
      </el-button>
      <el-button size="small" type="success" @click="$emit('sendDanmaku')">
        <el-icon><ChatDotRound /></el-icon>
        发送弹幕
      </el-button>
      <el-button size="small" type="primary" plain @click="$emit('parseDanmaku')">
        <el-icon><Document /></el-icon>
        解析弹幕
      </el-button>
      <el-button size="small" type="info" @click="$emit('syncVideo')">
        <el-icon><Refresh /></el-icon>
        同步信息
      </el-button>
      <el-button size="small" type="warning" @click="$emit('moveFiles')">
        <el-icon><FolderOpened /></el-icon>
        移动文件
      </el-button>
      <el-button size="small" plain @click="$emit('resetStatus')">
        <el-icon><RefreshLeft /></el-icon>
        重置状态
      </el-button>
      <el-button size="small" type="danger" plain @click="$emit('deleteOnly')">
        <el-icon><Delete /></el-icon>
        仅删除记录
      </el-button>
      <el-button size="small" type="danger" @click="$emit('deleteWithFiles')">
        <el-icon><DeleteFilled /></el-icon>
        删除记录和文件
      </el-button>
    </div>
  </div>
</template>

<script setup>
import { 
  Upload, 
  ChatDotRound, 
  Document,
  Refresh, 
  FolderOpened, 
  RefreshLeft, 
  Delete, 
  DeleteFilled,
  Promotion
} from '@element-plus/icons-vue'

defineProps({
  selectedHistories: {
    type: Array,
    required: true
  }
})

defineEmits([
  'upload',
  'publish',
  'sendDanmaku',
  'parseDanmaku',
  'syncVideo',
  'moveFiles',
  'resetStatus',
  'deleteOnly',
  'deleteWithFiles'
])
</script>

<style scoped>
.batch-actions {
  padding: 15px;
  background: #f5f7fa;
  border-radius: 8px;
  margin-top: 15px;
}

.batch-header {
  margin-bottom: 12px;
}

.batch-info {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.batch-buttons {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}

.batch-buttons .el-button {
  margin: 0;
}

@media (max-width: 1200px) {
  .batch-buttons {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 768px) {
  .batch-buttons {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
