<template>
  <el-dialog
    :model-value="visible"
    title="添加B站用户"
    width="500px"
    :close-on-click-modal="false"
    @update:model-value="$emit('update:visible', $event)"
  >
    <div class="qrcode-container-vertical">
      <!-- 登录方式选择 -->
      <div class="login-type-selector">
        <el-radio-group v-model="localQrcodeType" @change="handleTypeChange" size="default">
          <el-radio-button label="tv">TV端扫码</el-radio-button>
          <el-radio-button label="web">Web端扫码</el-radio-button>
        </el-radio-group>
        <div class="type-description">
          <template v-if="localQrcodeType === 'tv'">
            <el-icon><Star /></el-icon>
            <span>推荐：稳定性更好，适合长期使用</span>
          </template>
          <template v-else>
            <el-icon><InfoFilled /></el-icon>
            <span>兼容性更好，与网页端登录一致</span>
          </template>
        </div>
      </div>
      
      <!-- 二维码显示区域 -->
      <div class="qrcode-display-area">
        <div v-if="qrcodeLoading" class="qrcode-loading">
          <el-icon class="is-loading" :size="40"><Loading /></el-icon>
          <p>生成二维码中...</p>
        </div>
        <div v-else class="qrcode-wrapper">
          <div class="qrcode-image">
            <img v-if="qrcodeUrl" 
                 :src="'data:image/png;base64,' + qrcodeUrl" 
                 alt="登录二维码"
                 @error="handleImageError"
                 @load="handleImageLoad" />
            <div v-else class="qrcode-placeholder">
              <el-icon :size="60"><Picture /></el-icon>
              <span>等待二维码...</span>
            </div>
          </div>
          <div class="qrcode-info">
            <p class="scan-tip">
              <el-icon><Iphone /></el-icon>
              请使用哔哩哔哩APP扫描二维码登录
            </p>
            <el-divider />
            <p class="login-status" :class="getStatusClass()">
              <el-icon v-if="loginStatus.includes('成功')"><CircleCheck /></el-icon>
              <el-icon v-else-if="loginStatus.includes('失败') || loginStatus.includes('过期')"><CircleClose /></el-icon>
              <el-icon v-else-if="loginStatus.includes('已扫码')"><Loading class="is-loading" /></el-icon>
              <el-icon v-else><Clock /></el-icon>
              <span>{{ loginStatus }}</span>
            </p>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="$emit('cancel')">取消</el-button>
      <el-button 
        v-if="qrcodeUrl" 
        type="primary" 
        @click="$emit('regenerate')"
      >
        重新生成
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { 
  Loading, 
  Star, 
  InfoFilled, 
  Picture, 
  Iphone, 
  CircleCheck, 
  CircleClose, 
  Clock 
} from '@element-plus/icons-vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  qrcodeUrl: {
    type: String,
    default: ''
  },
  qrcodeLoading: {
    type: Boolean,
    default: false
  },
  loginStatus: {
    type: String,
    default: '等待扫码...'
  },
  qrcodeType: {
    type: String,
    default: 'tv'
  }
})

const emit = defineEmits(['update:visible', 'cancel', 'regenerate', 'type-change'])

const localQrcodeType = ref(props.qrcodeType)

watch(() => props.qrcodeType, (val) => {
  localQrcodeType.value = val
})

const handleTypeChange = () => {
  emit('type-change', localQrcodeType.value)
}

const handleImageError = (e) => {
  console.error('二维码图片加载失败:', e)
  ElMessage.error('二维码图片加载失败')
}

const handleImageLoad = () => {
  console.log('二维码图片加载成功')
}

const getStatusClass = () => {
  const status = props.loginStatus.toLowerCase()
  if (status.includes('成功')) return 'status-success'
  if (status.includes('失败') || status.includes('过期')) return 'status-error'
  if (status.includes('已扫码') || status.includes('确认')) return 'status-scanned'
  return 'status-waiting'
}
</script>

<style scoped>
.qrcode-container-vertical {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 20px 10px;
}

.login-type-selector {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.type-description {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #909399;
  padding: 8px 16px;
  background-color: #f4f4f5;
  border-radius: 4px;
}

.type-description .el-icon {
  font-size: 16px;
}

.qrcode-display-area {
  display: flex;
  justify-content: center;
  min-height: 350px;
}

.qrcode-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #909399;
}

.qrcode-loading p {
  font-size: 14px;
  margin: 0;
}

.qrcode-wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20px;
  width: 100%;
  max-width: 400px;
}

.qrcode-image {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 20px;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
}

.qrcode-image img {
  width: 256px;
  height: 256px;
  display: block;
  border-radius: 4px;
}

.qrcode-placeholder {
  width: 256px;
  height: 256px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  border: 2px dashed #dcdfe6;
  border-radius: 4px;
  color: #909399;
  background-color: #fafafa;
}

.qrcode-placeholder .el-icon {
  color: #c0c4cc;
}

.qrcode-info {
  width: 100%;
  text-align: center;
}

.scan-tip {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin: 0 0 12px 0;
  font-size: 14px;
  color: #606266;
}

.scan-tip .el-icon {
  font-size: 18px;
  color: #409eff;
}

.el-divider {
  margin: 12px 0;
}

.login-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin: 12px 0 0 0;
  font-size: 14px;
  font-weight: 500;
  padding: 8px 16px;
  border-radius: 4px;
  background-color: #f4f4f5;
}

.login-status .el-icon {
  font-size: 18px;
}

.login-status.status-waiting {
  color: #909399;
  background-color: #f4f4f5;
}

.login-status.status-scanned {
  color: #409eff;
  background-color: #ecf5ff;
}

.login-status.status-success {
  color: #67c23a;
  background-color: #f0f9ff;
}

.login-status.status-error {
  color: #f56c6c;
  background-color: #fef0f0;
}
</style>
