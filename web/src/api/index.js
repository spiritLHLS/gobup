import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const request = axios.create({
  baseURL: '/api',
  timeout: 30000
})

// 请求拦截器
request.interceptors.request.use(
  config => {
    // 添加 Basic Auth
    const username = localStorage.getItem('username')
    const password = localStorage.getItem('password')
    if (username && password) {
      config.headers.Authorization = 'Basic ' + btoa(username + ':' + password)
    }
    return config
  },
  error => {
    return Promise.reject(error)
  }
)

// 响应拦截器
request.interceptors.response.use(
  response => {
    return response.data
  },
  error => {
    // 处理 401 未授权错误
    if (error.response?.status === 401) {
      ElMessage.error('认证失败，请重新登录')
      localStorage.removeItem('username')
      localStorage.removeItem('password')
      router.push('/login')
      return Promise.reject(error)
    }
    
    ElMessage.error(error.response?.data?.message || error.message || '请求失败')
    return Promise.reject(error)
  }
)

// 房间管理
export const roomAPI = {
  list: () => request.post('/room'),
  update: (data) => request.post('/room/update', data),
  delete: (id) => request.get(`/room/delete/${id}`),
  getLines: () => request.get('/room/lines'),
  testLines: () => request.get('/room/testLines'),
  testSpeed: (line) => request.get('/room/testSpeed', { params: { line } }),
  verifyTemplate: (data) => request.post('/room/verifyTemplate', data)
}

// 录制历史
export const historyAPI = {
  list: (params) => request.post('/history/list', null, { params }),
  publish: (id) => request.post(`/history/publish/${id}`),
  delete: (id) => request.get(`/history/delete/${id}`),
  parts: (id) => request.get(`/history/part/${id}`)
}

// 用户管理
export const userAPI = {
  list: () => request.get('/biliUser/list'),
  login: () => request.get('/biliUser/login'),
  loginReturn: (key) => request.get('/biliUser/loginReturn', { params: { key } }),
  refresh: (id) => request.get(`/biliUser/refresh/${id}`),
  delete: (id) => request.get(`/biliUser/delete/${id}`)
}

// 配置管理
export const configAPI = {
  export: (data) => request.post('/config/export', data, { responseType: 'blob' }),
  import: (file) => {
    const formData = new FormData()
    formData.append('file', file)
    return request.post('/config/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  }
}

export default request
