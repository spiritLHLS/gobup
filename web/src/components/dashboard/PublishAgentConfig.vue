<template>
  <div class="form-section">
    <div class="section-title">投稿与 Agent</div>

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

    <el-form-item label="Agent 用途">
      <div class="publish-mode-panel">
        <el-radio-group v-model="config.agentPurpose">
          <el-radio-button value="upload">上传投稿</el-radio-button>
          <el-radio-button value="filescan">录制文件检查</el-radio-button>
          <el-radio-button value="both">两者</el-radio-button>
        </el-radio-group>
        <span class="help-text">安装 Agent 时会写入相同用途；检测时会校验远端能力是否匹配</span>
      </div>
    </el-form-item>

    <el-form-item label="投稿执行模式">
      <div class="publish-mode-panel">
        <el-radio-group v-model="config.publishMode">
          <el-radio-button value="local">本地</el-radio-button>
          <el-radio-button value="remote">Agent</el-radio-button>
        </el-radio-group>
        <span class="help-text">本地模式由当前服务投稿；Agent 模式将投稿请求转交到配置的 Agent</span>
      </div>
    </el-form-item>

    <el-form-item label="文件检查模式">
      <div class="publish-mode-panel">
        <el-radio-group v-model="config.fileCheckMode">
          <el-radio-button value="local">本地</el-radio-button>
          <el-radio-button value="remote">Agent</el-radio-button>
        </el-radio-group>
        <span class="help-text">用于检查录制目录文件数量、大小和入库状态，不会导入或删除文件</span>
      </div>
    </el-form-item>

    <el-form-item label="当前 Agent 地址/IP">
      <div class="path-input-wrapper">
        <el-input
          v-model="config.publishAgentEndpoint"
          placeholder="192.0.2.10 或 agent.example.com，端口默认 12381"
          size="large"
        />
        <span class="help-text">可只填 IP/域名，保存后自动补全为 http://地址:12381</span>
      </div>
    </el-form-item>

    <el-form-item label="统一 Agent Token">
      <div class="path-input-wrapper">
        <el-input
          v-model="config.publishAgentToken"
          type="password"
          show-password
          placeholder="留空时后端自动生成"
          size="large"
        />
        <span class="help-text">主控面板生成并复用同一个 token，各 Agent 安装命令默认使用它</span>
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
        <span class="help-text">秒，包含 Agent 接收请求、扫盘或转发投稿的等待时间</span>
      </div>
    </el-form-item>

    <el-form-item label="Agent 检测">
      <div class="button-group">
        <el-button type="primary" plain :loading="detectingAgent" :icon="Connection" @click="$emit('detect', config.agentPurpose)">
          检测 Agent
        </el-button>
        <el-button plain :loading="checkingFiles" :icon="Search" @click="$emit('checkFiles')">
          检查录制文件
        </el-button>
        <span class="help-text">节点增删改、屏蔽和强制删除在左侧 Agent 管理中处理</span>
      </div>
    </el-form-item>

    <el-form-item v-if="fileCheckResult" label="检查结果">
      <div class="check-result">
        <el-tag type="info">文件 {{ fileCheckResult.totalFiles || 0 }}</el-tag>
        <el-tag v-if="fileCheckResult.databaseAware" type="success">未入库 {{ fileCheckResult.newFiles || 0 }}</el-tag>
        <el-tag>总大小 {{ formatBytes(fileCheckResult.totalSize || 0) }}</el-tag>
        <span class="help-text">{{ fileCheckSummary }}</span>
      </div>
    </el-form-item>

    <el-divider />

    <el-form-item label="安装脚本来源">
      <div class="publish-mode-panel">
        <el-radio-group v-model="config.agentInstallerSource">
          <el-radio-button value="controller">控制端</el-radio-button>
          <el-radio-button value="github">GitHub</el-radio-button>
          <el-radio-button value="cdn">CDN</el-radio-button>
        </el-radio-group>
        <span class="help-text">控制端优先使用当前服务托管的脚本和 release 包；缺失时回退 GitHub</span>
      </div>
    </el-form-item>

    <el-form-item label="控制端地址">
      <div class="path-input-wrapper">
        <el-input
          v-model="config.agentControllerBaseUrl"
          placeholder="留空时后端按当前访问地址生成"
          size="large"
        />
      </div>
    </el-form-item>

    <el-form-item label="Release 仓库">
      <div class="path-input-wrapper">
        <el-input
          v-model="config.agentGitHubRepo"
          placeholder="spiritlhls/gobup"
          size="large"
        />
        <span class="help-text">GitHub/CDN 来源会按该仓库的最新 Release 下载 gobup-agent-linux-*.tar.gz</span>
      </div>
    </el-form-item>

    <el-form-item label="CDN 基址">
      <div class="path-input-wrapper">
        <el-input
          v-model="config.agentCdnBaseUrl"
          placeholder="例如 https://cdn0.spiritlhl.top"
          size="large"
        />
      </div>
    </el-form-item>

    <el-form-item label="安装命令">
      <div class="install-command">
        <div class="button-group">
          <el-button plain :loading="generatingInstallCommand" :icon="Refresh" @click="$emit('loadInstallCommand')">
            生成命令
          </el-button>
          <el-button plain :disabled="!installCommand?.command" :icon="DocumentCopy" @click="copyInstallCommand">
            复制
          </el-button>
        </div>
        <el-input
          :model-value="installCommand?.command || ''"
          type="textarea"
          :rows="3"
          readonly
          placeholder="保存配置后生成安装命令"
        />
        <span v-if="installCommand?.tokenMissing" class="help-text danger-text">当前未配置 Agent Token，命令中的占位符需要先替换</span>
      </div>
    </el-form-item>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, DocumentCopy, Refresh, Search } from '@element-plus/icons-vue'
import { copyText } from '@/utils/clipboard'

const props = defineProps({
  config: {
    type: Object,
    required: true
  },
  detectingAgent: {
    type: Boolean,
    default: false
  },
  checkingFiles: {
    type: Boolean,
    default: false
  },
  generatingInstallCommand: {
    type: Boolean,
    default: false
  },
  installCommand: {
    type: Object,
    default: null
  },
  fileCheckResult: {
    type: Object,
    default: null
  }
})

defineEmits(['toggleFeature', 'detect', 'checkFiles', 'loadInstallCommand'])

const fileCheckSummary = computed(() => {
  const result = props.fileCheckResult
  if (!result) return ''
  const source = props.config.fileCheckMode === 'remote' ? 'Agent' : '本地'
  const errorCount = Array.isArray(result.errors) ? result.errors.length : 0
  return `${source}检查，样本 ${result.files?.length || 0} 个${errorCount ? `，错误 ${errorCount} 条` : ''}`
})

const formatBytes = (value) => {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = Number(value)
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

const copyInstallCommand = async () => {
  if (!props.installCommand?.command) return
  if (await copyText(props.installCommand.command)) {
    ElMessage.success('安装命令已复制')
  } else {
    ElMessage.error('当前浏览器禁止自动复制')
  }
}
</script>

<style scoped>
.switch-item,
.number-input-wrapper,
.path-input-wrapper,
.publish-mode-panel,
.button-group,
.check-result {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex-wrap: wrap;
}

.button-group .el-button {
  min-width: 120px;
}

.install-command {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  width: 100%;
}

.help-text {
  flex: 1;
  min-width: 200px;
  color: var(--text-color-secondary);
  font-size: var(--font-size-sm);
  line-height: 1.6;
}

.danger-text {
  color: var(--danger-color, #f56c6c);
}
</style>
