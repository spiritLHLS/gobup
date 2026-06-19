<template>
  <div class="form-section">
    <div class="section-title">投稿执行</div>

    <el-form-item label="边录制边上传">
      <div class="switch-item">
        <el-switch
          v-model="config.uploadWhileRecording"
          @change="$emit('toggleFeature', 'uploadWhileRecording', $event)"
          size="large"
        />
        <span class="help-text">开启后，文件稳定即可预上传；关闭时必须等待对应直播结束</span>
      </div>
    </el-form-item>

    <el-form-item label="边录制边投稿">
      <div class="switch-item">
        <el-switch
          v-model="config.publishWhileRecording"
          @change="$emit('toggleFeature', 'publishWhileRecording', $event)"
          size="large"
        />
        <span class="help-text">开启后，已上传分P可先投稿，后续同场次分P会自动追加</span>
      </div>
    </el-form-item>

    <el-form-item label="投稿执行模式">
      <div class="publish-mode-panel">
        <el-radio-group v-model="config.publishMode">
          <el-radio-button value="local">本地</el-radio-button>
          <el-radio-button value="remote">远程 Agent</el-radio-button>
        </el-radio-group>
        <span class="help-text">本地模式由当前服务投稿；远程模式将投稿请求转交到配置的 Agent</span>
      </div>
    </el-form-item>

    <template v-if="config.publishMode === 'remote'">
      <el-form-item label="Agent 地址">
        <div class="path-input-wrapper">
          <el-input
            v-model="config.publishAgentEndpoint"
            placeholder="http://127.0.0.1:12380 或 https://agent.example.com"
            size="large"
          />
          <span class="help-text">Agent 需暴露 /agent/v1/health 和 /agent/v1/publish，并能访问相同录制文件和数据库上下文</span>
        </div>
      </el-form-item>

      <el-form-item label="Agent Token">
        <div class="path-input-wrapper">
          <el-input
            v-model="config.publishAgentToken"
            type="password"
            show-password
            placeholder="用于保护 Agent 接口的访问令牌"
            size="large"
          />
        </div>
      </el-form-item>

      <el-form-item label="Agent 超时">
        <div class="number-input-wrapper">
          <el-input-number
            v-model="config.publishAgentTimeout"
            :min="3"
            :max="600"
            :step="5"
            size="large"
          />
          <span class="help-text">秒，包含 Agent 接收请求和启动投稿流程的等待时间</span>
        </div>
      </el-form-item>

      <el-form-item label="Agent 检测">
        <div class="button-group">
          <el-button type="primary" plain :loading="detectingAgent" :icon="Connection" @click="$emit('detect')">
            检测 Agent
          </el-button>
          <span class="help-text">保存配置后检测远程 Agent 是否可达、token 是否正确</span>
        </div>
      </el-form-item>
    </template>
  </div>
</template>

<script setup>
import { Connection } from '@element-plus/icons-vue'

defineProps({
  config: {
    type: Object,
    required: true
  },
  detectingAgent: {
    type: Boolean,
    default: false
  }
})

defineEmits(['toggleFeature', 'detect'])
</script>

<style scoped>
.switch-item,
.number-input-wrapper,
.path-input-wrapper,
.publish-mode-panel,
.button-group {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.button-group .el-button {
  min-width: 120px;
}

.help-text {
  flex: 1;
  min-width: 200px;
  color: var(--text-color-secondary);
  font-size: var(--font-size-sm);
  line-height: 1.6;
}
</style>
