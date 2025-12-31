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
          <div class="qrcode-container">
            <!-- 登录方式选择 -->
            <div class="login-type-selector" style="margin-bottom: 16px; text-align: center;">
              <el-radio-group v-model="qrcodeType" @change="handleQRTypeChange">
                <el-radio label="tv">TV端扫码</el-radio>
                <el-radio label="web">Web端扫码</el-radio>
              </el-radio-group>
              <div style="font-size: 12px; color: #999; margin-top: 8px;">
                <template v-if="qrcodeType === 'tv'">
                  💡 推荐：稳定性更好，适合长期使用
                </template>
                <template v-else>
                  💡 兼容性更好，与网页端登录一致
                </template>
              </div>
            </div>
            
            <div v-if="qrcodeLoading" class="loading">
              <el-icon class="is-loading"><Loading /></el-icon>
              <p>生成二维码中...</p>
            </div>
            <div v-else class="qrcode">
              <img v-if="qrcodeUrl" 
                   :src="'data:image/png;base64,' + qrcodeUrl" 
                   alt="登录二维码"
                   style="width: 256px; height: 256px; display: block; margin: 0 auto;" 
                   @error="handleImageError"
                   @load="handleImageLoad" />
              <div v-else style="width: 256px; height: 256px; display: flex; align-items: center; justify-content: center; margin: 0 auto; border: 1px dashed #ccc;">
                <span>等待二维码...</span>
              </div>
              <p class="tip">请使用哔哩哔哩APP扫描二维码登录</p>
              <p class="status">{{ loginStatus }}</p>
            </div>
          </div>
        </el-tab-pane>

        <!-- Cookie登录 -->
        <el-tab-pane label="Cookie登录" name="cookie">
          <div class="cookie-container">
            <el-form label-width="0">
              <el-form-item>
                <el-input
                  v-model="cookieInput"
                  type="textarea"
                  :rows="6"
                  placeholder="请粘贴完整的Cookie，格式如：&#10;SESSDATA=xxx; DedeUserID=xxx; DedeUserID__ckMd5=xxx; bili_jct=xxx"
                  clearable
                />
                <div class="cookie-tips">
                  <p>💡 Cookie获取方法：</p>
                  <ol>
                    <li>使用浏览器登录 <a href="https://www.bilibili.com" target="_blank">bilibili.com</a></li>
                    <li>按F12打开开发者工具 → Network（网络）</li>
                    <li>刷新页面，点击任意请求</li>
                    <li>在Request Headers中找到Cookie，复制完整内容</li>
                  </ol>
                  <p class="warning">⚠️ 请勿将Cookie泄露给他人</p>
                </div>
              </el-form-item>
            </el-form>
          </div>
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

const users = ref([])
const loading = ref(false)
const loginDialogVisible = ref(false)
const loginMethod = ref('qrcode')
const qrcodeLoading = ref(false)
const showRateLimitDialog = ref(false)
const showWxPushDialog = ref(false)

// 二维码登录相关
const qrcodeUrl = ref('')
const qrcodeRef = ref(null)
const loginStatus = ref('等待扫码...')
const qrcodeType = ref('tv') // 默认使用TV端
let authKey = ''
let pollingTimer = null

// Cookie登录相关
const cookieInput = ref('')
const cookieLoginLoading = ref(false)

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
  qrcodeUrl.value = ''
  loginStatus.value = '等待扫码...'
  stopPolling()
  // 自动生成二维码
  nextTick(() => {
    generateQRCode()
  })
}

const generateQRCode = async () => {
  qrcodeLoading.value = true
  loginStatus.value = '等待扫码...'
  qrcodeUrl.value = '' // 清空旧的二维码
  
  try {
    // 新的API返回格式: {image: base64, key: sessionKey, type: "web"/"tv"}
    const data = await userAPI.login(qrcodeType.value)
    
    console.log('========== 二维码API响应 ==========')
    console.log('完整响应:', data)
    console.log('是否有error字段:', !!data.error)
    console.log('是否有image字段:', !!data.image)
    console.log('是否有key字段:', !!data.key)
    
    // 检查返回的数据
    if (data.error) {
      console.error('API返回错误:', data.error)
      ElMessage.error(data.error)
      loginStatus.value = data.error
      return
    }
    
    if (!data.image || !data.key) {
      console.error('数据不完整 - image:', !!data.image, 'key:', !!data.key)
      ElMessage.error('二维码数据不完整')
      loginStatus.value = '二维码数据不完整'
      return
    }
    
    authKey = data.key  // 保存session key用于轮询
    qrcodeUrl.value = data.image
    
    console.log('✓ 二维码已设置')
    console.log('✓ Base64长度:', data.image.length)
    console.log('✓ Base64前50字符:', data.image.substring(0, 50))
    console.log('✓ authKey:', authKey)
    console.log('✓ qrcodeUrl响应式值已更新:', qrcodeUrl.value.length)
    
    // 开始轮询登录状态
    startPolling()
  } catch (error) {
    console.error('========== 获取二维码异常 ==========')
    console.error('错误对象:', error)
    console.error('错误消息:', error.message)
    console.error('错误堆栈:', error.stack)
    loginStatus.value = '获取二维码失败: ' + (error.message || '未知错误')
    ElMessage.error('获取二维码失败: ' + (error.message || '未知错误'))
  } finally {
    qrcodeLoading.value = false
    console.log('========== 二维码生成流程结束 ==========')
    console.log('qrcodeUrl是否有值:', !!qrcodeUrl.value)
    console.log('qrcodeLoading:', qrcodeLoading.value)
  }
}

