<template>
  <div class="dashboard-container">
    <!-- 页面标题 -->
    <div class="page-header">
      <h2>系统控制面板</h2>
      <p>管理系统功能开关和查看运行状态</p>
    </div>

    <!-- 统计卡片 -->
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

    <!-- 功能开关 -->
    <el-card class="config-card">
      <template #header>
        <div class="card-header">
          <span>功能开关</span>
          <el-button type="primary" size="default" @click="saveConfig" :loading="saving">
            <el-icon><Check /></el-icon>
            保存配置
          </el-button>
        </div>
      </template>

      <el-form label-width="180px" v-loading="loading" label-position="left">
        <div class="form-section">
          <div class="section-title">上传与投稿</div>
          
          <el-form-item label="自动上传">
            <div class="switch-item">
              <el-switch 
                v-model="config.AutoUpload" 
                @change="toggleFeature('AutoUpload', $event)"
                size="large"
              />
              <span class="help-text">启用后，系统会自动将录制文件上传到B站</span>
            </div>
          </el-form-item>

          <el-form-item label="自动投稿">
            <div class="switch-item">
              <el-switch 
                v-model="config.AutoPublish" 
                @change="toggleFeature('AutoPublish', $event)"
                size="large"
              />
              <span class="help-text">启用后，上传完成后自动提交投稿</span>
            </div>
          </el-form-item>

          <el-form-item label="自动删除">
            <div class="switch-item">
              <el-switch 
                v-model="config.AutoDelete" 
                @change="toggleFeature('AutoDelete', $event)"
                size="large"
              />
              <span class="help-text">启用后，投稿成功后自动删除本地文件</span>
            </div>
          </el-form-item>

          <el-form-item label="自动弹幕发送">
            <div class="switch-item">
              <el-switch 
                v-model="config.AutoSendDanmaku" 
                @change="toggleFeature('AutoSendDanmaku', $event)"
                size="large"
              />
              <span class="help-text">启用后，自动发送高能弹幕</span>
            </div>
          </el-form-item>
        </div>

        <el-divider />

        <div class="form-section">
          <div class="section-title">文件扫描</div>
          
          <el-form-item label="自动扫盘录入">
            <div class="switch-item">
              <el-switch 
                v-model="config.AutoFileScan" 
                @change="toggleFeature('AutoFileScan', $event)"
                size="large"
              />
              <span class="help-text">启用后，定时扫描录制目录，自动录入新文件</span>
            </div>
          </el-form-item>

          <el-form-item label="扫盘间隔（分钟）" v-if="config.AutoFileScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="config.FileScanInterval" 
                :min="10" 
                :max="1440"
                :step="10"
                size="large"
              />
              <span class="help-text">扫描间隔时间，最小10分钟</span>
            </div>
          </el-form-item>

          <el-form-item label="文件最小年龄（小时）" v-if="config.AutoFileScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="config.FileScanMinAge" 
                :min="1" 
                :max="72"
                size="large"
              />
              <span class="help-text">文件创建超过此时间才录入，避免扫描正在写入的文件（推荐12小时）</span>
            </div>
          </el-form-item>

          <el-form-item label="文件最小大小（MB）" v-if="config.AutoFileScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="fileScanMinSizeMB" 
                :min="1" 
                :max="10240"
                size="large"
                @change="updateFileScanMinSize"
              />
              <span class="help-text">小于此大小的文件将被忽略</span>
            </div>
          </el-form-item>

          <el-form-item label="文件最大年龄（小时）" v-if="config.AutoFileScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="fileScanMaxAgeHours" 
                :min="24" 
                :max="8760"
                :step="24"
                size="large"
                @change="updateFileScanMaxAge"
              />
              <span class="help-text">超过此时间的文件将被忽略（默认30天）</span>
            </div>
          </el-form-item>

          <el-form-item label="手动扫盘">
            <div class="button-group">
              <el-button 
                type="primary" 
                @click="triggerFileScan(false)" 
                :loading="scanning"
                :icon="Refresh"
              >
                扫描录入
              </el-button>
              <el-button 
                type="warning" 
                @click="openFileScanDialog" 
                :loading="scanning"
                :icon="FolderOpened"
              >
                强制扫盘（选择）
              </el-button>
              <span class="help-text">立即扫描录制目录。强制扫盘可以手动选择要入库的文件</span>
            </div>
          </el-form-item>
        </div>

        <el-divider />

        <div class="form-section">
          <div class="section-title">维护与清理</div>
          
          <el-form-item label="孤儿文件扫描">
            <div class="switch-item">
              <el-switch 
                v-model="config.EnableOrphanScan" 
                @change="toggleFeature('EnableOrphanScan', $event)"
                size="large"
              />
              <span class="help-text">启用后，定时清理无关联的历史记录</span>
            </div>
          </el-form-item>

          <el-form-item label="孤儿扫描间隔（分钟）" v-if="config.EnableOrphanScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="config.OrphanScanInterval" 
                :min="60" 
                :max="1440"
                :step="60"
                size="large"
              />
              <span class="help-text">孤儿文件扫描间隔时间，最小1小时</span>
            </div>
          </el-form-item>

          <el-form-item label="工作目录">
            <div class="path-input-wrapper">
              <el-input 
                v-model="config.WorkPath" 
                placeholder="/path/to/recordings"
                size="large"
              />
              <span class="help-text">录制文件存放的根目录</span>
            </div>
          </el-form-item>
        </div>
      </el-form>
    </el-card>

    <!-- 文件扫描对话框 -->
    <FileScanDialog ref="fileScanDialogRef" @imported="handleFilesImported" />
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { VideoCamera, Upload, Clock, Warning, Check, Refresh, FolderOpened } from '@element-plus/icons-vue'
import api, { filescanAPI } from '../api'
import FileScanDialog from '../components/filescan/FileScanDialog.vue'

