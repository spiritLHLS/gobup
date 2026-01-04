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
          <div class="section-title">文件扫描</div>
          
          <el-form-item label="自动扫盘录入">
            <div class="switch-item">
              <el-switch 
                v-model="config.autoFileScan" 
                @change="toggleFeature('autoFileScan', $event)"
                size="large"
              />
              <span class="help-text">启用后，定时扫描录制目录，自动录入新文件</span>
            </div>
          </el-form-item>

          <el-form-item label="扫盘间隔（分钟）" v-if="config.autoFileScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="config.fileScanInterval" 
                :min="10" 
                :max="1440"
                :step="10"
                size="large"
              />
              <span class="help-text">扫描间隔时间，最小10分钟</span>
            </div>
          </el-form-item>

          <el-form-item label="文件最小年龄（小时）" v-if="config.autoFileScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="config.fileScanMinAge" 
                :min="1" 
                :max="72"
                size="large"
              />
              <span class="help-text">文件创建超过此时间才录入，避免扫描正在写入的文件（推荐12小时）</span>
            </div>
          </el-form-item>

          <el-form-item label="文件最小大小（MB）" v-if="config.autoFileScan">
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

          <el-form-item label="文件最大年龄（小时）" v-if="config.autoFileScan">
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

          <el-form-item label="工作目录">
            <div class="path-input-wrapper">
              <el-input 
                v-model="config.workPath" 
                placeholder="/rec 或 /path/to/recordings"
                size="large"
              />
              <span class="help-text">录制文件存放的根目录（Docker默认/rec，裸机默认./data/recordings）</span>
            </div>
          </el-form-item>

          <el-form-item label="自定义扫描目录">
            <div class="path-input-wrapper">
              <el-input 
                v-model="config.customScanPaths" 
                placeholder="/path1,/path2,/path3"
                size="large"
                type="textarea"
                :rows="2"
              />
              <span class="help-text">额外的扫描目录，多个路径用逗号分隔，优先扫描这些目录，然后扫描工作目录</span>
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
          
          <el-form-item label="自动数据修复">
            <div class="switch-item">
              <el-switch 
                v-model="config.autoDataRepair" 
                @change="toggleFeature('autoDataRepair', $event)"
                size="large"
              />
              <span class="help-text">启用后，每天自动检查并修复数据一致性问题（孤儿分P、空历史记录等）</span>
            </div>
          </el-form-item>

          <el-form-item label="数据一致性检查">
            <div class="button-group">
              <el-button 
                type="primary" 
                @click="checkDataConsistency" 
                :loading="checking"
                :icon="Search"
              >
                检查问题
              </el-button>
              <el-button 
                type="warning" 
                @click="repairDataConsistency" 
                :loading="repairing"
                :icon="Tools"
              >
                修复数据
              </el-button>
              <span class="help-text">检查并修复分P与历史记录之间的数据不一致问题</span>
            </div>
          </el-form-item>

          <el-form-item label="孤儿文件扫描">
            <div class="switch-item">
              <el-switch 
                v-model="config.enableOrphanScan" 
                @change="toggleFeature('enableOrphanScan', $event)"
                size="large"
              />
              <span class="help-text">启用后，定时清理无关联的历史记录</span>
            </div>
          </el-form-item>

          <el-form-item label="孤儿扫描间隔（分钟）" v-if="config.enableOrphanScan">
            <div class="number-input-wrapper">
              <el-input-number 
                v-model="config.orphanScanInterval" 
                :min="60" 
                :max="1440"
                :step="60"
                size="large"
              />
              <span class="help-text">孤儿文件扫描间隔时间，最小1小时</span>
            </div>
          </el-form-item>

          <el-form-item label="孤立文件清理">
            <div class="button-group">
              <el-button 
                type="danger" 
                @click="cleanCompletedFiles" 
                :loading="cleaning"
                :icon="Delete"
              >
                清理已完成文件
              </el-button>
              <span class="help-text">删除已上传投稿成功且解析弹幕完成且已发送弹幕的对应xml文件和jpg文件</span>
            </div>
          </el-form-item>
        </div>

        <el-divider />

        <div class="form-section">
          <div class="section-title">弹幕代理配置（全局）</div>
          
          <el-form-item label="启用代理池">
            <div class="switch-item">
              <el-switch 
                v-model="config.enableDanmakuProxy" 
                @change="toggleFeature('enableDanmakuProxy', $event)"
                size="large"
              />
              <span class="help-text">启用后，发送弹幕时将轮询使用代理池中的IP，突破单IP限流</span>
            </div>
          </el-form-item>

          <el-form-item label="代理列表" v-if="config.enableDanmakuProxy">
            <div class="proxy-input-wrapper">
              <el-input
                v-model="config.danmakuProxyList"
                type="textarea"
                :rows="10"
                placeholder="每行一个代理，支持格式：&#10;socks5://ip:port&#10;socks5://user:pass@ip:port&#10;http://ip:port&#10;http://user:pass@ip:port&#10;https://ip:port&#10;&#10;示例：&#10;socks5://127.0.0.1:1080&#10;http://user:pass@proxy.example.com:8080"
                size="large"
              />
            </div>
          </el-form-item>

          <el-alert
            v-if="config.enableDanmakuProxy && proxyCount > 0"
            :title="`当前配置了 ${proxyCount} 个代理IP + 1 个本地IP，总计 ${proxyCount + 1} 个IP`"
            type="success"
            :closable="false"
            style="margin-top: 10px; margin-bottom: 10px;"
          />

          <el-alert
            v-if="config.enableDanmakuProxy && !config.danmakuProxyList"
            title="未配置代理，将仅使用本地IP"
            type="warning"
            :closable="false"
            style="margin-top: 10px; margin-bottom: 10px;"
          />

          <el-alert
            v-if="config.enableDanmakuProxy"
            type="info"
            :closable="false"
          >
            <template #default>
              <div style="font-size: 12px; line-height: 1.6;">
                <p style="margin: 4px 0;"><strong>💡 使用说明：</strong></p>
                <ul style="margin: 4px 0; padding-left: 20px;">
                  <li>每行一个代理地址，支持 socks5 和 http(s) 协议</li>
                  <li>系统会自动包含本地IP，无需单独配置</li>
                  <li>每个IP独立限流（22秒/条），实现真正的并行发送</li>
                  <li>代理池会轮询使用所有可用IP（所有用户共享此代理池）</li>
                  <li>以 # 开头的行会被忽略（可用于注释）</li>
                </ul>
              </div>
            </template>
          </el-alert>
        </div>
      </el-form>
    </el-card>

    <!-- 文件扫描对话框 -->
    <FileScanDialog ref="fileScanDialogRef" @imported="handleFilesImported" />
    
    <!-- 文件清理对话框 -->
    <CleanFilesDialog ref="cleanFilesDialogRef" @success="handleFilesCleanSuccess" />
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { VideoCamera, Upload, Clock, Warning, Check, Refresh, FolderOpened, Search, Tools, Delete } from '@element-plus/icons-vue'
import api, { filescanAPI, dataRepairAPI } from '../api'
import FileScanDialog from '../components/filescan/FileScanDialog.vue'
import CleanFilesDialog from '../components/filescan/CleanFilesDialog.vue'

