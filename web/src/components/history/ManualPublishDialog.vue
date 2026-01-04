<template>
  <el-dialog 
    :model-value="visible"
    title="手动标记投稿" 
    width="650px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <el-alert
      title="说明"
      type="info"
      :closable="false"
      style="margin-bottom: 20px"
    >
      <p>当系统无法自动判定投稿状态时，可以手动填写B站视频的BV号来标记为已投稿。</p>
      <p style="margin-top: 8px;">请确保视频确实已在B站投稿成功。</p>
    </el-alert>

    <el-form 
      ref="formRef"
      :model="form" 
      :rules="rules"
      label-width="100px"
    >
      <el-form-item label="BV号" prop="bvId">
        <el-input 
          v-model="form.bvId" 
          placeholder="请输入BV号，例如：BV16CiBBPE1b"
          clearable
        >
          <template #prepend>
            <el-icon><Link /></el-icon>
          </template>
        </el-input>
        <div class="form-tip">
          12位字符，以BV开头。可从视频页面URL中获取。
        </div>
      </el-form-item>

      <el-form-item label="AV号" prop="avId">
        <el-input 
          v-model="form.avId" 
          placeholder="选填，留空自动从BV号转换"
          clearable
        >
          <template #prepend>
            <el-icon><Ticket /></el-icon>
          </template>
        </el-input>
        <div class="form-tip">
          选填，系统会自动从BV号转换获取。
        </div>
      </el-form-item>

      <el-form-item v-if="history?.bvId" label="">
        <el-checkbox v-model="form.force">
          强制覆盖已有投稿信息
        </el-checkbox>
        <div class="form-tip warning">
          当前已有投稿信息：{{ history.bvId }}
        </div>
      </el-form-item>

      <el-form-item label="快速填充">
        <el-button 
          size="small" 
          @click="pasteFromClipboard"
        >
          <el-icon><DocumentCopy /></el-icon>
          从剪贴板粘贴
        </el-button>
        <div class="form-tip">
          自动识别剪贴板中的BV号或视频链接。
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button 
        type="primary" 
        :loading="loading"
        @click="handleSubmit"
      >
        确认标记
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, reactive, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Link, Ticket, DocumentCopy } from '@element-plus/icons-vue'
import axios from 'axios'

const props = defineProps({
  visible: Boolean,
  history: Object
})

const emit = defineEmits(['update:visible', 'success'])

const formRef = ref(null)
const loading = ref(false)

const form = reactive({
  bvId: '',
  avId: '',
  force: false
})

// 表单验证规则
const rules = {
  bvId: [
    { required: true, message: '请输入BV号', trigger: 'blur' },
    { 
      pattern: /^BV[a-zA-Z0-9]{10}$/, 
      message: 'BV号格式错误，应为12位且以BV开头', 
      trigger: 'blur' 
    }
  ]
}

// 监听对话框打开，重置表单
watch(() => props.visible, (val) => {
  if (val) {
    form.bvId = ''
    form.avId = ''
    form.force = false
    formRef.value?.clearValidate()
  }
})

// 从剪贴板粘贴并识别BV号
const pasteFromClipboard = async () => {
  try {
    const text = await navigator.clipboard.readText()
    
    // 尝试从文本中提取BV号
    const bvMatch = text.match(/BV[a-zA-Z0-9]{10}/)
    if (bvMatch) {
      form.bvId = bvMatch[0]
      ElMessage.success('已识别BV号：' + bvMatch[0])
    } else {
      ElMessage.warning('未能识别到BV号，请手动输入')
    }
  } catch (error) {
    ElMessage.error('读取剪贴板失败，请手动输入')
  }
}

// 提交表单
const handleSubmit = async () => {
  try {
    await formRef.value.validate()
    
    loading.value = true
    const response = await axios.post(
      `/api/history/manualSetPublish/${props.history.id}`,
      {
        bvId: form.bvId.trim(),
        avId: form.avId.trim() || undefined,
        force: form.force
      }
    )

    if (response.data.type === 'success') {
      ElMessage.success(response.data.msg)
      emit('update:visible', false)
      emit('success')
    } else if (response.data.type === 'warning') {
      ElMessage.warning(response.data.msg)
    } else {
      ElMessage.error(response.data.msg || '操作失败')
    }
  } catch (error) {
    console.error('标记投稿失败:', error)
    if (error.response?.data?.msg) {
      ElMessage.error(error.response.data.msg)
    } else {
      ElMessage.error('标记投稿失败')
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.5;
}

.form-tip.warning {
  color: #e6a23c;
}

:deep(.el-alert) {
  line-height: 1.6;
}

:deep(.el-alert p) {
  margin: 0;
}
</style>
