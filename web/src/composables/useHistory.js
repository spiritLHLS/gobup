import { ref, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox, ElLoading } from 'element-plus'
import axios from 'axios'

export function useHistoryProgress() {
  const uploadProgress = ref(null)
  const progressTimer = ref(null)
  const speedTracking = ref({})
  const historyProgressMap = ref({})
  const historyProgressTimer = ref(null)
  const danmakuProgressMap = ref({})
  const danmakuProgressTimer = ref(null)

  // 开始轮询上传进度
  const startProgressPolling = async (historyId) => {
    stopProgressPolling()
    
    await fetchProgress(historyId)
    
    if (uploadProgress.value && uploadProgress.value.activeCount > 0) {
      progressTimer.value = setInterval(() => {
        fetchProgress(historyId)
      }, 1500)
    }
  }

  // 停止轮询
  const stopProgressPolling = () => {
    if (progressTimer.value) {
      clearInterval(progressTimer.value)
      progressTimer.value = null
    }
    speedTracking.value = {}
  }

  // 获取进度
  const fetchProgress = async (historyId) => {
    try {
      const response = await axios.get(`/api/progress/history/${historyId}`)
      uploadProgress.value = response.data
      
      updateSpeedTracking(uploadProgress.value)
      
      if (!uploadProgress.value || uploadProgress.value.activeCount === 0) {
        stopProgressPolling()
      }
    } catch (error) {
      console.error('获取进度失败:', error)
    }
  }

  // 更新速度追踪
  const updateSpeedTracking = (progress) => {
    if (!progress || !progress.items) return
    
    const now = Date.now()
    progress.items.forEach(item => {
      if (item.state !== 'UPLOADING' || !item.chunkTotal) return
      
      const partId = item.partId
      if (!speedTracking.value[partId]) {
        speedTracking.value[partId] = {
          samples: [],
          lastChunkDone: item.chunkDone,
          lastTime: now,
          chunkTotal: item.chunkTotal
        }
      } else {
        const track = speedTracking.value[partId]
        const timeDiff = (now - track.lastTime) / 1000
        const chunkDiff = item.chunkDone - track.lastChunkDone
        
        if (timeDiff > 0 && chunkDiff > 0) {
          const speed = chunkDiff / timeDiff
          track.samples.push({ speed, time: now })
          
          if (track.samples.length > 10) {
            track.samples.shift()
          }
          
          track.lastChunkDone = item.chunkDone
          track.lastTime = now
          track.chunkTotal = item.chunkTotal
        }
      }
    })
  }

  // 开始历史记录进度轮询
  const startHistoryProgressPolling = () => {
    if (historyProgressTimer.value) return
    
    historyProgressTimer.value = setInterval(() => {
      // 这个函数需要从外部传入历史记录列表
    }, 2000)
  }

  // 停止历史记录进度轮询
  const stopHistoryProgressPolling = () => {
    if (historyProgressTimer.value) {
      clearInterval(historyProgressTimer.value)
      historyProgressTimer.value = null
    }
  }

  // 获取所有上传中的历史记录进度
  const fetchHistoryProgress = async (histories) => {
    const uploadingHistories = histories.filter(h => h.uploadStatus === 1 && !h.bvId)
    if (uploadingHistories.length === 0) {
      stopHistoryProgressPolling()
      return
    }
    
    for (const history of uploadingHistories) {
      try {
        const response = await axios.get(`/api/progress/history/${history.id}`)
        if (response.data) {
          historyProgressMap.value[history.id] = response.data
        }
      } catch (error) {
        console.error(`获取历史记录${history.id}进度失败:`, error)
      }
    }
  }

  // 获取历史记录的进度信息
  const getHistoryProgress = (historyId) => {
    return historyProgressMap.value[historyId]
  }

  // 获取历史记录的整体上传进度百分比
  const getHistoryUploadPercent = (historyId) => {
    const progress = getHistoryProgress(historyId)
    if (!progress) return 0
    return progress.overallPercent || 0
  }

  // 开始弹幕进度轮询
  const startDanmakuProgressPolling = () => {
    if (danmakuProgressTimer.value) return
    
    danmakuProgressTimer.value = setInterval(() => {
      // 这个函数需要从外部传入历史记录列表
    }, 1000)
  }

  // 停止弹幕进度轮询
  const stopDanmakuProgressPolling = () => {
    if (danmakuProgressTimer.value) {
      clearInterval(danmakuProgressTimer.value)
      danmakuProgressTimer.value = null
    }
  }

  // 获取弹幕发送进度
  const fetchDanmakuProgress = async (histories) => {
    // 找出所有有弹幕进度的历史记录
    const historyIds = Object.keys(danmakuProgressMap.value).map(id => parseInt(id))
    
    if (historyIds.length === 0) {
      stopDanmakuProgressPolling()
      return
    }
    
    let hasActiveProgress = false
    
    for (const historyId of historyIds) {
      try {
        const response = await axios.get(`/api/progress/danmaku/${historyId}`)
        if (response.data) {
          danmakuProgressMap.value[historyId] = response.data
          
          // 如果有正在发送的弹幕，保持轮询
          if (response.data.sending && !response.data.completed) {
            hasActiveProgress = true
          }
        }
      } catch (error) {
        console.error(`获取弹幕进度${historyId}失败:`, error)
      }
    }
    
    if (!hasActiveProgress) {
      stopDanmakuProgressPolling()
    }
  }

  // 获取弹幕进度信息
  const getDanmakuProgress = (historyId) => {
    return danmakuProgressMap.value[historyId]
  }

  // 获取弹幕进度百分比
  const getDanmakuProgressPercent = (historyId) => {
    const progress = getDanmakuProgress(historyId)
    if (!progress || !progress.total) return 0
    return Math.round((progress.current / progress.total) * 100)
  }

  // 标记弹幕发送开始
  const markDanmakuSendingStart = (historyId, total) => {
    danmakuProgressMap.value[historyId] = {
      sending: true,
      completed: false,
      current: 0,
      total: total
    }
    startDanmakuProgressPolling()
  }

  // 清除弹幕进度
  const clearDanmakuProgress = (historyId) => {
    delete danmakuProgressMap.value[historyId]
  }

  onUnmounted(() => {
    stopProgressPolling()
    stopHistoryProgressPolling()
    stopDanmakuProgressPolling()
  })

  return {
    uploadProgress,
    speedTracking,
    historyProgressMap,
    danmakuProgressMap,
    startProgressPolling,
    stopProgressPolling,
    fetchProgress,
    startHistoryProgressPolling,
    stopHistoryProgressPolling,
    fetchHistoryProgress,
    getHistoryProgress,
    getHistoryUploadPercent,
    startDanmakuProgressPolling,
    stopDanmakuProgressPolling,
    fetchDanmakuProgress,
    getDanmakuProgress,
    getDanmakuProgressPercent,
    markDanmakuSendingStart,
    clearDanmakuProgress
  }
}