const loading = ref(false)
const saving = ref(false)
const scanning = ref(false)
const fileScanDialogRef = ref(null)
const config = ref({
  AutoUpload: false,
  AutoPublish: false,
  AutoDelete: false,
  AutoSendDanmaku: false,
  AutoFileScan: true,
  FileScanInterval: 60,
  FileScanMinAge: 12,
  FileScanMinSize: 1048576,
  FileScanMaxAge: 720,
  WorkPath: '',
  EnableOrphanScan: true,
  OrphanScanInterval: 360
})

const stats = ref({
  totalRecordings: 0,
  uploadedCount: 0,
  pendingCount: 0,
  failedCount: 0
})

// 计算属性：将字节转MB显示
const fileScanMinSizeMB = computed({
  get: () => Math.round(config.value.FileScanMinSize / (1024 * 1024)),
  set: (val) => {} // 空setter，实际更新在updateFileScanMinSize中
})

// 计算属性：将小时转换显示
const fileScanMaxAgeHours = computed({
  get: () => config.value.FileScanMaxAge,
  set: (val) => {} // 空setter，实际更新在updateFileScanMaxAge中
})

// 更新文件最小大小（MB转字节）
const updateFileScanMinSize = (val) => {
  config.value.FileScanMinSize = val * 1024 * 1024
}

// 更新文件最大年龄（小时）
const updateFileScanMaxAge = (val) => {
  config.value.FileScanMaxAge = val
}

// 加载配置
const loadConfig = async () => {
  loading.value = true
  try {
    const response = await api.get('/config/system')
    // 后端直接返回 config 对象
    config.value = response
  } catch (error) {
    console.error('加载配置失败:', error)
    ElMessage.error('加载配置失败: ' + (error.message || '网络错误'))
  } finally {
    loading.value = false
  }
}

// 加载统计数据
const loadStats = async () => {
  try {
    const response = await api.get('/config/stats')
    // 后端直接返回 stats 对象
    stats.value = response
  } catch (error) {
    console.error('加载统计数据失败:', error)
  }
}

// 切换功能开关（实时生效）
const toggleFeature = async (feature, enabled) => {
  try {
    // 将大驼峰转换为小驼峰，以匹配后端的 JSON 标签
    const key = feature.charAt(0).toLowerCase() + feature.slice(1)
    
    const response = await api.post('/config/toggle', {
      key: key,
      value: enabled
    })
    if (response.type === 'success') {
      ElMessage.success(`${getFeatureName(feature)}已${enabled ? '开启' : '关闭'}`)
    } else {
      ElMessage.error(response.msg || '切换失败')
      // 还原状态
      config.value[feature] = !enabled
    }
  } catch (error) {
    console.error('切换功能失败:', error)
    ElMessage.error('切换失败: ' + (error.message || '网络错误'))
    // 还原状态
    config.value[feature] = !enabled
  }
}

// 保存完整配置
const saveConfig = async () => {
  saving.value = true
  try {
    const response = await api.put('/config/system', config.value)
    if (response.type === 'success') {
      ElMessage.success('配置保存成功')
      loadConfig()
    } else {
      ElMessage.error(response.msg || '保存失败')
    }
  } catch (error) {
    console.error('保存配置失败:', error)
    ElMessage.error('保存失败: ' + (error.message || '网络错误'))
  } finally {
    saving.value = false
  }
}

