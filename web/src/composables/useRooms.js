import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { roomAPI } from '@/api'

export function useLineTest() {
  const lineStats = ref({})
  const lineSpeeds = ref({})
  const testingLines = ref(false)
  const testingDeepSpeed = ref(false)

  const testLines = async () => {
    try {
      await ElMessageBox.confirm(
        '线路检测将分批测试30+条线路，为避免触发风控，测试过程约需30-45秒，是否继续？',
        '提示',
        {
          confirmButtonText: '开始检测',
          cancelButtonText: '取消',
          type: 'info'
        }
      )
    } catch {
      return
    }

    testingLines.value = true
    lineStats.value = {}
    lineSpeeds.value = {}
    
    ElMessage.info('开始检测线路，请耐心等待约30-45秒...')
    
    try {
      const data = await roomAPI.testLines()
      lineStats.value = data || {}
      ElMessage.success('线路检测完成')
    } catch (error) {
      console.error('线路检测失败:', error)
      ElMessage.error('线路检测失败')
    } finally {
      testingLines.value = false
    }
  }

  const testDeepSpeed = async () => {
    if (Object.keys(lineStats.value).length === 0) {
      ElMessage.warning('请先进行普通线路检测')
      return
    }
    
    const availableLines = Object.keys(lineStats.value).filter(line => {
      const status = lineStats.value[line]
      return status && status.includes('ms')
    })
    
    if (availableLines.length === 0) {
      ElMessage.warning('没有可用的线路进行深度测速')
      return
    }

    try {
      await ElMessageBox.confirm(
        `将对${availableLines.length}条可用线路进行真实上传测速（1MB数据），为避免风控每条线路间隔2秒，预计耗时${Math.ceil(availableLines.length * 2 / 60)}分钟，是否继续？`,
        '深度测速确认',
        {
          confirmButtonText: '开始测速',
          cancelButtonText: '取消',
          type: 'warning'
        }
      )
    } catch {
      return
    }
    
    testingDeepSpeed.value = true
    lineSpeeds.value = {}
    
    ElMessage.info(`开始深度测速，将测试${availableLines.length}条线路，请耐心等待...`)
    
    for (let i = 0; i < availableLines.length; i++) {
      const line = availableLines[i]
      lineSpeeds.value[line] = `测速中(${i+1}/${availableLines.length})...`
      
      try {
        const result = await roomAPI.testSpeed(line)
        if (result.success) {
          lineSpeeds.value[line] = result.speed
        } else {
          lineSpeeds.value[line] = '失败'
        }
      } catch (error) {
        lineSpeeds.value[line] = '失败'
      }

      if (i < availableLines.length - 1) {
        await new Promise(resolve => setTimeout(resolve, 2000))
      }
    }
    
    testingDeepSpeed.value = false
    ElMessage.success('深度测速完成')
  }

  return {
    lineStats,
    lineSpeeds,
    testingLines,
    testingDeepSpeed,
    testLines,
    testDeepSpeed
  }
}

export function formatLine(line) {
  if (!line) return '-'
  
  const lineMap = {
    'upos': '华北',
    'CS_UPOS': '华北',
    'kodo': '七牛云',
    'app': '移动端',
    'bda2': '华东',
    'qn': '华东',
    'ws': '华南',
    'bda': '东南亚',
    'HW_UPOS': '华为云',
    'TX_UPOS': '腾讯云'
  }
  
  for (const key in lineMap) {
    if (line && line.toLowerCase().includes(key.toLowerCase())) {
      const region = lineMap[key]
      return region === line ? line : `${line} (${region})`
    }
  }
  
  return line
}
