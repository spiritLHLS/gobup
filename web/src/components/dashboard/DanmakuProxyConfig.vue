<template>
  <div class="form-section">
    <div class="section-title">弹幕代理配置（全局）</div>
    
    <el-form-item label="启用代理池">
      <div class="switch-item">
        <el-switch 
          v-model="config.enableDanmakuProxy" 
          @change="$emit('toggleFeature', 'enableDanmakuProxy', $event)"
          size="large"
        />
        <span class="help-text">启用后，发送弹幕时将轮询使用代理池中的IP，突破单IP限流</span>
      </div>
    </el-form-item>

    <el-form-item label="代理列表">
      <div class="proxy-input-wrapper">
        <el-input
          v-model="config.danmakuProxyList"
          type="textarea"
          :rows="10"
          placeholder="每行一个代理，支持格式：&#10;socks5://ip:port&#10;socks5://user:pass@ip:port&#10;http://ip:port&#10;http://user:pass@ip:port&#10;https://ip:port&#10;&#10;示例：&#10;socks5://127.0.0.1:1080&#10;http://user:pass@proxy.example.com:8080"
          size="large"
        />
      </div>
    </el-form-item>

    <el-alert
      v-if="config.enableDanmakuProxy && proxyCount > 0"
      :title="`当前配置了 ${proxyCount} 个代理IP + 1 个本地IP，总计 ${proxyCount + 1} 个IP`"
      type="success"
      :closable="false"
      class="proxy-alert"
    />

    <el-alert
      v-if="config.enableDanmakuProxy && !config.danmakuProxyList"
      title="未配置代理，将仅使用本地IP"
      type="warning"
      :closable="false"
      class="proxy-alert"
    />

    <el-alert
      v-if="config.enableDanmakuProxy"
      type="info"
      :closable="false"
    >
      <template #default>
        <div class="proxy-tip">
          <p><strong>使用说明：</strong></p>
          <ul>
            <li>每行一个代理地址，支持 socks5 和 http(s) 协议</li>
            <li>系统会自动包含本地IP，无需单独配置</li>
            <li>每个IP独立限流（22秒/条），实现真正的并行发送</li>
            <li>代理池会轮询使用所有可用IP（所有用户共享此代理池）</li>
            <li>以 # 开头的行会被忽略（可用于注释）</li>
          </ul>
        </div>
      </template>
    </el-alert>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  config: {
    type: Object,
    required: true
  }
})

defineEmits(['toggleFeature'])

const proxyCount = computed(() => {
  if (!props.config.danmakuProxyList) {
    return 0
  }
  
  const lines = props.config.danmakuProxyList.split('\n')
  return lines.filter(line => {
    const trimmed = line.trim()
    return trimmed && !trimmed.startsWith('#')
  }).length
})
</script>

<style scoped lang="scss">
.form-section {
  margin-bottom: var(--spacing-xl);
  
  .section-title {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
    color: var(--text-color-primary);
    margin-bottom: var(--spacing-lg);
    padding-bottom: var(--spacing-sm);
    border-bottom: 2px solid var(--primary-color);
    display: inline-block;
  }
}

.switch-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.help-text {
  flex: 1;
  min-width: 200px;
  color: var(--text-color-secondary);
  font-size: var(--font-size-sm);
  line-height: 1.6;
}

.proxy-input-wrapper {
  width: 100%;
  
  :deep(.el-textarea) {
    width: 100%;
    max-width: 100%;
    font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;
    font-size: 13px;
  }
  
  :deep(.el-textarea__inner) {
    font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;
    font-size: 13px;
  }
}

.proxy-alert {
  margin-top: 10px;
  margin-bottom: 10px;
}

.proxy-tip {
  font-size: 12px;
  line-height: 1.6;
  
  p {
    margin: 4px 0;
  }
  
  ul {
    margin: 4px 0;
    padding-left: 20px;
  }
}

@media (max-width: 768px) {
  .switch-item {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-sm);
  }
  
  .help-text {
    min-width: auto;
  }
}
</style>