export function useHistoryOperations() {
  // 弹幕进度追踪
  const danmakuProgressMap = ref({})
  
  // 上传视频
  const handleUpload = async (row, callback) => {
    try {
      await ElMessageBox.confirm('确定要开始上传视频到B站吗？', '上传确认', {
        type: 'warning'
      })
      
      const userResponse = await axios.get('/api/biliUser/list')
      const users = userResponse.data || []
      
      if (users.length === 0) {
        ElMessage.warning('请先添加B站用户')
        return
      }
      
      const userId = users[0].id
      
      const loadingInstance = ElLoading.service({ text: '上传任务已启动，请稍候...' })
      try {
        const response = await axios.post(`/api/history/upload/${row.id}`, { userId })
        ElMessage.success(response.data.msg || '上传任务已启动')
        callback?.()
      } finally {
        loadingInstance.close()
      }
    } catch (error) {
      if (error !== 'cancel') {
        console.error('上传失败:', error)
        ElMessage.error(error.response?.data?.msg || '上传失败')
      }
    }
  }

  // 投稿视频
  const handlePublish = async (row, callback) => {
    try {
      await ElMessageBox.confirm('确定要投稿这个视频到B站吗？', '投稿确认', {
        type: 'warning'
      })
      
      const userResponse = await axios.get('/api/biliUser/list')
      const users = userResponse.data || []
      
      if (users.length === 0) {
        ElMessage.warning('请先添加B站用户')
        return
      }
      
      const userId = users[0].id
      
      const loadingInstance = ElLoading.service({ text: '投稿中，请稍候...' })
      try {
        await axios.post(`/api/history/publish/${row.id}`, { userId })
        ElMessage.success('投稿任务已提交')
        callback?.()
      } finally {
        loadingInstance.close()
      }
    } catch (error) {
      if (error !== 'cancel') {
        console.error('投稿失败:', error)
        ElMessage.error(error.response?.data?.msg || '投稿失败')
      }
    }
  }

  // 发送弹幕
  const handleSendDanmaku = async (row, callback) => {
    try {
      await ElMessageBox.confirm('确定要将直播弹幕转移到视频吗？此操作可能需要较长时间。', '发送弹幕', {
        type: 'warning'
      })
      
      const userResponse = await axios.get('/api/biliUser/list')
      const users = userResponse.data || []
      
      if (users.length === 0) {
        ElMessage.warning('请先添加B站用户')
        return
      }
      
      const userId = users[0].id
      
      ElMessage.info('弹幕发送任务已启动，请查看进度...')
      
      // 异步发送，不等待结果
      axios.post(`/api/history/sendDanmaku/${row.id}`, { userId })
        .then(() => {
          ElMessage.success('弹幕发送完成')
          callback?.()
        })
        .catch((error) => {
          console.error('发送弹幕失败:', error)
          ElMessage.error(error.response?.data?.msg || '发送弹幕失败')
          callback?.()
        })
      
      // 立即回调以刷新状态
      if (callback) {
        setTimeout(callback, 500)
      }
    } catch (error) {
      if (error !== 'cancel') {
        console.error('发送弹幕失败:', error)
        ElMessage.error(error.response?.data?.msg || '发送弹幕失败')
      }
    }
  }

  // 同步视频信息
  const handleSyncVideo = async (row, callback) => {
    try {
      const loadingInstance = ElLoading.service({ text: '同步中...' })
      try {
        await axios.post(`/api/history/syncVideo/${row.id}`)
        ElMessage.success('视频信息同步成功')
        callback?.()
      } finally {
        loadingInstance.close()
      }
    } catch (error) {
      console.error('同步视频信息失败:', error)
      ElMessage.error(error.response?.data?.msg || '同步失败')
    }
  }

  // 移动文件
  const handleMoveFiles = async (row, callback) => {
    try {
      await ElMessageBox.confirm('确定要移动此历史记录的所有相关文件吗？', '移动文件', {
        type: 'warning'
      })
      
      const loadingInstance = ElLoading.service({ text: '文件移动中...' })
      try {
        await axios.post(`/api/history/moveFiles/${row.id}`)
        ElMessage.success('文件移动成功')
        callback?.()
      } finally {
        loadingInstance.close()
      }
    } catch (error) {
      if (error !== 'cancel') {
        console.error('移动文件失败:', error)
        ElMessage.error(error.response?.data?.msg || '移动失败')
      }
    }
  }

  // 重置状态
  const handleResetStatus = async (historyId, options, callback) => {
    try {
      const loadingInstance = ElLoading.service({ text: '重置中...' })
      try {
        await axios.post(`/api/history/resetStatus/${historyId}`, options)
        ElMessage.success('状态已重置')
        callback?.()
      } finally {
        loadingInstance.close()
      }
    } catch (error) {
      console.error('重置失败:', error)
      ElMessage.error(error.response?.data?.msg || '重置失败')
    }
  }

  // 仅删除记录
  const handleDeleteOnly = async (historyId, callback) => {
    try {
      await ElMessageBox.confirm(
        '此操作将仅删除数据库记录，不会删除文件。确定要删除吗？',
        '删除记录',
        { type: 'warning' }
      )
      
      await axios.get(`/api/history/delete/${historyId}`)
      ElMessage.success('记录已删除')
      callback?.()
    } catch (error) {
      if (error !== 'cancel') {
        console.error('删除失败:', error)
        ElMessage.error(error.response?.data?.msg || '删除失败')
      }
    }
  }

  // 删除记录和文件
  const handleDeleteWithFiles = async (historyId, callback) => {
    try {
      await ElMessageBox.confirm(
        '此操作将删除数据库记录和所有相关文件，不可恢复。确定要删除吗？',
        '删除记录和文件',
        { type: 'error', confirmButtonText: '确定删除' }
      )
      
      const loadingInstance = ElLoading.service({ text: '删除中...' })
      try {
        await axios.post(`/api/history/deleteWithFiles/${historyId}`)
        ElMessage.success('记录和文件已删除')
        callback?.()
      } finally {
        loadingInstance.close()
      }
    } catch (error) {
      if (error !== 'cancel') {
        console.error('删除失败:', error)
        ElMessage.error(error.response?.data?.msg || '删除失败')
      }
    }
  }

  return {
    handleUpload,
    handlePublish,
    handleSendDanmaku,
    handleSyncVideo,
    handleMoveFiles,
    handleResetStatus,
    handleDeleteOnly,
    handleDeleteWithFiles
  }
}
