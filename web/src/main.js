import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus, { ElMessage } from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import axios from 'axios'
import App from './App.vue'
import router from './router'

const normalizeApiPayload = (data) => {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return data
  if (data.message && !data.msg) data.msg = data.message
  if (data.error && !data.msg) data.msg = data.error
  if (typeof data.code !== 'undefined' && typeof data.ok === 'undefined') {
    data.ok = Number(data.code) === 0
  }
  if (data.type && typeof data.ok === 'undefined') {
    data.ok = data.type === 'success'
  }
  return data
}

axios.interceptors.request.use((config) => {
  const username = localStorage.getItem('username')
  const password = localStorage.getItem('password')
  const isApiRequest = typeof config.url === 'string' && config.url.startsWith('/api')
  if (isApiRequest && username && password && !config.headers?.Authorization) {
    config.headers = config.headers || {}
    config.headers.Authorization = 'Basic ' + btoa(username + ':' + password)
  }
  return config
})

axios.interceptors.response.use(
  (response) => {
    response.data = normalizeApiPayload(response.data)
    return response
  },
  (error) => {
    if (error.response?.data) {
      error.response.data = normalizeApiPayload(error.response.data)
    }
    if (error.response?.status === 401) {
      ElMessage.error('认证失败，请重新登录')
      localStorage.removeItem('username')
      localStorage.removeItem('password')
      router.push('/login')
    }
    return Promise.reject(error)
  }
)

// 提前初始化主题，防止白屏闪烁
const savedTheme = localStorage.getItem('theme')
if (savedTheme === 'dark') {
  document.documentElement.setAttribute('data-theme', 'dark')
}

const app = createApp(App)

// 注册所有Element Plus图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { size: 'default' })

app.mount('#app')