const handleCookieLogin = async () => {
  const cookies = cookieInput.value.trim()
  if (!cookies) {
    ElMessage.warning('请输入Cookie')
    return
  }

  cookieLoginLoading.value = true
  try {
    const result = await userAPI.loginByCookie(cookies)
    if (result.type === 'success') {
      ElMessage.success('登录成功')
      loginDialogVisible.value = false
      cookieInput.value = ''
      fetchUsers()
    } else {
      ElMessage.error(result.msg || '登录失败')
    }
  } catch (error) {
    console.error('Cookie登录失败:', error)
    ElMessage.error('登录失败，请检查Cookie是否正确')
  } finally {
    cookieLoginLoading.value = false
  }
}

const startPolling = () => {
  stopPolling()
  
  pollingTimer = setInterval(async () => {
    try {
      // 使用新的loginCheck API
      const data = await userAPI.loginCheck(authKey)
      
      loginStatus.value = data.message || '检查中...'
      
      if (data.status === 'success') {
        loginStatus.value = '登录成功！'
        ElMessage.success('登录成功')
        stopPolling()
        loginDialogVisible.value = false
        fetchUsers()
      } else if (data.status === 'expired') {
        loginStatus.value = '二维码已过期，请重新获取'
        stopPolling()
      } else if (data.status === 'scanned') {
        loginStatus.value = '已扫码，请在手机上确认'
      } else if (data.status === 'failed') {
        loginStatus.value = data.message || '登录失败'
        stopPolling()
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

const handleQRTypeChange = () => {
  // 切换登录方式时重新生成二维码
  if (loginDialogVisible.value) {
    stopPolling()
    generateQRCode()
  }
}

const cancelLogin = () => {
  stopPolling()
  loginDialogVisible.value = false
  cookieInput.value = ''
  qrcodeUrl.value = ''
}

const handleImageError = (e) => {
  console.error('二维码图片加载失败:', e)
  loginStatus.value = '二维码图片加载失败，请重新生成'
  ElMessage.error('二维码图片加载失败')
}

const handleImageLoad = () => {
  console.log('二维码图片加载成功')
}

const handleCheckStatus = async (row) => {
  row.checking = true
  try {
    const result = await userAPI.checkStatus(row.id)
    if (result.type === 'success') {
      ElMessage.success(result.msg || 'Cookie有效，用户状态正常')
      // 更新用户信息
      if (result.user) {
        Object.assign(row, result.user)
      }
    } else {
      ElMessage.error(result.msg || 'Cookie已失效')
      // 更新用户登录状态
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

// 处理二维码类型切换
const handleQRTypeChange = () => {
  // 切换登录方式时重新生成二维码
  if (loginDialogVisible.value) {
    stopPolling()
    generateQRCode()
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

.login-tabs {
  margin-top: -10px;
}

.cookie-container {
  padding: 10px 0;
}

.cookie-tips {
  margin-top: 15px;
  padding: 15px;
  background-color: #f5f7fa;
  border-radius: 4px;
  font-size: 13px;
  color: #666;
  line-height: 1.8;
}

.cookie-tips p {
  margin: 8px 0;
}

.cookie-tips ol {
  margin: 10px 0;
  padding-left: 20px;
}

.cookie-tips ol li {
  margin: 5px 0;
}

.cookie-tips a {
  color: #1890ff;
  text-decoration: none;
}

.cookie-tips a:hover {
  text-decoration: underline;
}

.cookie-tips .warning {
  color: #ff4d4f;
  font-weight: bold;
  margin-top: 10px;
}

.login-type-selector {
  text-align: center;
  margin-bottom: 16px;
}

.login-type-selector .el-radio-group {
  margin-bottom: 8px;
}

.empty {
  text-align: center;
  padding: 40px 0;
}
</style>
