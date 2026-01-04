<template>
  <div class="users-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>用户列表</span>
          <div class="header-actions">
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

      <el-table :data="users" style="width: 100%" v-loading="loading">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="uname" label="用户名" width="150" />
        <el-table-column prop="uid" label="UID" width="150" />
        <el-table-column label="头像" width="80">
          <template #default="{ row }">
            <el-avatar :src="row.face" />
          </template>
        </el-table-column>
        <el-table-column label="Cookie状态" width="120">
          <template #default="{ row }">
            <el-tag :type="row.login ? 'success' : 'danger'">
              {{ row.login ? '有效' : '无效' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="WxPusher" width="120">
          <template #default="{ row }">
            <el-tag :type="row.wxPushToken ? 'success' : 'info'">
              {{ row.wxPushToken ? '已配置' : '未配置' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="弹幕代理" width="120">
          <template #default="{ row }">
            <el-tag :type="row.enableDanmakuProxy ? 'success' : 'info'">
              {{ row.enableDanmakuProxy ? '已启用' : '未启用' }}
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
              type="primary"
              @click="handleEditProxy(row)"
            >
              代理配置
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
            :visible="loginMethod === 'qrcode' && loginDialogVisible"
            :qrcode-url="qrcodeUrl"
            :qrcode-loading="qrcodeLoading"
            :login-status="loginStatus"
            :qrcode-type="qrcodeType"
            @cancel="cancelLogin"
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

    <!-- 代理配置对话框 -->
    <ProxyConfigDialog
      v-model:visible="showProxyDialog"
      :config="proxyConfig"
      @save="handleSaveProxy"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Setting } from '@element-plus/icons-vue'
import { userAPI } from '@/api'
import axios from 'axios'
import QrcodeLogin from '@/components/users/QrcodeLogin.vue'
import CookieLogin from '@/components/users/CookieLogin.vue'
import RateLimitDialog from '@/components/users/RateLimitDialog.vue'
import WxPushDialog from '@/components/users/WxPushDialog.vue'
import ProxyConfigDialog from '@/components/users/ProxyConfigDialog.vue'
import { useQrcodeLogin, useCookieLogin } from '@/composables/useUserLogin'

const users = ref([])
const loading = ref(false)
const loginDialogVisible = ref(false)
const loginMethod = ref('qrcode')
const showRateLimitDialog = ref(false)
const showWxPushDialog = ref(false)
const showProxyDialog = ref(false)

// 使用composables
const {
  qrcodeUrl,
  qrcodeLoading,
  loginStatus,
  qrcodeType,
  generateQRCode,
  stopPolling,
  cleanup
} = useQrcodeLogin()

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

const proxyConfig = ref({
  userId: null,
  enableDanmakuProxy: false,
  danmakuProxyList: ''
})

const fetchUsers = async () => {
  loading.value = true
  try {
    const data = await userAPI.list()
    users.value = data || []
  } catch (error) {
    console.error('获取用户列表失败:', error)
  } finally {
    loading.value = false
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
    token: row.wxPushToken || ''
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

const handleEditProxy = (row) => {
  proxyConfig.value = {
    userId: row.id,
    enableDanmakuProxy: row.enableDanmakuProxy || false,
    danmakuProxyList: row.danmakuProxyList || ''
  }
  showProxyDialog.value = true
}

const handleSaveProxy = async (config) => {
  try {
    await userAPI.update({
      id: config.userId,
      enableDanmakuProxy: config.enableDanmakuProxy,
      danmakuProxyList: config.danmakuProxyList
    })
    ElMessage.success('代理配置已保存')
    showProxyDialog.value = false
    fetchUsers()
  } catch (error) {
    console.error('保存代理配置失败:', error)
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
}

.login-tabs {
  margin-top: -10px;
}
</style>