const loading = ref(false)
const saving = ref(false)
const scanning = ref(false)
const checking = ref(false)
const repairing = ref(false)
const cleaning = ref(false)
const fileScanDialogRef = ref(null)
const cleanFilesDialogRef = ref(null)
const config = ref({
  autoFileScan: true,
  fileScanInterval: 60,
  fileScanMinAge: 12,
  fileScanMinSize: 1048576,
  fileScanMaxAge: 720,
  workPath: '',
  customScanPaths: '',
  autoDataRepair: false,
  enableOrphanScan: true,
  orphanScanInterval: 360,
  enableDanmakuProxy: false,
  danmakuProxyList: ''
})

const stats = ref({
  totalRecordings: 0,
  uploadedCount: 0,
  pendingCount: 0,
  failedCount: 0
})

// 计算属性：将字节转MB显示
const fileScanMinSizeMB = computed({
  get: () => Math.round(config.value.fileScanMinSize / (1024 * 1024)),
  set: (val) => {} // 空setter，实际更新在updateFileScanMinSize中
})

// 计算属性：将小时转换显示
const fileScanMaxAgeHours = computed({
  get: () => config.value.fileScanMaxAge,
  set: (val) => {} // 空setter，实际更新在updateFileScanMaxAge中
})

// 更新文件最小大小（MB转字节）
const updateFileScanMinSize = (val) => {
  config.value.fileScanMinSize = val * 1024 * 1024
}

// 更新文件最大年龄（小时）
const updateFileScanMaxAge = (val) => {
  config.value.fileScanMaxAge = val
}

