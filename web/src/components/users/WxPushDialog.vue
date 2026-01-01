<template>
  <el-dialog 
    :model-value="visible"
    title="配置WxPusher推送" 
    width="500px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <el-form label-width="120px">
      <el-form-item label="WxPusher Token">
        <el-input
          v-model="localForm.token"
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
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button type="primary" @click="handleSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch } from 'vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  form: {
    type: Object,
    default: () => ({
      userId: null,
      token: ''
    })
  }
})

const emit = defineEmits(['update:visible', 'save'])

const localForm = ref({ ...props.form })

watch(() => props.form, (val) => {
  localForm.value = { ...val }
}, { deep: true })

const handleSave = () => {
  emit('save', localForm.value)
}
</script>
