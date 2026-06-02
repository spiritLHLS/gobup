<template>
  <el-row :gutter="20" class="stats-row">
    <el-col :xs="24" :sm="12" :md="6">
      <el-card shadow="hover" class="stat-card">
        <div class="stat-content">
          <div class="stat-icon primary-icon">
            <el-icon><VideoCamera /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-number">{{ stats.totalRecordings || 0 }}</div>
            <div class="stat-label">总录制数</div>
          </div>
        </div>
      </el-card>
    </el-col>

    <el-col :xs="24" :sm="12" :md="6">
      <el-card shadow="hover" class="stat-card success-card">
        <div class="stat-content">
          <div class="stat-icon success-icon">
            <el-icon><Upload /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-number">{{ stats.uploadedCount || 0 }}</div>
            <div class="stat-label">已上传</div>
          </div>
        </div>
      </el-card>
    </el-col>

    <el-col :xs="24" :sm="12" :md="6">
      <el-card shadow="hover" class="stat-card warning-card">
        <div class="stat-content">
          <div class="stat-icon warning-icon">
            <el-icon><Clock /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-number">{{ stats.pendingCount || 0 }}</div>
            <div class="stat-label">待处理</div>
          </div>
        </div>
      </el-card>
    </el-col>

    <el-col :xs="24" :sm="12" :md="6">
      <el-card shadow="hover" class="stat-card danger-card">
        <div class="stat-content">
          <div class="stat-icon danger-icon">
            <el-icon><Warning /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-number">{{ stats.failedCount || 0 }}</div>
            <div class="stat-label">失败</div>
          </div>
        </div>
      </el-card>
    </el-col>
  </el-row>
</template>

<script setup>
import { VideoCamera, Upload, Clock, Warning } from '@element-plus/icons-vue'

defineProps({
  stats: {
    type: Object,
    default: () => ({
      totalRecordings: 0,
      uploadedCount: 0,
      pendingCount: 0,
      failedCount: 0
    })
  }
})
</script>

<style scoped lang="scss">
.stats-row {
  margin-bottom: var(--spacing-xl);
}

.stat-card {
  height: 120px;
  border-radius: var(--border-radius-xl);
  transition: var(--transition-normal);
  cursor: pointer;
  border: 2px solid transparent;

  &:hover {
    transform: translateY(-4px);
    box-shadow: var(--box-shadow-hover);
  }

  &.success-card {
    border-color: rgba(103, 194, 58, 0.2);
  }

  &.warning-card {
    border-color: rgba(230, 162, 60, 0.2);
  }

  &.danger-card {
    border-color: rgba(245, 108, 108, 0.2);
  }

  :deep(.el-card__body) {
    height: 100%;
    display: flex;
    align-items: center;
    padding: 20px;
  }
}

.stat-content {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  width: 100%;
}

.stat-icon {
  width: 60px;
  height: 60px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  flex-shrink: 0;

  &.primary-icon {
    background: linear-gradient(135deg, var(--primary-color), var(--primary-color-light));
    color: white;
  }

  &.success-icon {
    background: linear-gradient(135deg, #67c23a, #85ce61);
    color: white;
  }

  &.warning-icon {
    background: linear-gradient(135deg, #e6a23c, #f0c78a);
    color: white;
  }

  &.danger-icon {
    background: linear-gradient(135deg, #f56c6c, #f89898);
    color: white;
  }
}

.stat-info {
  flex: 1;
  min-width: 0;
}

.stat-number {
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-bold);
  color: var(--text-color-primary);
  line-height: 1.2;
  margin-bottom: 4px;
}

.stat-label {
  font-size: var(--font-size-sm);
  color: var(--text-color-secondary);
  font-weight: var(--font-weight-medium);
}

@media (max-width: 1024px) {
  .stat-card {
    height: 110px;
    margin-bottom: var(--spacing-md);
  }

  .stat-icon {
    width: 55px;
    height: 55px;
    font-size: 26px;
  }

  .stat-number {
    font-size: var(--font-size-2xl);
  }
}

@media (max-width: 768px) {
  .stats-row {
    margin-bottom: var(--spacing-lg);
  }

  .stat-card {
    height: 100px;

    :deep(.el-card__body) {
      padding: 16px;
    }
  }

  .stat-content {
    gap: var(--spacing-sm);
  }

  .stat-icon {
    width: 50px;
    height: 50px;
    font-size: 24px;
  }

  .stat-number {
    font-size: var(--font-size-2xl);
  }

  .stat-label {
    font-size: var(--font-size-xs);
  }
}

@media (max-width: 480px) {
  .stat-card {
    height: 90px;

    :deep(.el-card__body) {
      padding: 12px;
    }
  }

  .stat-icon {
    width: 45px;
    height: 45px;
    font-size: 20px;
  }

  .stat-number {
    font-size: var(--font-size-xl);
  }

  .stat-label {
    font-size: 12px;
  }
}
</style>
