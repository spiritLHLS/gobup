<template>
  <div class="notification-settings-tab">
    <el-form :model="localForm" label-width="120px">
      <el-divider content-position="left">WxPusher推送</el-divider>
      
      <el-form-item label="WxPusher UID">
        <el-input 
          v-model="localForm.wxuid" 
          placeholder="填写后将推送通知到微信"
        />
        <div class="help-text">
          获取地址: 
          <a 
            href="https://wxpusher.zjiecode.com/" 
            target="_blank"
            class="link"
          >
            https://wxpusher.zjiecode.com/
          </a>
        </div>
      </el-form-item>
      
      <el-form-item label="推送消息类型">
        <el-checkbox-group v-model="pushTags">
          <el-checkbox label="开播">
            <div class="checkbox-content">
              <span class="checkbox-label">开播通知</span>
              <div class="checkbox-desc">直播间开播时推送</div>
            </div>
          </el-checkbox>
          <el-checkbox label="上传">
            <div class="checkbox-content">
              <span class="checkbox-label">上传通知</span>
              <div class="checkbox-desc">开始上传视频时推送</div>
            </div>
          </el-checkbox>
          <el-checkbox label="投稿">
            <div class="checkbox-content">
              <span class="checkbox-label">投稿通知</span>
              <div class="checkbox-desc">视频投稿成功时推送</div>
            </div>
          </el-checkbox>
        </el-checkbox-group>
      </el-form-item>
      
      <el-alert
        v-if="localForm.wxuid"
        title="推送提示"
        type="info"
        :closable="false"
        style="margin-top: 20px;"
      >
        <div>将在以下情况推送通知：</div>
        <ul style="margin: 10px 0 0 20px; padding: 0;">
          <li v-if="pushTags.includes('开播')">直播间开播</li>
          <li v-if="pushTags.includes('上传')">开始上传视频到B站</li>
          <li v-if="pushTags.includes('投稿')">视频投稿成功</li>
        </ul>
        <div v-if="pushTags.length === 0" style="color: #e6a23c;">
          ⚠️ 未选择任何推送类型，将不会收到通知
        </div>
      </el-alert>
      
      <el-alert
        v-else
        title="未配置推送"
        type="warning"
        :closable="false"
        style="margin-top: 20px;"
      >
        请先配置 WxPusher UID 才能接收推送通知
      </el-alert>
    </el-form>
  </div>
</template>

<script setup>
import { computed, watch } from 'vue'

const props = defineProps({
  modelValue: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['update:modelValue'])

const localForm = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const pushTags = computed({
  get: () => {
    return localForm.value.pushMsgTags 
      ? localForm.value.pushMsgTags.split(',').filter(Boolean)
      : []
  },
  set: (val) => {
    localForm.value.pushMsgTags = val.join(',')
  }
})
</script>

<style scoped>
.notification-settings-tab {
  padding: 20px 0;
}

.help-text {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.5;
}

.link {
  color: #409eff;
  text-decoration: none;
}

.link:hover {
  text-decoration: underline;
}

:deep(.el-checkbox) {
  display: flex;
  align-items: flex-start;
  margin-bottom: 15px;
}

.checkbox-content {
  display: flex;
  flex-direction: column;
  margin-left: 5px;
}

.checkbox-label {
  font-size: 14px;
  color: #303133;
}

.checkbox-desc {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

:deep(.el-divider__text) {
  font-weight: 500;
  color: #303133;
}

:deep(.el-alert ul) {
  list-style-type: disc;
}
</style>
