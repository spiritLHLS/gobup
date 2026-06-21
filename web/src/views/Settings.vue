<template>
  <div class="dashboard-container">
    <!-- 页面标题 -->
    <div class="page-header">
      <h2>系统设置</h2>
      <p>管理扫盘、维护、弹幕烧录、Agent 和全局运行配置</p>
    </div>

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

          <el-form-item label="目录事件监控" v-if="config.autoFileScan">
            <div class="switch-item">
              <el-switch
                v-model="config.enableFileWatcher"
                @change="toggleFeature('enableFileWatcher', $event)"
                size="large"
              />
              <span class="help-text">监听录制目录变化并触发扫盘，定时扫描仍作为兜底</span>
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

          <el-form-item label="全局上传限速">
            <div class="number-input-wrapper">
              <el-input-number
                v-model="config.uploadSpeedLimitMbps"
                :min="0"
                :max="1000"
                :step="0.5"
                :precision="1"
                size="large"
              />
              <span class="help-text">MB/s，0 表示不限制；房间级限速会覆盖全局限速</span>
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

        <PublishAgentConfig
          :config="config"
          :detecting-agent="detectingAgent"
          :checking-files="checkingAgentFiles"
          :generating-install-command="generatingInstallCommand"
          :install-command="agentInstallCommand"
          :file-check-result="agentFileCheckResult"
          @toggle-feature="toggleFeature"
          @detect="detectPublishAgent"
          @check-files="checkAgentFiles"
          @load-install-command="loadAgentInstallCommand"
        />

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
              <span class="help-text">检查并修复数据不一致问题（孤儿分P、孤立历史记录、时间错误等）</span>
            </div>
          </el-form-item>

          <el-form-item label="数据库瘦身">
            <div class="button-group">
              <el-button 
                type="info" 
                @click="previewDatabaseCleanup" 
                :loading="cleanupPreviewing"
                :icon="View"
              >
                预览清理
              </el-button>
              <el-button 
                type="danger" 
                @click="cleanupDatabase" 
                :loading="cleaningDatabase"
                :icon="Delete"
              >
                执行清理
              </el-button>
              <span class="help-text">删除已软删除的记录（文件已删除但数据库仍保留的记录）</span>
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

        <DanmakuBurnGlobalConfig :config="config" />

        <el-divider />

        <DanmakuProxyConfig :config="config" @toggle-feature="toggleFeature" />
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
import { Check, Refresh, FolderOpened, Search, Tools, Delete, View } from '@element-plus/icons-vue'
import api, { filescanAPI, dataRepairAPI } from '../api'
import FileScanDialog from '../components/filescan/FileScanDialog.vue'
import CleanFilesDialog from '../components/filescan/CleanFilesDialog.vue'
import DanmakuBurnGlobalConfig from '../components/dashboard/DanmakuBurnGlobalConfig.vue'
import DanmakuProxyConfig from '../components/dashboard/DanmakuProxyConfig.vue'
import PublishAgentConfig from '../components/dashboard/PublishAgentConfig.vue'
import { createDefaultDashboardConfig, normalizeDashboardConfig } from '../utils/dashboardConfig'

const loading = ref(false)
const saving = ref(false)
const scanning = ref(false)
const checking = ref(false)
const repairing = ref(false)
const cleaning = ref(false)
const cleanupPreviewing = ref(false)
const cleaningDatabase = ref(false)
const detectingAgent = ref(false)
const checkingAgentFiles = ref(false)
const generatingInstallCommand = ref(false)
const agentInstallCommand = ref(null)
const agentFileCheckResult = ref(null)
const fileScanDialogRef = ref(null)
const cleanFilesDialogRef = ref(null)
const config = ref(createDefaultDashboardConfig())

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

// 加载配置
const loadConfig = async () => {
  loading.value = true
  try {
    const response = await api.get('/config/system')
    // 后端直接返回 config 对象
    config.value = normalizeDashboardConfig(config.value, response)
  } catch (error) {
    console.error('加载配置失败:', error)
    ElMessage.error('加载配置失败: ' + (error.message || '网络错误'))
  } finally {
    loading.value = false
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
        config.value = normalizeDashboardConfig(config.value, response.data)
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
    enableFileWatcher: '目录事件监控',
    autoDataRepair: '自动数据修复',
    enableOrphanScan: '孤儿文件扫描',
    enableDanmakuProxy: '弹幕代理池',
    uploadWhileRecording: '边录制边上传',
    publishWhileRecording: '边录制边投稿'
  }
  return names[feature] || feature
}

const detectPublishAgent = async (purpose) => {
  detectingAgent.value = true
  try {
    const response = await api.get('/agent/detect', {
      params: { purpose: purpose || config.value.agentPurpose }
    })
    if (response.type === 'success') {
      ElMessage.success(response.msg || '远程 Agent 可用')
    } else {
      ElMessage.error(response.msg || '远程 Agent 不可用')
    }
  } catch (error) {
    console.error('检测远程 Agent 失败:', error)
    ElMessage.error('检测远程 Agent 失败')
  } finally {
    detectingAgent.value = false
  }
}

