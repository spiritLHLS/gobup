<template>
  <div class="operations-container">
    <div class="page-header">
      <h2>任务队列</h2>
      <p>查看上传队列、后台任务以及暂停、恢复、取消和重试操作</p>
    </div>

    <UploadQueueCard
      :status="queueStatus"
      :loading="queueLoading"
      @refresh="loadQueueStatus"
    />

    <TaskManagerCard
      :status="taskStatus"
      :loading="taskLoading"
      @refresh="loadTaskStatus"
    />
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { queueAPI } from '@/api'
import UploadQueueCard from '@/components/dashboard/UploadQueueCard.vue'
import TaskManagerCard from '@/components/dashboard/TaskManagerCard.vue'

const queueLoading = ref(false)
const taskLoading = ref(false)
const queueStatus = ref({
  counts: { pending: 0, running: 0, completed: 0 },
  pending: [],
  running: [],
  completed: [],
  queues: {}
})
const taskStatus = ref({})
let queueTimer = null

const loadQueueStatus = async () => {
  queueLoading.value = true
  try {
    queueStatus.value = await queueAPI.uploadStatus()
  } catch (error) {
    console.error('加载队列状态失败:', error)
  } finally {
    queueLoading.value = false
  }
}

const loadTaskStatus = async () => {
  taskLoading.value = true
  try {
    taskStatus.value = await queueAPI.taskStatus()
  } catch (error) {
    console.error('加载任务状态失败:', error)
  } finally {
    taskLoading.value = false
  }
}

onMounted(() => {
  loadQueueStatus()
  loadTaskStatus()
  queueTimer = setInterval(() => {
    loadQueueStatus()
    loadTaskStatus()
  }, 5000)
})

onUnmounted(() => {
  if (queueTimer) clearInterval(queueTimer)
})
</script>

<style scoped lang="scss">
.operations-container {
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: var(--spacing-lg);

  h2 {
    font-size: var(--font-size-3xl);
    color: var(--text-color-primary);
    font-weight: var(--font-weight-bold);
    margin: 0 0 8px 0;
  }

  p {
    font-size: var(--font-size-base);
    color: var(--text-color-secondary);
    margin: 0;
  }
}
</style>
