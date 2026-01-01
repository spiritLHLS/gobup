<template>
  <div class="cookie-container">
    <el-form label-width="0">
      <el-form-item>
        <el-input
          v-model="localCookieInput"
          type="textarea"
          :rows="6"
          placeholder="请粘贴完整的Cookie，格式如：&#10;SESSDATA=xxx; DedeUserID=xxx; DedeUserID__ckMd5=xxx; bili_jct=xxx"
          clearable
        />
        <div class="cookie-tips">
          <p>💡 Cookie获取方法：</p>
          <ol>
            <li>使用浏览器登录 <a href="https://www.bilibili.com" target="_blank">bilibili.com</a></li>
            <li>按F12打开开发者工具 → Network（网络）</li>
            <li>刷新页面，点击任意请求</li>
            <li>在Request Headers中找到Cookie，复制完整内容</li>
          </ol>
          <p class="warning">⚠️ 请勿将Cookie泄露给他人</p>
        </div>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'

const props = defineProps({
  cookieInput: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['update:cookieInput'])

const localCookieInput = ref(props.cookieInput)

watch(localCookieInput, (val) => {
  emit('update:cookieInput', val)
})

watch(() => props.cookieInput, (val) => {
  localCookieInput.value = val
})
</script>

<style scoped>
.cookie-container {
  padding: 10px 0;
}

.cookie-tips {
  margin-top: 15px;
  padding: 15px;
  background-color: #f5f7fa;
  border-radius: 4px;
  font-size: 13px;
  color: #666;
  line-height: 1.8;
}

.cookie-tips p {
  margin: 8px 0;
}

.cookie-tips ol {
  margin: 10px 0;
  padding-left: 20px;
}

.cookie-tips ol li {
  margin: 5px 0;
}

.cookie-tips a {
  color: #1890ff;
  text-decoration: none;
}

.cookie-tips a:hover {
  text-decoration: underline;
}

.cookie-tips .warning {
  color: #ff4d4f;
  font-weight: bold;
  margin-top: 10px;
}
</style>