// 获取功能名称
const getFeatureName = (feature) => {
  const names = {
    AutoUpload: '自动上传',
    AutoPublish: '自动投稿',
    AutoDelete: '自动删除',
    AutoSendDanmaku: '自动弹幕发送',
    AutoFileScan: '自动扫盘录入',
    EnableOrphanScan: '孤儿文件扫描'
  }
  return names[feature] || feature
}

// 触发文件扫描
const triggerFileScan = async (force = false) => {
  const action = force ? '强制扫盘' : '扫描录入'
  const confirmMessage = force 
    ? '强制扫盘将无视文件年龄限制，可能导入正在写入的文件。是否继续？' 
    : '确定要立即扫描录制目录吗？'
  
  try {
    await ElMessageBox.confirm(confirmMessage, '确认' + action, {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: force ? 'warning' : 'info'
    })
    
    scanning.value = true
    const response = await filescanAPI.trigger(force)
    
    if (response.type === 'success') {
      const message = `扫描完成！总文件: ${response.totalFiles}, 新导入: ${response.newFiles}, 跳过: ${response.skippedFiles}, 失败: ${response.failedFiles}`
      
      if (response.failedFiles > 0 && response.errors && response.errors.length > 0) {
        ElMessageBox.alert(
          message + '\n\n失败文件：\n' + response.errors.join('\n'),
          '扫描结果',
          { type: 'warning' }
        )
      } else {
        ElMessage.success(message)
      }
      
      // 刷新统计数据
      loadStats()
    } else {
      ElMessage.error(response.msg || action + '失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error(action + '失败:', error)
      ElMessage.error(action + '失败: ' + (error.message || '网络错误'))
    }
  } finally {
    scanning.value = false
  }
}

// 打开文件扫描对话框
const openFileScanDialog = () => {
  if (fileScanDialogRef.value) {
    fileScanDialogRef.value.open()
  }
}

// 文件导入完成后的处理
const handleFilesImported = () => {
  // 刷新统计数据
  loadStats()
}

onMounted(() => {
  loadConfig()
  loadStats()
  
  // 每30秒刷新统计数据
  setInterval(loadStats, 30000)
})
</script>

<style scoped lang="scss">
.dashboard-container {
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--spacing-lg);
  
  h2 {
    font-size: var(--font-size-3xl);
    color: var(--text-color-primary);
    font-weight: var(--font-weight-bold);
    margin: 0 0 8px 0;
  }
  
  p {
    font-size: var(--font-size-base);
    color: var(--text-color-secondary);
    margin: 0;
  }
}

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

.config-card {
  margin-bottom: var(--spacing-xl);
  
  :deep(.el-card__header) {
    background-color: var(--bg-color-tertiary);
  }
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  
  > span {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--text-color-primary);
  }
}

.form-section {
  margin-bottom: var(--spacing-xl);
  
  .section-title {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--text-color-primary);
    margin-bottom: var(--spacing-lg);
    padding-bottom: var(--spacing-sm);
    border-bottom: 2px solid var(--primary-color);
    display: inline-block;
  }
}

.switch-item,
.number-input-wrapper,
.path-input-wrapper,
.button-group {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.button-group {
  .el-button {
    min-width: 120px;
  }
}

.help-text {
  flex: 1;
  min-width: 200px;
  color: var(--text-color-secondary);
  font-size: var(--font-size-sm);
  line-height: 1.6;
}

:deep(.el-form-item) {
  margin-bottom: var(--spacing-lg);
  
  .el-form-item__label {
    color: var(--text-color-primary);
    font-weight: var(--font-weight-medium);
  }
}

:deep(.el-divider) {
  margin: var(--spacing-2xl) 0;
  border-color: var(--border-color-light);
}

:deep(.el-input-number) {
  width: 180px;
}

:deep(.el-input) {
  max-width: 500px;
}

/* 响应式 */
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
  .page-header h2 {
    font-size: var(--font-size-2xl);
  }
  
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
  
  :deep(.el-form) {
    .el-form-item__label {
      font-size: var(--font-size-sm);
    }
  }
  
  .switch-item,
  .number-input-wrapper {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-sm);
  }
  
  .help-text {
    min-width: auto;
  }
}

@media (max-width: 480px) {
  .page-header {
    margin-bottom: var(--spacing-md);
    
    h2 {
      font-size: var(--font-size-xl);
    }
    
    p {
      font-size: var(--font-size-sm);
    }
  }
  
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
  
  .card-header {
    flex-direction: column;
    gap: var(--spacing-sm);
    align-items: flex-start;
    
    :deep(.el-button) {
      width: 100%;
    }
  }
  
  :deep(.el-input-number) {
    width: 100%;
  }
}
</style>
