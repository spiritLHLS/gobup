<template>
  <el-dialog 
    :model-value="visible"
    title="弹幕代理配置" 
    width="600px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <el-form label-width="120px">
      <el-form-item label="启用代理池">
        <el-switch v-model="localConfig.enableDanmakuProxy" />
        <div style="margin-top: 8px; font-size: 12px; color: #999;">
          启用后，发送弹幕时将轮询使用代理池中的IP，突破单IP限流
        </div>
      </el-form-item>
      
      <el-form-item label="代理列表" v-if="localConfig.enableDanmakuProxy">
        <el-input
          v-model="localConfig.danmakuProxyList"
          type="textarea"
          :rows="10"
          placeholder="每行一个代理，支持格式：&#10;socks5://ip:port&#10;socks5://user:pass@ip:port&#10;http://ip:port&#10;http://user:pass@ip:port&#10;https://ip:port&#10;&#10;示例：&#10;socks5://127.0.0.1:1080&#10;http://user:pass@proxy.example.com:8080"
        />
        <div style="margin-top: 8px; font-size: 12px; color: #666;">
          <p style="margin: 4px 0;">💡 使用说明：</p>
          <ul style="margin: 4px 0; padding-left: 20px;">
            <li>每行一个代理地址，支持 socks5 和 http(s) 协议</li>
            <li>系统会自动包含本地IP，无需单独配置</li>
            <li>每个IP独立限流（22秒/条），实现真正的并行发送</li>
            <li>代理池会轮询使用所有可用IP</li>
            <li>以 # 开头的行会被忽略（可用于注释）</li>
          </ul>
        </div>
      </el-form-item>

      <el-alert
        v-if="localConfig.enableDanmakuProxy && proxyCount > 0"
        :title="`当前配置了 ${proxyCount} 个代理IP + 1 个本地IP，总计 ${proxyCount + 1} 个IP`"
        type="success"
        :closable="false"
        style="margin-top: 10px;"
      />

      <el-alert
        v-if="localConfig.enableDanmakuProxy && !localConfig.danmakuProxyList"
        title="未配置代理，将仅使用本地IP"
        type="warning"
        :closable="false"
        style="margin-top: 10px;"
      />
    </el-form>
    
    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button type="primary" @click="handleSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch, computed } from 'vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  config: {
    type: Object,
    default: () => ({
      enableDanmakuProxy: false,
      danmakuProxyList: ''
    })
  }
})

const emit = defineEmits(['update:visible', 'save'])

const localConfig = ref({ ...props.config })

watch(() => props.config, (val) => {
  localConfig.value = { ...val }
}, { deep: true })

const proxyCount = computed(() => {
  if (!localConfig.value.danmakuProxyList) {
    return 0
  }
  
  const lines = localConfig.value.danmakuProxyList.split('\n')
  return lines.filter(line => {
    const trimmed = line.trim()
    return trimmed && !trimmed.startsWith('#')
  }).length
})

const handleSave = () => {
  emit('save', localConfig.value)
}
</script>

<style scoped>
:deep(.el-textarea__inner) {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;
  font-size: 13px;
}
</style>
