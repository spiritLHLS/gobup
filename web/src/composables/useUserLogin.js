import { ref, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { userAPI } from '@/api'

export function useQrcodeLogin() {
  const qrcodeUrl = ref('')
  const qrcodeLoading = ref(false)
  const loginStatus = ref('等待扫码...')
  const qrcodeType = ref('tv')
  let authKey = ''
  let pollingTimer = null

  const generateQRCode = async () => {
    qrcodeLoading.value = true
    loginStatus.value = '等待扫码...'
    qrcodeUrl.value = ''
    
    try {
      const data = await userAPI.login(qrcodeType.value)
      
      if (data.error) {
        ElMessage.error(data.error)
        loginStatus.value = data.error
        return
      }
      
      if (!data.image || !data.key) {
        ElMessage.error('二维码数据不完整')
        loginStatus.value = '二维码数据不完整'
        return
      }
      
      authKey = data.key
      qrcodeUrl.value = data.image
      
      startPolling()
    } catch (error) {
      console.error('获取二维码失败:', error)
      loginStatus.value = '获取二维码失败: ' + (error.message || '未知错误')
      ElMessage.error('获取二维码失败: ' + (error.message || '未知错误'))
    } finally {
      qrcodeLoading.value = false
    }
  }

  const startPolling = () => {
    stopPolling()
    
    pollingTimer = setInterval(async () => {
      try {
        const data = await userAPI.loginCheck(authKey)
        
        loginStatus.value = data.message || '检查中...'
        
        if (data.status === 'success') {
          loginStatus.value = '登录成功！'
          ElMessage.success('登录成功')
          stopPolling()
          return { success: true }
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

  const cleanup = () => {
    stopPolling()
    qrcodeUrl.value = ''
    loginStatus.value = '等待扫码...'
  }

  onUnmounted(() => {
    stopPolling()
  })

  return {
    qrcodeUrl,
    qrcodeLoading,
    loginStatus,
    qrcodeType,
    generateQRCode,
    stopPolling,
    cleanup
  }
}

export function useCookieLogin() {
  const cookieInput = ref('')
  const cookieLoginLoading = ref(false)

  const handleLogin = async () => {
    const cookies = cookieInput.value.trim()
    if (!cookies) {
      ElMessage.warning('请输入Cookie')
      return { success: false }
    }

    cookieLoginLoading.value = true
    try {
      const result = await userAPI.loginByCookie(cookies)
      if (result.type === 'success') {
        ElMessage.success('登录成功')
        cookieInput.value = ''
        return { success: true }
      } else {
        ElMessage.error(result.msg || '登录失败')
        return { success: false }
      }
    } catch (error) {
      console.error('Cookie登录失败:', error)
      ElMessage.error('登录失败，请检查Cookie是否正确')
      return { success: false }
    } finally {
      cookieLoginLoading.value = false
    }
  }

  return {
    cookieInput,
    cookieLoginLoading,
    handleLogin
  }
}
