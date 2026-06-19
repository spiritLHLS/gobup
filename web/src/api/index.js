import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const request = axios.create({
  baseURL: '/api',
  timeout: 30000
})

const getResponseMessage = (data, fallback = '请求失败') => {
  if (!data) return fallback
  if (typeof data === 'string') return data
  return data.msg || data.message || data.error || fallback
}

const normalizeBusinessPayload = (data) => {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return data

  if (data.message && !data.msg) {
    data.msg = data.message
  }
  if (data.error && !data.msg) {
    data.msg = data.error
  }
  if (typeof data.code !== 'undefined' && typeof data.ok === 'undefined') {
    data.ok = Number(data.code) === 0
  }
  if (data.type && typeof data.ok === 'undefined') {
    data.ok = data.type === 'success'
  }
  return data
}

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
    return normalizeBusinessPayload(response.data)
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
    
    ElMessage.error(getResponseMessage(error.response?.data, error.message || '请求失败'))
    return Promise.reject(error)
  }
)

// 房间管理
export const roomAPI = {
  list: () => request.post('/room'),
  update: (data) => request.post('/room/update', data),
  delete: (id) => request.get(`/room/delete/${id}`),
  getLines: () => request.get('/room/lines'),
  testLines: () => request.get('/room/testLines', { timeout: 60000 }),
  testSpeed: (line) => request.get('/room/testSpeed', { params: { line }, timeout: 30000 }),
  verifyTemplate: (data) => request.post('/room/verifyTemplate', data),
  getSeasons: (userId) => request.get('/room/seasons', { params: { userId } }),
  createSeason: (data) => request.post('/room/seasons', data)
}

// 录制历史
export const historyAPI = {
  list: (params) => request.post('/history/list', params),
  export: (params) => request.post('/history/export', params, { responseType: 'blob' }),
  publish: (id) => request.post(`/history/publish/${id}`),
  delete: (id) => request.get(`/history/delete/${id}`),
  deleteWithFiles: (id) => request.post(`/history/deleteWithFiles/${id}`, {
    confirmDeleteFiles: true,
    confirmText: 'DELETE_FILES'
  }),
  parts: (id) => request.get(`/history/part/${id}`),
  upload: (id) => request.post(`/history/upload/${id}`),
  syncVideo: (id) => request.post(`/history/syncVideo/${id}`),
  syncSessions: (historyIds = []) => request.post('/history/sync', { historyIds }),
  moveFiles: (id) => request.post(`/history/moveFiles/${id}`),
  resetStatus: (id, options) => request.post(`/history/resetStatus/${id}`, options),
  forceArchive: (id) => request.post(`/history/forceArchive/${id}`),
  candidateFiles: (id) => request.get(`/history/candidateFiles/${id}`)
}

// 队列管理
export const queueAPI = {
  taskStatus: () => request.get('/queue/tasks/status'),
  uploadStatus: () => request.get('/queue/upload/status'),
  pauseUploadPart: (id) => request.post(`/queue/upload/part/${id}/pause`),
  resumeUploadPart: (id) => request.post(`/queue/upload/part/${id}/resume`),
  cancelUploadPart: (id) => request.post(`/queue/upload/part/${id}/cancel`),
  retryUploadPart: (id) => request.post(`/queue/upload/part/${id}/retry`),
  pauseAllUploads: () => request.post('/queue/upload/pauseAll'),
  resumeAllUploads: () => request.post('/queue/upload/resumeAll'),
  cancelAllUploads: () => request.post('/queue/upload/cancelAll'),
  danmakuStatus: () => request.get('/queue/danmaku/status'),
  parseStatus: () => request.get('/queue/parse/status')
}

// 用户管理
export const userAPI = {
  list: () => request.get('/biliUser/list'),
  login: (type = 'tv') => request.get('/biliUser/login', { params: { type } }),
  loginCheck: (key) => request.get('/biliUser/loginCheck', { params: { key } }),
  loginCancel: (key) => request.get('/biliUser/loginCancel', { params: { key } }),
  loginByCookie: (cookies) => request.post('/biliUser/loginByCookie', { cookies }),
  update: (data) => request.post('/biliUser/update', data),
  refresh: (id) => request.get(`/biliUser/refresh/${id}`),
  checkStatus: (id) => request.get(`/biliUser/checkStatus/${id}`),
  delete: (id) => request.get(`/biliUser/delete/${id}`),
  setEnabled: (id, enabled) => request.post(`/biliUser/enabled/${id}`, { enabled }),
  export: (data) => request.post('/biliUser/export', data, { responseType: 'blob' }),
  import: (file) => {
    const formData = new FormData()
    formData.append('file', file)
    return request.post('/biliUser/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  }
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

// 文件扫描
export const filescanAPI = {
  trigger: (force = false) => request.post('/filescan/trigger', null, { params: { force } }),
  preview: () => request.get('/filescan/preview'),
  import: (filePaths) => request.post('/filescan/import', { filePaths }),
  cleanPreview: () => request.get('/filescan/cleanPreview'),
  cleanSelected: (filePaths) => request.post('/filescan/cleanSelected', {
    filePaths,
    confirmDeleteFiles: true,
    confirmText: 'DELETE_FILES'
  }),
  cleanCompleted: () => request.post('/filescan/cleanCompleted', {
    confirmDeleteFiles: true,
    confirmText: 'DELETE_FILES'
  })
}

// 数据修复
export const dataRepairAPI = {
  check: (dryRun = true) => request.get('/datarepair/check', { params: { dryRun } }),
  repair: () => request.post('/datarepair/repair')
}

export default request
