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
        <el-table-column prop="createdAt" label="添加时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
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
      title="扫码登录"
      width="400px"
      :close-on-click-modal="false"
    >
      <div class="qrcode-container">
        <div v-if="qrcodeLoading" class="loading">
          <el-icon class="is-loading"><Loading /></el-icon>
          <p>生成二维码中...</p>
        </div>
        <div v-else-if="qrcodeUrl" class="qrcode">
          <div ref="qrcodeRef" class="qrcode-image"></div>
          <p class="tip">请使用哔哩哔哩APP扫描二维码登录</p>
          <p class="status">{{ loginStatus }}</p>
        </div>
      </div>
      <template #footer>
        <el-button @click="cancelLogin">取消</el-button>
        <el-button type="primary" @click="handleLogin">重新生成</el-button>
      </template>
    </el-dialog>

    <!-- 上传限速对话框 -->
    <el-dialog v-model="showRateLimitDialog" title="上传限速配置" width="400px">
      <el-form label-width="100px">
        <el-form-item label="启用限速">
          <el-switch v-model="rateLimitConfig.enabled" />
        </el-form-item>
        <el-form-item label="限速(MB/s)" v-if="rateLimitConfig.enabled">
          <el-input-number
            v-model="rateLimitConfig.speedMBps"
            :min="1"
            :max="100"
            :step="0.5"
          />
          <div style="margin-top: 8px; font-size: 12px; color: #999;">
            设置上传速度上限，避免占用过多带宽
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRateLimitDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSaveRateLimit">保存</el-button>
      </template>
    </el-dialog>

    <!-- WxPusher配置对话框 -->
    <el-dialog v-model="showWxPushDialog" title="配置WxPusher推送" width="500px">
      <el-form label-width="120px">
        <el-form-item label="WxPusher Token">
          <el-input
            v-model="wxPushForm.token"
            placeholder="请输入WxPusher AppToken"
            clearable
          />
          <div style="margin-top: 8px; font-size: 12px; color: #999;">
            在 <a href="https://wxpusher.zjiecode.com" target="_blank">WxPusher官网</a> 注册获取AppToken
          </div>
        </el-form-item>
        <el-form-item label="说明">
          <div style="font-size: 13px; color: #666; line-height: 1.6;">
            <p>配置后，可在房间设置中填写微信UID，实现以下推送通知：</p>
            <ul style="padding-left: 20px; margin: 5px 0;">
              <li>开播通知</li>
              <li>上传进度通知</li>
              <li>投稿成功通知</li>
              <li>上传失败提醒</li>
            </ul>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showWxPushDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSaveWxPush">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { userAPI } from '@/api'
import axios from 'axios'
import QRCode from 'qrcode'

const users = ref([])
const loading = ref(false)
const loginDialogVisible = ref(false)
const qrcodeLoading = ref(false)
const showRateLimitDialog = ref(false)
const showWxPushDialog = ref(false)

const rateLimitConfig = ref({
  enabled: false,
  speedMBps: 10
})

const wxPushForm = ref({
  userId: null,
  token: ''
})

const qrcodeUrl = ref('')
const qrcodeRef = ref(null)
const loginStatus = ref('等待扫码...')
let authKey = ''
let pollingTimer = null

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

const handleLogin = async () => {
  loginDialogVisible.value = true
  qrcodeLoading.value = true
  loginStatus.value = '等待扫码...'
  
  try {
    const data = await userAPI.login()
    qrcodeUrl.value = data.url
    authKey = data.authCode
    
    await nextTick()
    
    // 生成二维码
    if (qrcodeRef.value) {
      qrcodeRef.value.innerHTML = ''
      await QRCode.toCanvas(qrcodeUrl.value, {
        width: 200,
        margin: 1
      }).then(canvas => {
        qrcodeRef.value.appendChild(canvas)
      })
    }
    
    // 开始轮询登录状态
    startPolling()
  } catch (error) {
    console.error('获取二维码失败:', error)
    ElMessage.error('获取二维码失败')
  } finally {
    qrcodeLoading.value = false
  }
}

const startPolling = () => {
  stopPolling()
  
  pollingTimer = setInterval(async () => {
    try {
      const data = await userAPI.loginReturn(authKey)
      
      if (data.code === 0) {
        loginStatus.value = '登录成功！'
        ElMessage.success('登录成功')
        stopPolling()
        loginDialogVisible.value = false
        fetchUsers()
      } else if (data.code === 86038) {
        loginStatus.value = '二维码已过期，请重新获取'
        stopPolling()
      } else if (data.code === 86090) {
        loginStatus.value = '已扫码，等待确认...'
      }
    } catch (error) {
      console.error('查询登录状态失败:', error)
    }
  }, 2000)
}

const stopPolling = () => {
  if (pollingTimer) {
    clearInterval(pollingTimer)
    pollingTimer = null
  }
}

const cancelLogin = () => {
  stopPolling()
  loginDialogVisible.value = false
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

// 加载限速配置
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

// 保存限速配置
const handleSaveRateLimit = async () => {
  try {
    await axios.post('/api/ratelimit/config', rateLimitConfig.value)
    ElMessage.success('限速配置已保存')
    showRateLimitDialog.value = false
  } catch (error) {
    console.error('保存限速配置失败:', error)
    ElMessage.error('保存失败')
  }
}

// 编辑WxPusher配置
const handleEditWxPush = (row) => {
  wxPushForm.value = {
    userId: row.id,
    token: row.wxPushToken || ''
  }
  showWxPushDialog.value = true
}

// 保存WxPusher配置
const handleSaveWxPush = async () => {
  try {
    await userAPI.update({
      id: wxPushForm.value.userId,
      wxPushToken: wxPushForm.value.token
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
}

.qrcode-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 300px;
}

.loading {
  text-align: center;
}

.loading .el-icon {
  font-size: 40px;
  margin-bottom: 10px;
}

.qrcode {
  text-align: center;
}

.qrcode-image {
  margin-bottom: 15px;
}

.tip {
  color: #666;
  font-size: 14px;
  margin-bottom: 10px;
}

.status {
  color: #1890ff;
  font-size: 14px;
  font-weight: bold;
}
</style>
