<template>
  <div class="agents-container">
    <el-card>
      <template #header>
        <div class="card-header">
          <div class="header-title">Agent 管理</div>
          <div class="header-actions">
            <el-button plain :icon="Refresh" :loading="loading" @click="loadAgents">
              刷新
            </el-button>
            <el-button type="primary" :icon="Plus" @click="openCreateDialog">
              添加 Agent
            </el-button>
          </div>
        </div>
      </template>

      <el-table :data="agents" v-loading="loading" style="width: 100%">
        <el-table-column prop="id" label="ID" width="72" />
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="endpoint" label="地址" min-width="220" show-overflow-tooltip />
        <el-table-column label="用途" width="130">
          <template #default="{ row }">
            <el-tag :type="purposeTagType(row.purpose)" effect="plain">
              {{ purposeLabel(row.purpose) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="优先级" width="100">
          <template #default="{ row }">
            {{ row.priority ?? 50 }}
          </template>
        </el-table-column>
        <el-table-column label="状态" width="190">
          <template #default="{ row }">
            <div class="status-tags">
              <el-tag v-if="row.isPrimary" type="success">当前</el-tag>
              <el-tag v-if="row.blocked" type="danger">已屏蔽</el-tag>
              <el-tag v-else-if="row.enabled" type="success" effect="plain">启用</el-tag>
              <el-tag v-else type="info" effect="plain">停用</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="健康" min-width="220">
          <template #default="{ row }">
            <div class="health-cell">
              <el-tag :type="healthTagType(row.lastHealthStatus)" size="small">
                {{ healthLabel(row.lastHealthStatus) }}
              </el-tag>
              <span class="muted-text">{{ row.lastHealthMessage || '-' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="最近检测" width="170">
          <template #default="{ row }">
            {{ formatTime(row.lastSeenAt || row.updatedAt) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" fixed="right" width="430">
          <template #default="{ row }">
            <div class="row-actions">
              <el-button size="small" :icon="Connection" :loading="row.detecting" @click="detectAgent(row)">
                检测
              </el-button>
              <el-button size="small" type="primary" plain :disabled="!canUseAgent(row)" @click="useAgent(row)">
                设为当前
              </el-button>
              <el-button size="small" :icon="DocumentCopy" @click="openInstallDialog(row)">
                命令
              </el-button>
              <el-button size="small" :icon="Edit" @click="openEditDialog(row)">
                编辑
              </el-button>
              <el-button
                v-if="row.blocked"
                size="small"
                type="success"
                plain
                :icon="Unlock"
                @click="unblockAgent(row)"
              >
                解屏
              </el-button>
              <el-button
                v-else
                size="small"
                type="warning"
                plain
                :icon="Lock"
                @click="blockAgent(row)"
              >
                屏蔽
              </el-button>
              <el-dropdown trigger="click" @command="cmd => handleDeleteCommand(row, cmd)">
                <el-button size="small" type="danger" plain :icon="Delete">
                  删除
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="soft">删除</el-dropdown-item>
                    <el-dropdown-item command="force">强制删除</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="editDialogVisible"
      :title="editingAgent?.id ? '编辑 Agent' : '添加 Agent'"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form :model="agentForm" label-width="110px">
        <el-form-item label="名称">
          <el-input v-model="agentForm.name" placeholder="例如 华东上传 Agent" />
        </el-form-item>
        <el-form-item label="地址/IP" required>
          <el-input v-model="agentForm.endpoint" placeholder="192.0.2.10 或 agent.example.com" />
        </el-form-item>
        <el-form-item label="用途">
          <el-radio-group v-model="agentForm.purpose">
            <el-radio-button value="upload">上传投稿</el-radio-button>
            <el-radio-button value="filescan">文件检查</el-radio-button>
            <el-radio-button value="both">两者</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="agentForm.priority" :min="0" :max="100" :step="5" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="agentForm.enabled" :disabled="agentForm.blocked" />
        </el-form-item>
        <el-form-item label="屏蔽">
          <el-switch v-model="agentForm.blocked" />
        </el-form-item>
        <el-form-item v-if="agentForm.blocked" label="屏蔽原因">
          <el-input v-model="agentForm.blockReason" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingAgent" @click="saveAgent">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="installDialogVisible"
      title="安装命令"
      width="760px"
      :close-on-click-modal="false"
    >
      <div class="install-dialog">
        <div class="install-options">
          <el-radio-group v-model="installSource" @change="loadInstallCommand">
            <el-radio-button value="controller">控制端</el-radio-button>
            <el-radio-button value="github">GitHub</el-radio-button>
            <el-radio-button value="cdn">CDN</el-radio-button>
          </el-radio-group>
          <el-button plain :icon="Refresh" :loading="generatingCommand" @click="loadInstallCommand">
            重新生成
          </el-button>
          <el-button type="primary" plain :disabled="!installCommand" :icon="DocumentCopy" @click="copyInstallCommand">
            复制
          </el-button>
        </div>
        <el-input
          :model-value="installCommand"
          type="textarea"
          :rows="5"
          readonly
          placeholder="选择 Agent 后生成安装命令"
        />
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Connection, Delete, DocumentCopy, Edit, Lock, Plus, Refresh, Unlock } from '@element-plus/icons-vue'
import { agentAPI } from '@/api'
import { copyText } from '@/utils/clipboard'

const agents = ref([])
const loading = ref(false)
const editDialogVisible = ref(false)
const savingAgent = ref(false)
const editingAgent = ref(null)
const installDialogVisible = ref(false)
const installAgent = ref(null)
const installSource = ref('controller')
const installCommand = ref('')
const generatingCommand = ref(false)

const agentForm = reactive({
  name: '',
  endpoint: '',
  purpose: 'both',
  priority: 50,
  enabled: true,
  blocked: false,
  blockReason: ''
})

const resetForm = (agent = null) => {
  editingAgent.value = agent
  agentForm.name = agent?.name || ''
  agentForm.endpoint = agent?.endpoint || ''
  agentForm.purpose = agent?.purpose || 'both'
  agentForm.priority = agent?.priority ?? 50
  agentForm.enabled = agent?.enabled ?? true
  agentForm.blocked = agent?.blocked ?? false
  agentForm.blockReason = agent?.blockReason || ''
}

const loadAgents = async () => {
  loading.value = true
  try {
    const response = await agentAPI.list()
    if (response.type === 'success') {
      agents.value = Array.isArray(response.data) ? response.data : []
    } else {
      ElMessage.error(response.msg || '加载 Agent 失败')
    }
  } catch (error) {
    console.error('加载 Agent 失败:', error)
  } finally {
    loading.value = false
  }
}

const openCreateDialog = () => {
  resetForm()
  editDialogVisible.value = true
}

const openEditDialog = (agent) => {
  resetForm(agent)
  editDialogVisible.value = true
}

const saveAgent = async () => {
  if (!agentForm.endpoint.trim()) {
    ElMessage.warning('请输入 Agent 地址/IP')
    return
  }
  savingAgent.value = true
  try {
    const payload = { ...agentForm }
    const response = editingAgent.value?.id
      ? await agentAPI.update(editingAgent.value.id, payload)
      : await agentAPI.create(payload)
    if (response.type === 'success') {
      ElMessage.success(response.msg || '保存成功')
      editDialogVisible.value = false
      await loadAgents()
      if (!editingAgent.value?.id && response.data?.id) {
        const created = agents.value.find(item => item.id === response.data.id) || response.data
        await openInstallDialog(created)
      }
    } else {
      ElMessage.error(response.msg || '保存失败')
    }
  } catch (error) {
    console.error('保存 Agent 失败:', error)
  } finally {
    savingAgent.value = false
  }
}

const detectAgent = async (agent) => {
  agent.detecting = true
  try {
    const response = await agentAPI.detect(agent.id)
    if (response.type === 'success') {
      ElMessage.success(response.msg || 'Agent 可用')
    } else {
      ElMessage.error(response.msg || 'Agent 检测失败')
    }
    await loadAgents()
  } catch (error) {
    console.error('检测 Agent 失败:', error)
  } finally {
    agent.detecting = false
  }
}

const useAgent = async (agent) => {
  if (!canUseAgent(agent)) {
    ElMessage.warning('请先检测该 Agent，确认健康状态正常')
    return
  }
  const response = await agentAPI.use(agent.id)
  if (response.type === 'success') {
    ElMessage.success(response.msg || '已设为当前 Agent')
    await loadAgents()
  } else {
    ElMessage.error(response.msg || '设置当前 Agent 失败')
  }
}

const blockAgent = async (agent) => {
  try {
    const { value } = await ElMessageBox.prompt('屏蔽原因', `屏蔽 Agent ${agent.name || agent.endpoint}`, {
      confirmButtonText: '屏蔽',
      cancelButtonText: '取消',
      inputType: 'textarea',
      inputPlaceholder: '可留空'
    })
    const response = await agentAPI.block(agent.id, value || '')
    if (response.type === 'success') {
      ElMessage.success(response.msg || 'Agent 已屏蔽')
      await loadAgents()
    } else {
      ElMessage.error(response.msg || '屏蔽失败')
    }
  } catch (error) {
    if (error !== 'cancel') console.error('屏蔽 Agent 失败:', error)
  }
}

const unblockAgent = async (agent) => {
  const response = await agentAPI.unblock(agent.id)
  if (response.type === 'success') {
    ElMessage.success(response.msg || 'Agent 已启用')
    await loadAgents()
  } else {
    ElMessage.error(response.msg || '解除屏蔽失败')
  }
}

const handleDeleteCommand = async (agent, command) => {
  const force = command === 'force'
  try {
    await ElMessageBox.confirm(
      force ? '强制删除会永久移除该 Agent 记录。' : '删除后可通过数据库软删除记录恢复。',
      force ? '强制删除 Agent' : '删除 Agent',
      { confirmButtonText: force ? '强制删除' : '删除', cancelButtonText: '取消', type: 'warning' }
    )
    const response = await agentAPI.delete(agent.id, force)
    if (response.type === 'success') {
      ElMessage.success(response.msg || '删除成功')
      await loadAgents()
    } else {
      ElMessage.error(response.msg || '删除失败')
    }
  } catch (error) {
    if (error !== 'cancel') console.error('删除 Agent 失败:', error)
  }
}

const openInstallDialog = async (agent) => {
  installAgent.value = agent
  installSource.value = 'controller'
  installCommand.value = ''
  installDialogVisible.value = true
  await loadInstallCommand()
}

const loadInstallCommand = async () => {
  if (!installAgent.value?.id) return
  generatingCommand.value = true
  try {
    const response = await agentAPI.installCommand(installAgent.value.id, {
      source: installSource.value,
      purpose: installAgent.value.purpose
    })
    if (response.type === 'success') {
      installCommand.value = response.command || ''
    } else {
      ElMessage.error(response.msg || '生成安装命令失败')
    }
  } catch (error) {
    console.error('生成安装命令失败:', error)
  } finally {
    generatingCommand.value = false
  }
}

const copyInstallCommand = async () => {
  if (!installCommand.value) return
  if (await copyText(installCommand.value)) {
    ElMessage.success('安装命令已复制')
  } else {
    ElMessage.error('当前浏览器禁止自动复制')
  }
}

const purposeLabel = (purpose) => {
  const normalized = purpose || 'both'
  if (normalized === 'upload') return '上传投稿'
  if (normalized === 'filescan') return '文件检查'
  return '两者'
}

const purposeTagType = (purpose) => {
  if (purpose === 'upload') return 'primary'
  if (purpose === 'filescan') return 'warning'
  return 'success'
}

const healthLabel = (status) => {
  if (status === 'success') return '正常'
  if (status === 'error') return '异常'
  return '未检测'
}

const healthTagType = (status) => {
  if (status === 'success') return 'success'
  if (status === 'error') return 'danger'
  return 'info'
}

const canUseAgent = (agent) => {
  return Boolean(agent?.enabled && !agent?.blocked && agent?.lastHealthStatus === 'success')
}

const formatTime = (value) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', { hour12: false })
}

onMounted(loadAgents)
</script>

<style scoped>
.agents-container {
  padding: 20px;
}

.card-header,
.header-actions,
.status-tags,
.row-actions,
.install-options,
.health-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.card-header {
  justify-content: space-between;
}

.header-title {
  font-size: 16px;
  font-weight: 600;
}

.row-actions {
  gap: 6px;
}

.muted-text {
  color: var(--text-color-secondary);
  font-size: 13px;
}

.install-dialog {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

@media (max-width: 768px) {
  .agents-container {
    padding: 12px;
  }
}
</style>