const checkAgentFiles = async () => {
  checkingAgentFiles.value = true
  try {
    const response = await api.get('/agent/files/check', { params: { limit: 100 } })
    if (response.type === 'success') {
      agentFileCheckResult.value = response.data || null
      ElMessage.success(response.msg || '文件检查完成')
    } else {
      ElMessage.error(response.msg || '文件检查失败')
    }
  } catch (error) {
    console.error('Agent 文件检查失败:', error)
    ElMessage.error('Agent 文件检查失败')
  } finally {
    checkingAgentFiles.value = false
  }
}

const loadAgentInstallCommand = async () => {
  generatingInstallCommand.value = true
  try {
    const response = await api.get('/agent/install-command', {
      params: {
        purpose: config.value.agentPurpose,
        source: config.value.agentInstallerSource
      }
    })
    if (response.type === 'success') {
      agentInstallCommand.value = response
      if (response.tokenMissing) {
        ElMessage.warning('请先配置并保存 Agent Token')
      } else {
        ElMessage.success('安装命令已生成')
      }
    } else {
      ElMessage.error(response.msg || '生成安装命令失败')
    }
  } catch (error) {
    console.error('生成 Agent 安装命令失败:', error)
    ElMessage.error('生成安装命令失败')
  } finally {
    generatingInstallCommand.value = false
  }
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
      '将检查以下数据一致性问题（不会修改数据）：\n' +
      '1. 孤儿分P（有分P但无历史记录）\n' +
      '2. 孤立历史记录（有历史记录但无分P）\n' +
      '3. 时间范围错误\n\n' +
      '是否继续？',
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
      message += `发现孤立历史记录: ${response.emptyHistories} 个\n`
      message += `\n注：孤立历史记录将被删除（保留正在录制中的和最近10分钟内创建的）`
      
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
      '1. 孤儿分P（有分P但无历史记录） → 创建或关联历史记录\n' +
      '2. 孤立历史记录（有历史记录但无分P） → 删除（保留正在录制中的）\n' +
      '3. 历史记录时间范围错误 → 根据分P更新\n\n' +
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
      message += `发现孤儿分P: ${response.orphanParts} 个\n`
      message += `发现孤立历史记录: ${response.emptyHistories} 个\n`
      
      if (hasChanges) {
        message += `\n修复操作：\n`
        if (response.createdHistories > 0) {
          message += `- 创建历史记录: ${response.createdHistories} 个\n`
        }
        if (response.deletedEmptyHistories > 0) {
          message += `- 删除孤立历史记录: ${response.deletedEmptyHistories} 个\n`
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

// 预览数据库瘦身
const previewDatabaseCleanup = async () => {
  try {
    cleanupPreviewing.value = true
    const response = await api.post('/config/cleanup?preview=true')
    
    if (response.code === 0) {
      const { deletedPartsCount, orphanHistoriesCount } = response.data
      
      ElMessageBox.alert(
        `预计清理：\n` +
        `• 已软删除的分P记录：${deletedPartsCount} 条\n` +
        `• 孤立的历史记录（没有任何有效分P）：${orphanHistoriesCount} 条\n\n` +
        `这些记录对应的文件已被删除，但数据库仍保留记录。\n` +
        `执行清理将永久删除这些数据库记录，释放数据库空间。`,
        '数据库瘦身预览',
        {
          confirmButtonText: '确定',
          type: 'info',
          dangerouslyUseHTMLString: false
        }
      )
    } else {
      ElMessage.error('预览失败: ' + (response.msg || '未知错误'))
    }
  } catch (error) {
    console.error('预览失败:', error)
    ElMessage.error('预览失败: ' + (error.response?.data?.msg || error.message || '网络错误'))
  } finally {
    cleanupPreviewing.value = false
  }
}

// 执行数据库瘦身
const cleanupDatabase = async () => {
  try {
    await ElMessageBox.confirm(
      '此操作将永久删除已软删除的记录（文件已删除但数据库仍保留），是否继续？',
      '确认数据库瘦身',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    cleaningDatabase.value = true
    const response = await api.post('/config/cleanup')
    
    if (response.code === 0) {
      const { deletedPartsCount, orphanHistoriesCount } = response.data
      ElMessage.success(
        `清理完成！删除了 ${deletedPartsCount} 条分P记录，${orphanHistoriesCount} 条历史记录`
      )
    } else {
      ElMessage.error('清理失败: ' + (response.msg || '未知错误'))
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('清理失败:', error)
      ElMessage.error('清理失败: ' + (error.response?.data?.msg || error.message || '网络错误'))
    }
  } finally {
    cleaningDatabase.value = false
  }
}

// 文件导入完成后的处理
const handleFilesImported = () => {}

// 清理已完成文件（xml和jpg）
const cleanCompletedFiles = async () => {
  // 打开文件选择对话框
  cleanFilesDialogRef.value?.open()
}

// 处理文件清理成功
const handleFilesCleanSuccess = () => {
  ElMessage.success('文件清理完成')
}

onMounted(() => {
  loadConfig()
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
@media (max-width: 768px) {
  .page-header h2 {
    font-size: var(--font-size-2xl);
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
