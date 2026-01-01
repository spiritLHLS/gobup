<template>
  <div class="dashboard">
    <el-card class="header-card">
      <h2>系统控制面板</h2>
      <p class="subtitle">管理系统功能开关和查看运行状态</p>
    </el-card>

    <!-- 统计卡片 -->
    <el-row :gutter="20" class="stats-row">
      <el-col :xs="24" :sm="12" :md="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-item">
            <el-icon class="stat-icon" color="#409EFF"><VideoCamera /></el-icon>
            <div class="stat-content">
              <div class="stat-value">{{ stats.totalRecordings || 0 }}</div>
              <div class="stat-label">总录制数</div>
            </div>
          </div>
        </el-card>
      </el-col>
      
      <el-col :xs="24" :sm="12" :md="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-item">
            <el-icon class="stat-icon" color="#67C23A"><Upload /></el-icon>
            <div class="stat-content">
              <div class="stat-value">{{ stats.uploadedCount || 0 }}</div>
              <div class="stat-label">已上传</div>
            </div>
          </div>
        </el-card>
      </el-col>
      
      <el-col :xs="24" :sm="12" :md="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-item">
            <el-icon class="stat-icon" color="#E6A23C"><Clock /></el-icon>
            <div class="stat-content">
              <div class="stat-value">{{ stats.pendingCount || 0 }}</div>
              <div class="stat-label">待处理</div>
            </div>
          </div>
        </el-card>
      </el-col>
      
      <el-col :xs="24" :sm="12" :md="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-item">
            <el-icon class="stat-icon" color="#F56C6C"><Warning /></el-icon>
            <div class="stat-content">
              <div class="stat-value">{{ stats.failedCount || 0 }}</div>
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
          <el-button type="primary" size="small" @click="saveConfig" :loading="saving">
            保存配置
          </el-button>
        </div>
      </template>

      <el-form label-width="200px" v-loading="loading">
        <el-form-item label="自动上传">
          <el-switch 
            v-model="config.AutoUpload" 
            @change="toggleFeature('AutoUpload', $event)"
            active-text="开启"
            inactive-text="关闭"
          />
          <span class="help-text">启用后，系统会自动将录制文件上传到B站</span>
        </el-form-item>

        <el-form-item label="自动投稿">
          <el-switch 
            v-model="config.AutoPublish" 
            @change="toggleFeature('AutoPublish', $event)"
            active-text="开启"
            inactive-text="关闭"
          />
          <span class="help-text">启用后，上传完成后自动提交投稿</span>
        </el-form-item>

        <el-form-item label="自动删除">
          <el-switch 
            v-model="config.AutoDelete" 
            @change="toggleFeature('AutoDelete', $event)"
            active-text="开启"
            inactive-text="关闭"
          />
          <span class="help-text">启用后，投稿成功后自动删除本地文件</span>
        </el-form-item>

        <el-form-item label="自动弹幕发送">
          <el-switch 
            v-model="config.AutoSendDanmaku" 
            @change="toggleFeature('AutoSendDanmaku', $event)"
            active-text="开启"
            inactive-text="关闭"
          />
          <span class="help-text">启用后，自动发送高能弹幕</span>
        </el-form-item>

        <el-divider />

        <el-form-item label="自动扫盘录入">
          <el-switch 
            v-model="config.AutoFileScan" 
            @change="toggleFeature('AutoFileScan', $event)"
            active-text="开启"
            inactive-text="关闭"
          />
          <span class="help-text">启用后，定时扫描录制目录，自动录入新文件</span>
        </el-form-item>

        <el-form-item label="扫盘间隔（分钟）" v-if="config.AutoFileScan">
          <el-input-number 
            v-model="config.FileScanInterval" 
            :min="10" 
            :max="1440"
            :step="10"
          />
          <span class="help-text">扫描间隔时间，最小10分钟</span>
        </el-form-item>

        <el-form-item label="文件最小年龄（小时）" v-if="config.AutoFileScan">
          <el-input-number 
            v-model="config.FileScanMinAge" 
            :min="1" 
            :max="72"
          />
          <span class="help-text">文件创建超过此时间才录入，避免扫描正在写入的文件（推荐12小时）</span>
        </el-form-item>

        <el-form-item label="文件最小大小（MB）" v-if="config.AutoFileScan">
          <el-input-number 
            v-model="fileScanMinSizeMB" 
            :min="1" 
            :max="10240"
            @change="updateFileScanMinSize"
          />
          <span class="help-text">小于此大小的文件将被忽略</span>
        </el-form-item>

        <el-form-item label="文件最大年龄（小时）" v-if="config.AutoFileScan">
          <el-input-number 
            v-model="fileScanMaxAgeHours" 
            :min="24" 
            :max="8760"
            :step="24"
            @change="updateFileScanMaxAge"
          />
          <span class="help-text">超过此时间的文件将被忽略（默认30天）</span>
        </el-form-item>

        <el-divider />

        <el-form-item label="孤儿文件扫描">
          <el-switch 
            v-model="config.EnableOrphanScan" 
            @change="toggleFeature('EnableOrphanScan', $event)"
            active-text="开启"
            inactive-text="关闭"
          />
          <span class="help-text">启用后，定时清理无关联的历史记录</span>
        </el-form-item>

        <el-form-item label="孤儿扫描间隔（分钟）" v-if="config.EnableOrphanScan">
          <el-input-number 
            v-model="config.OrphanScanInterval" 
            :min="60" 
            :max="1440"
            :step="60"
          />
          <span class="help-text">孤儿文件扫描间隔时间，最小1小时</span>
        </el-form-item>

        <el-form-item label="工作目录">
          <el-input v-model="config.WorkPath" placeholder="/path/to/recordings" />
          <span class="help-text">录制文件存放的根目录</span>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { VideoCamera, Upload, Clock, Warning } from '@element-plus/icons-vue'
