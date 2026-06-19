<template>
  <div class="users-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>用户列表</span>
          <div class="header-actions">
            <el-button type="success" plain :disabled="selectedUsers.length === 0" :loading="exportingUsers" @click="handleExportSelectedUsers">
              <el-icon><Download /></el-icon>
              导出选中
            </el-button>
            <el-button type="warning" plain :loading="importingUsers" @click="triggerUserImport">
              <el-icon><Upload /></el-icon>
              导入用户
            </el-button>
            <el-button type="primary" plain @click="showRateLimitDialog = true">
              <el-icon><Setting /></el-icon>
              上传限速
            </el-button>
            <el-button type="primary" @click="handleLogin">
              <el-icon><Plus /></el-icon>
              添加用户
            </el-button>
          </div>
        </div>
      </template>

      <input ref="userImportInputRef" type="file" accept=".json,application/json" class="hidden-file-input" @change="handleImportUsers" />

      <el-table :data="users" style="width: 100%" v-loading="loading" @selection-change="handleUserSelectionChange">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="uname" label="用户名" width="150" />
        <el-table-column prop="uid" label="UID" width="150" />
        <el-table-column label="头像" width="80">
          <template #default="{ row }">
            <el-avatar :src="row.face" />
          </template>
        </el-table-column>
        <el-table-column label="启用" width="100">
          <template #default="{ row }">
            <el-switch
              v-model="row.enabled"
              :loading="row.updatingEnabled"
              @change="handleEnabledChange(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="每日配额" width="160">
          <template #default="{ row }">
            <el-input-number
              v-model="row.dailyUploadQuota"
              :min="0"
              :max="10000"
              :step="1"
              controls-position="right"
              :disabled="row.updatingQuota"
              @change="handleDailyQuotaChange(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="Cookie状态" width="120">
          <template #default="{ row }">
            <el-tag :type="getCookieStatusType(row)">
              {{ row.login ? '有效' : '无效' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Cookie有效期" width="180">
          <template #default="{ row }">
            {{ formatTime(row.expireTime) }}
          </template>
        </el-table-column>
        <el-table-column label="最近巡检" width="220">
          <template #default="{ row }">
            <div class="check-status">
              <span>{{ formatTime(row.lastCheckTime) }}</span>
              <el-tooltip
                v-if="row.lastCheckError"
                :content="row.lastCheckError"
                placement="top"
              >
                <el-tag type="danger" size="small">异常</el-tag>
              </el-tooltip>
              <el-tag v-else-if="row.lastCheckTime" type="success" size="small">正常</el-tag>
              <el-tag v-else type="info" size="small" effect="plain">未检查</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="WxPusher" width="120">
          <template #default="{ row }">
            <el-tag :type="row.hasWxPushToken ? 'success' : 'info'">
              {{ row.hasWxPushToken ? '已配置' : '未配置' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="添加时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" fixed="right">
          <template #default="{ row }">
            <el-button
              size="small"
              @click="handleCheckStatus(row)"
              :loading="row.checking"
            >
              检查状态
            </el-button>
            <el-button
              size="small"
              @click="handleEditWxPush(row)"
            >
              配置推送
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="handleDelete(row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 登录对话框 -->
    <el-dialog
      v-model="loginDialogVisible"
      title="添加B站用户"
      width="500px"
      :close-on-click-modal="false"
    >
      <el-tabs v-model="loginMethod" class="login-tabs">
        <!-- 扫码登录 -->
        <el-tab-pane label="扫码登录" name="qrcode">
          <QrcodeLogin
            :qrcode-url="qrcodeUrl"
            :qrcode-loading="qrcodeLoading"
            :login-status="loginStatus"
            :qrcode-type="qrcodeType"
            @regenerate="generateQRCode"
            @type-change="handleQRTypeChange"
          />
        </el-tab-pane>

        <!-- Cookie登录 -->
        <el-tab-pane label="Cookie登录" name="cookie">
          <CookieLogin v-model:cookie-input="cookieInput" />
        </el-tab-pane>
      </el-tabs>

      <template #footer>
        <el-button @click="cancelLogin">取消</el-button>
        <el-button 
          v-if="loginMethod === 'qrcode' && qrcodeUrl" 
          type="primary" 
          @click="generateQRCode"
        >
          重新生成
        </el-button>
        <el-button 
          v-if="loginMethod === 'cookie'" 
          type="primary" 
          @click="handleCookieLogin"
          :loading="cookieLoginLoading"
        >
          确认登录
        </el-button>
      </template>
    </el-dialog>

    <!-- 上传限速对话框 -->
    <RateLimitDialog
      v-model:visible="showRateLimitDialog"
      :config="rateLimitConfig"
      @save="handleSaveRateLimit"
    />

    <!-- WxPusher配置对话框 -->
    <WxPushDialog
      v-model:visible="showWxPushDialog"
      :form="wxPushForm"
      @save="handleSaveWxPush"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Setting, Download, Upload } from '@element-plus/icons-vue'
import { userAPI } from '@/api'
import axios from 'axios'
import QrcodeLogin from '@/components/users/QrcodeLogin.vue'
import CookieLogin from '@/components/users/CookieLogin.vue'
import RateLimitDialog from '@/components/users/RateLimitDialog.vue'
import WxPushDialog from '@/components/users/WxPushDialog.vue'
import { useQrcodeLogin, useCookieLogin } from '@/composables/useUserLogin'

const users = ref([])
const loading = ref(false)
const loginDialogVisible = ref(false)
const loginMethod = ref('qrcode')
const showRateLimitDialog = ref(false)
const showWxPushDialog = ref(false)
const selectedUsers = ref([])
const exportingUsers = ref(false)
const importingUsers = ref(false)
const userImportInputRef = ref(null)

// 使用composables
const {
  qrcodeUrl,
  qrcodeLoading,
  loginStatus,
  qrcodeType,
  generateQRCode,
  stopPolling,
  cleanup,
  setOnSuccess
} = useQrcodeLogin()

// 扫码登录成功后自动关闭弹窗并刷新用户列表
setOnSuccess(() => {
  loginDialogVisible.value = false
  cleanup()
  fetchUsers()
})

const {
  cookieInput,
  cookieLoginLoading,
  handleLogin: cookieLogin
} = useCookieLogin()

const rateLimitConfig = ref({
  enabled: false,
  speedMBps: 10
})

const wxPushForm = ref({
  userId: null,
  token: ''
})

const fetchUsers = async () => {
  loading.value = true
  try {
    const data = await userAPI.list()
    users.value = (data || []).map(user => ({
      ...user,
      enabled: user.enabled !== false,
      dailyUploadQuota: Number(user.dailyUploadQuota || 0)
    }))
  } catch (error) {
    console.error('获取用户列表失败:', error)
  } finally {
    loading.value = false
  }
}

const handleUserSelectionChange = (selection) => {
  selectedUsers.value = selection
}

const handleExportSelectedUsers = async () => {
  if (!selectedUsers.value.length) {
    ElMessage.warning('请先选择要导出的用户')
    return
  }
  try {
    await ElMessageBox.confirm(
      `将导出 ${selectedUsers.value.length} 个用户的迁移文件，包含 Cookie 和 AccessKey 等敏感凭据。请妥善保存。`,
      '导出用户',
      { type: 'warning', confirmButtonText: '导出' }
    )
    exportingUsers.value = true
    const blob = await userAPI.export({
      ids: selectedUsers.value.map(user => user.id),
      includeSecrets: true
    })
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `gobup-users-${Date.now()}.json`
    a.click()
    window.URL.revokeObjectURL(url)
    ElMessage.success('用户导出成功')
  } catch (error) {
    if (error !== 'cancel') {
      console.error('导出用户失败:', error)
      ElMessage.error('导出用户失败')
    }
  } finally {
    exportingUsers.value = false
  }
}

const triggerUserImport = () => {
  userImportInputRef.value?.click()
}

const handleImportUsers = async (event) => {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  try {
    await ElMessageBox.confirm(
      '导入会按 UID 覆盖同名账号的迁移字段，请确认文件来源可信。',
      '导入用户',
      { type: 'warning', confirmButtonText: '导入' }
    )
    importingUsers.value = true
    const result = await userAPI.import(file)
    if (result.type === 'success') {
      ElMessage.success(result.msg || '导入成功')
      fetchUsers()
    } else {
      ElMessage.error(result.msg || '导入失败')
    }
  } catch (error) {
    if (error !== 'cancel') {
      console.error('导入用户失败:', error)
      ElMessage.error('导入用户失败')
    }
  } finally {
    importingUsers.value = false
  }
}

const handleLogin = () => {
  loginDialogVisible.value = true
  loginMethod.value = 'qrcode'
  cookieInput.value = ''
  cleanup()
  nextTick(() => {
    generateQRCode()
  })
}

const handleQRTypeChange = (type) => {
  qrcodeType.value = type
  if (loginDialogVisible.value) {
    stopPolling()
    generateQRCode()
  }
}

const handleCookieLogin = async () => {
  const result = await cookieLogin()
  if (result.success) {
    loginDialogVisible.value = false
    fetchUsers()
  }
}

const cancelLogin = () => {
  stopPolling()
  loginDialogVisible.value = false
  cookieInput.value = ''
  cleanup()
}

const handleCheckStatus = async (row) => {
  row.checking = true
  try {
    const result = await userAPI.checkStatus(row.id)
    if (result.type === 'success') {
      ElMessage.success(result.msg || 'Cookie有效，用户状态正常')
      if (result.user) {
        Object.assign(row, result.user)
      }
    } else {
      ElMessage.error(result.msg || 'Cookie已失效')
      if (result.user) {
        Object.assign(row, result.user)
      }
    }
  } catch (error) {
    console.error('检查状态失败:', error)
    ElMessage.error('检查失败，请稍后重试')
  } finally {
    row.checking = false
  }
}

const handleEnabledChange = async (row) => {
  const nextEnabled = row.enabled
  row.updatingEnabled = true
  try {
    const result = await userAPI.setEnabled(row.id, nextEnabled)
    if (result.type === 'success') {
      ElMessage.success(result.msg || (nextEnabled ? '账号已启用' : '账号已禁用'))
    } else {
      row.enabled = !nextEnabled
      ElMessage.error(result.msg || '状态更新失败')
    }
  } catch (error) {
    row.enabled = !nextEnabled
    console.error('更新账号状态失败:', error)
    ElMessage.error('状态更新失败')
  } finally {
    row.updatingEnabled = false
  }
}

const handleDailyQuotaChange = async (row) => {
  const quota = Math.max(0, Number(row.dailyUploadQuota) || 0)
  row.dailyUploadQuota = quota
  row.updatingQuota = true
  try {
    const result = await userAPI.update({
      id: row.id,
      dailyUploadQuota: quota
    })
    if (result.type === 'success') {
      ElMessage.success(quota > 0 ? `每日配额已设为 ${quota} 个分P` : '每日配额已设为不限额')
    } else {
      ElMessage.error(result.msg || '配额更新失败')
      fetchUsers()
    }
  } catch (error) {
    console.error('更新每日配额失败:', error)
    ElMessage.error('配额更新失败')
    fetchUsers()
  } finally {
    row.updatingQuota = false
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确定要删除这个用户吗？', '提示', {
      type: 'warning'
    })
    await userAPI.delete(row.id)
    ElMessage.success('删除成功')
    fetchUsers()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('删除失败:', error)
    }
  }
}

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString('zh-CN')
}