// 计算代理数量
const proxyCount = computed(() => {
  if (!config.value.danmakuProxyList) {
    return 0
  }
  
  const lines = config.value.danmakuProxyList.split('\n')
  return lines.filter(line => {
    const trimmed = line.trim()
    return trimmed && !trimmed.startsWith('#')
  }).length
})

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
    const response = await api.post('/config/toggle', {
      key: feature,
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
      // 使用后端返回的最新配置更新前端
      if (response.data) {
        config.value = response.data
      }
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
    autoFileScan: '自动扫盘录入',
    autoDataRepair: '自动数据修复',
    enableOrphanScan: '孤儿文件扫描'
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

// 检查数据一致性
const checkDataConsistency = async () => {
  try {
    await ElMessageBox.confirm(
      '将检查分P与历史记录之间的数据一致性问题（不会修改数据）。是否继续？',
      '数据一致性检查',
      {
        confirmButtonText: '检查',
        cancelButtonText: '取消',
        type: 'info'
      }
    )
    
    checking.value = true
    const response = await dataRepairAPI.check(true) // dryRun=true
    
    if (response.type === 'success') {
      const hasIssues = response.orphanParts > 0 || response.emptyHistories > 0
      
      let message = `检查完成！\n\n`
      message += `发现孤儿分P: ${response.orphanParts} 个\n`
      message += `发现空历史记录: ${response.emptyHistories} 个\n`
      
      if (hasIssues) {
        message += `\n如需修复，请点击"修复数据"按钮。`
        
        if (response.errors && response.errors.length > 0) {
          message += `\n\n错误信息：\n` + response.errors.join('\n')
        }
        
        ElMessageBox.alert(message, '检查结果', { 
          type: 'warning',
          confirmButtonText: '知道了'
        })
      } else {
        ElMessage.success('数据一致性良好，未发现问题！')
      }
    } else {
      ElMessage.error(response.msg || '检查失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('数据检查失败:', error)
      ElMessage.error('检查失败: ' + (error.message || '网络错误'))
    }
  } finally {
    checking.value = false
  }
}

// 修复数据一致性
const repairDataConsistency = async () => {
  try {
    await ElMessageBox.confirm(
      '将自动修复以下问题：\n' +
      '1. 孤儿分P（有分P但无历史记录）\n' +
      '2. 空历史记录（有历史记录但无分P）\n' +
      '3. 历史记录时间范围错误\n\n' +
      '是否继续？',
      '数据一致性修复',
      {
        confirmButtonText: '修复',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    repairing.value = true
    const response = await dataRepairAPI.repair()
    
    if (response.type === 'success') {
      const hasChanges = response.createdHistories > 0 || 
                        response.deletedEmptyHistories > 0 || 
                        response.reassignedParts > 0 || 
                        response.updatedHistoryTimes > 0
      
      let message = `修复完成！\n\n`
      message += `孤儿分P: ${response.orphanParts} 个\n`
      message += `空历史记录: ${response.emptyHistories} 个\n`
      
      if (hasChanges) {
        message += `\n修复操作：\n`
        if (response.createdHistories > 0) {
          message += `- 创建历史记录: ${response.createdHistories} 个\n`
        }
        if (response.deletedEmptyHistories > 0) {
          message += `- 删除空历史记录: ${response.deletedEmptyHistories} 个\n`
        }
        if (response.reassignedParts > 0) {
          message += `- 重新分配分P: ${response.reassignedParts} 个\n`
        }
        if (response.updatedHistoryTimes > 0) {
          message += `- 更新时间范围: ${response.updatedHistoryTimes} 个\n`
        }
      }
      
      if (response.errors && response.errors.length > 0) {
        message += `\n错误信息：\n` + response.errors.join('\n')
      }
      
      ElMessageBox.alert(message, '修复结果', { 
        type: hasChanges ? 'success' : 'info',
        confirmButtonText: '知道了'
      })
      
      // 刷新统计数据
      if (hasChanges) {
        loadStats()
      }
    } else {
      ElMessage.error(response.msg || '修复失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('数据修复失败:', error)
      ElMessage.error('修复失败: ' + (error.message || '网络错误'))
    }
  } finally {
    repairing.value = false
  }
}

// 文件导入完成后的处理
const handleFilesImported = () => {
  // 刷新统计数据
  loadStats()
}

// 清理已完成文件（xml和jpg）
const cleanCompletedFiles = async () => {
  // 打开文件选择对话框
  cleanFilesDialogRef.value?.open()
}

// 处理文件清理成功
const handleFilesCleanSuccess = () => {
  ElMessage.success('文件清理完成')
  // 可选：刷新统计数据
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