import api from '../api'

const loading = ref(false)
const saving = ref(false)
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
    if (response.data.code === 0) {
      config.value = response.data.data
    } else {
      ElMessage.error(response.data.message || '加载配置失败')
    }
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
    if (response.data.code === 0) {
      stats.value = response.data.data
    }
  } catch (error) {
    console.error('加载统计数据失败:', error)
  }
}

// 切换功能开关（实时生效）
const toggleFeature = async (feature, enabled) => {
  try {
    const response = await api.post('/config/toggle', {
      feature,
      enabled
    })
    if (response.data.code === 0) {
      ElMessage.success(`${getFeatureName(feature)}已${enabled ? '开启' : '关闭'}`)
    } else {
      ElMessage.error(response.data.message || '切换失败')
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
    if (response.data.code === 0) {
      ElMessage.success('配置保存成功')
      loadConfig()
    } else {
      ElMessage.error(response.data.message || '保存失败')
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

onMounted(() => {
  loadConfig()
  loadStats()
  
  // 每30秒刷新统计数据
  setInterval(loadStats, 30000)
})
</script>

<style scoped>
.dashboard {
  padding: 20px;
}

.header-card {
  margin-bottom: 20px;
}

.header-card h2 {
  margin: 0 0 10px 0;
  font-size: 24px;
  color: #303133;
}

.subtitle {
  margin: 0;
  color: #909399;
  font-size: 14px;
}

.stats-row {
  margin-bottom: 20px;
}

.stat-card {
  margin-bottom: 20px;
}

.stat-item {
  display: flex;
  align-items: center;
}

.stat-icon {
  font-size: 48px;
  margin-right: 20px;
}

.stat-content {
  flex: 1;
}

.stat-value {
  font-size: 32px;
  font-weight: bold;
  color: #303133;
  line-height: 1;
  margin-bottom: 8px;
}

.stat-label {
  font-size: 14px;
  color: #909399;
}

.config-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.help-text {
  margin-left: 10px;
  color: #909399;
  font-size: 12px;
}

:deep(.el-form-item) {
  margin-bottom: 24px;
}

:deep(.el-divider) {
  margin: 30px 0;
}

@media (max-width: 768px) {
  .dashboard {
    padding: 10px;
  }
  
  .stat-icon {
    font-size: 36px;
    margin-right: 15px;
  }
  
  .stat-value {
    font-size: 24px;
  }
}
</style>