const getCookieStatusType = (row) => {
  if (!row.login) return 'danger'
  if (!row.expireTime) return 'success'
  const expireAt = new Date(row.expireTime).getTime()
  if (!Number.isFinite(expireAt)) return 'success'
  const daysLeft = (expireAt - Date.now()) / 86400000
  if (daysLeft <= 0) return 'danger'
  if (daysLeft <= 7) return 'warning'
  return 'success'
}

const loadRateLimitConfig = async () => {
  try {
    const { data } = await axios.get('/api/ratelimit/config')
    rateLimitConfig.value = {
      enabled: data.enabled || false,
      speedMBps: data.speedMBps || 10
    }
  } catch (error) {
    console.error('获取限速配置失败:', error)
  }
}

const handleSaveRateLimit = async (config) => {
  try {
    await axios.post('/api/ratelimit/config', config)
    ElMessage.success('限速配置已保存')
    showRateLimitDialog.value = false
  } catch (error) {
    console.error('保存限速配置失败:', error)
    ElMessage.error('保存失败')
  }
}

const handleEditWxPush = (row) => {
  wxPushForm.value = {
    userId: row.id,
    token: ''
  }
  showWxPushDialog.value = true
}

const handleSaveWxPush = async (form) => {
  try {
    await userAPI.update({
      id: form.userId,
      wxPushToken: form.token
    })
    ElMessage.success('WxPusher配置已保存')
    showWxPushDialog.value = false
    fetchUsers()
  } catch (error) {
    console.error('保存WxPusher配置失败:', error)
    ElMessage.error('保存失败')
  }
}

onMounted(() => {
  fetchUsers()
  loadRateLimitConfig()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.hidden-file-input {
  display: none;
}

.login-tabs {
  margin-top: -10px;
}

.check-status {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
</style>
