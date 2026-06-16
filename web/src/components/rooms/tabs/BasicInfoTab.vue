<template>
  <div class="basic-info-tab">
    <el-form :model="localForm" label-width="120px">
      <el-form-item label="房间ID" required>
        <el-input 
          v-model="localForm.roomId" 
          placeholder="请输入B站直播间房间号"
        />
      </el-form-item>
      
      <el-form-item label="启用上传">
        <el-switch v-model="localForm.upload" />
        <div class="help-text">开启后才会处理该房间的录制文件上传</div>
      </el-form-item>
      
      <el-form-item label="上传用户">
        <el-select 
          v-model="localForm.uploadUserId" 
          placeholder="请选择用户"
          style="width: 100%"
        >
          <el-option
            v-for="user in users"
            :key="user.id"
            :label="`${user.uname || '未命名账号'} (${user.uid})`"
            :value="user.id"
          />
        </el-select>
        <div class="help-text">选择用于上传视频的B站账号</div>
      </el-form-item>

      <el-form-item label="任务优先级">
        <el-input-number
          v-model="localForm.priority"
          :min="0"
          :max="100"
          :step="5"
          controls-position="right"
          style="width: 200px"
        />
        <div class="help-text">自动上传调度优先处理高优先级房间；默认 50</div>
      </el-form-item>

      <el-form-item label="多账号策略">
        <el-select v-model="localForm.uploadUserStrategy" style="width: 100%">
          <el-option value="fixed" label="固定使用所选账号" />
          <el-option value="round_robin" label="轮询分配已登录账号" />
          <el-option value="least_queue" label="分配给队列最短账号" />
          <el-option value="daily_quota" label="按每日剩余配额分配" />
        </el-select>
        <div class="help-text">多账号策略只影响上传分P；投稿仍使用房间选择的上传用户，避免投稿归属混乱</div>
      </el-form-item>

      <el-form-item label="定时上传窗口">
        <el-switch v-model="localForm.uploadWindowEnabled" />
        <div class="help-text">启用后只在指定时间段内上传，可避开高峰期；支持跨天窗口</div>
      </el-form-item>

      <el-form-item v-if="localForm.uploadWindowEnabled" label="上传时间段">
        <div style="display:flex;gap:10px;align-items:center;width:100%">
          <el-time-picker
            v-model="localForm.uploadWindowStart"
            format="HH:mm"
            value-format="HH:mm"
            placeholder="开始时间"
            style="flex:1"
          />
          <span>至</span>
          <el-time-picker
            v-model="localForm.uploadWindowEnd"
            format="HH:mm"
            value-format="HH:mm"
            placeholder="结束时间"
            style="flex:1"
          />
        </div>
      </el-form-item>
      
      <el-divider content-position="left">自动化流程</el-divider>
      
      <el-alert
        title="自动化流程说明"
        type="info"
        :closable="false"
        style="margin-bottom: 20px"
      >
        <div style="line-height: 1.8">
          <strong>完整自动化流程：</strong>录制 → 自动上传 → 自动投稿 → 状态同步<br>
          <strong>SessionID合并：</strong>同一场直播的所有分P将自动合并到一个投稿，避免创建多个视频<br>
          <strong>推荐配置：</strong>全部开启
        </div>
      </el-alert>
      
      <el-form-item label="自动上传分P">
        <el-switch v-model="localForm.autoUpload" />
        <div class="help-text">📤 录制完成的分P将自动加入上传队列（需要先开启"启用上传"）</div>
      </el-form-item>
      
      <el-form-item label="自动投稿">
        <el-switch v-model="localForm.autoPublish" />
        <div class="help-text">📝 所有分P上传完成后将自动提交投稿</div>
      </el-form-item>
      
      <el-form-item label="SessionID合并">
        <el-switch v-model="localForm.mergeBySession" />
        <div class="help-text important-config">
          ⭐ <strong>重要功能</strong>：同一场直播的所有分P将自动合并到一个投稿<br>
          • 开启后：首次投稿创建视频，后续分P自动追加到该视频（推荐）<br>
          • 关闭后：每次投稿都会创建新的视频
        </div>
      </el-form-item>
      
      <el-form-item label="自动解析弹幕">
        <el-switch v-model="localForm.autoParseDanmaku" />
        <div class="help-text">💬 录制完成的分P将自动解析弹幕文件（追加分P时也会自动解析）</div>
      </el-form-item>
      
      <el-form-item label="定时同步信息">
        <el-switch v-model="localForm.autoSyncInfo" />
        <div class="help-text">🔄 每30分钟自动同步已投稿视频的审核状态</div>
      </el-form-item>
      
      <el-divider content-position="left">视频信息</el-divider>
      
      <el-form-item label="标题模板">
        <el-input 
          v-model="localForm.titleTemplate" 
          type="textarea" 
          :rows="2"
          placeholder="请输入视频标题模板"
        />
        <div class="help-text">
          支持变量: ${uname} ${title} ${sequence} ${yyyy年MM月dd日HH点mm分} ${roomId} ${areaName}
        </div>
      </el-form-item>
      
      <el-form-item label="简介模板">
        <el-input 
          v-model="localForm.descTemplate" 
          type="textarea" 
          :rows="3"
          placeholder="请输入视频简介模板"
        />
        <div class="help-text">支持与标题相同的变量</div>
      </el-form-item>
      
      <el-form-item label="标签">
        <el-input 
          v-model="localForm.tags" 
          placeholder="多个标签用逗号分隔，最多10个标签"
        />
        <div class="help-text">示例: 直播回放,${uname},${areaName}</div>
      </el-form-item>
      
      <el-divider content-position="left">分区设置</el-divider>
      
      <el-form-item label="投稿分区">
        <el-select
          v-model="localForm.tid"
          filterable
          placeholder="请选择或搜索分区"
          style="width: 100%"
        >
          <el-option-group label="游戏">
            <el-option label="电子竞技 (171)" :value="171" />
            <el-option label="手机游戏 (172)" :value="172" />
            <el-option label="网络游戏 (173)" :value="173" />
            <el-option label="单机游戏 (17)" :value="17" />
            <el-option label="游戏杂谈 (175)" :value="175" />
            <el-option label="音游 (176)" :value="176" />
            <el-option label="桌游棋牌 (161)" :value="161" />
          </el-option-group>
          <el-option-group label="知识·科技">
            <el-option label="野生技术协会 (122)" :value="122" />
            <el-option label="极客DIY (124)" :value="124" />
            <el-option label="软件应用 (230)" :value="230" />
          </el-option-group>
          <el-option-group label="生活">
            <el-option label="日常 (21)" :value="21" />
            <el-option label="搞笑 (138)" :value="138" />
            <el-option label="运动 (163)" :value="163" />
          </el-option-group>
          <el-option-group label="音乐">
            <el-option label="原创音乐 (28)" :value="28" />
            <el-option label="翻唱 (31)" :value="31" />
          </el-option-group>
          <el-option-group label="其他">
            <el-option label="综合 (131)" :value="131" />
          </el-option-group>
        </el-select>
        <div class="help-text">
          选择投稿分区，默认为「电子竞技 (171)」。游戏直播推荐使用游戏分区下的子分区；可在下拉框中搜索
        </div>
      </el-form-item>
      
      <el-form-item label="投稿合集">
        <div style="display: flex; gap: 8px; width: 100%">
          <el-select
            v-model="localForm.seasonId"
            placeholder="不加入合集"
            clearable
            style="flex: 1"
            :loading="loadingSeasons"
            :disabled="!localForm.uploadUserId"
            @focus="fetchSeasons"
          >
            <el-option label="不加入合集" :value="0" />
            <el-option
              v-for="season in seasons"
              :key="season.id"
              :label="season.name + ' (' + season.count + '个视频)'"
              :value="season.sectionId > 0 ? season.sectionId : season.id"
            />
          </el-select>
          <el-button
            :loading="loadingSeasons"
            :disabled="!localForm.uploadUserId"
            @click="fetchSeasons"
            circle
            :icon="RefreshIcon"
          />
          <el-button
            :disabled="!localForm.uploadUserId"
            @click="openCreateSeasonDialog"
            :icon="PlusIcon"
          >
            新建合集
          </el-button>
        </div>
        <div class="help-text">
          投稿成功后自动将视频加入指定合集。需先选择上传用户才能加载合集列表。若所选合集的节 ID 有误可能导致加入失败，建议通过刷新按钮重新拉取
        </div>
      </el-form-item>
      
      <el-form-item label="版权">
        <el-radio-group v-model="localForm.copyright">
          <el-radio :label="1">自制</el-radio>
          <el-radio :label="2">转载</el-radio>
        </el-radio-group>
      </el-form-item>
      
      <el-form-item label="转载来源模板" v-if="localForm.copyright === 2">
        <el-input 
          v-model="localForm.sourceTemplate"
          placeholder="直播间: https://live.bilibili.com/${roomId}  稿件直播源"
          type="textarea"
          :rows="2"
        />
        <div class="help-text">
          支持变量: ${roomId} ${uname} ${areaName} ${title} 等。留空则使用默认模板
        </div>
      </el-form-item>
      
      <el-form-item label="分P标题模板">
        <el-input 
          v-model="localForm.partTitleTemplate"
          placeholder="多P视频的分P标题"
        />
        <div class="help-text">
          支持变量: ${index} ${sequence} ${MM月dd日HH点mm分} ${areaName} ${fileName}
        </div>
      </el-form-item>

      <el-divider content-position="left">投稿权限</el-divider>

      <el-form-item label="仅自己可见">
        <el-switch v-model="localForm.isOnlySelf" />
        <div class="help-text">开启后投稿视频仅自己可见，方便校对内容后再公开</div>
      </el-form-item>

      <el-form-item label="不打扰模式">
        <el-switch v-model="localForm.noDisturbance" />
        <div class="help-text">开启后在设定的公屏区间内停止录制而不游荣主播（需录制端支持）</div>
      </el-form-item>
    </el-form>

    <el-dialog v-model="createSeasonVisible" title="新建投稿合集" width="520px">
      <el-form label-width="90px">
        <el-form-item label="合集标题" required>
          <el-input v-model="seasonForm.title" maxlength="80" show-word-limit />
        </el-form-item>
        <el-form-item label="合集简介">
          <el-input v-model="seasonForm.desc" type="textarea" :rows="3" maxlength="200" show-word-limit />
        </el-form-item>
        <el-form-item label="封面地址">
          <el-input v-model="seasonForm.cover" placeholder="可选，建议使用已上传的封面 URL" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createSeasonVisible = false">取消</el-button>
        <el-button type="primary" :loading="creatingSeason" @click="createSeason">创建并选择</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Plus as PlusIcon, Refresh as RefreshIcon } from '@element-plus/icons-vue'
import { roomAPI } from '@/api/index.js'
import { ElMessage } from 'element-plus'

const props = defineProps({
  modelValue: {
    type: Object,
    required: true
  },
  users: {
    type: Array,
    default: () => []
  }
})

const emit = defineEmits(['update:modelValue'])

const localForm = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const seasons = ref([])
const loadingSeasons = ref(false)
const createSeasonVisible = ref(false)
const creatingSeason = ref(false)
const seasonForm = ref({
  title: '',
  desc: '',
  cover: ''
})

const fetchSeasons = async () => {
  const userId = localForm.value.uploadUserId
  if (!userId) return
  loadingSeasons.value = true
  try {
    const data = await roomAPI.getSeasons(userId)
    seasons.value = Array.isArray(data) ? data : []
  } catch (e) {
    ElMessage.warning('获取合集列表失败: ' + (e?.message || e))
    seasons.value = []
  } finally {
    loadingSeasons.value = false
  }
}

const openCreateSeasonDialog = () => {
  const titleBase = localForm.value.uname || localForm.value.roomId || '直播回放'
  seasonForm.value = {
    title: `${titleBase} 直播回放`,
    desc: '',
    cover: localForm.value.coverUrl || ''
  }
  createSeasonVisible.value = true
}

const createSeason = async () => {
  const title = seasonForm.value.title?.trim()
  if (!title) {
    ElMessage.warning('请输入合集标题')
    return
  }
  creatingSeason.value = true
  try {
    const season = await roomAPI.createSeason({
      userId: localForm.value.uploadUserId,
      title,
      desc: seasonForm.value.desc || '',
      cover: seasonForm.value.cover || ''
    })
    if (season?.type === 'error' || season?.ok === false) {
      throw new Error(season.msg || '创建合集失败')
    }
    await fetchSeasons()
    const selectedId = season?.sectionId > 0 ? season.sectionId : season?.id
    if (selectedId) {
      localForm.value.seasonId = selectedId
    }
    createSeasonVisible.value = false
    ElMessage.success('合集已创建')
  } catch (e) {
    ElMessage.error('创建合集失败: ' + (e?.message || e))
  } finally {
    creatingSeason.value = false
  }
}

// 当上传用户改变时，自动刷新合集列表并清除当前已选合集
watch(
  () => localForm.value.uploadUserId,
  (newId, oldId) => {
    if (newId && newId !== oldId) {
      // 切换用户时清除已选合集（不同用户的合集不互通）
      if (oldId) {
        localForm.value.seasonId = 0
      }
      fetchSeasons()
    }
  },
  { immediate: true }
)
</script>

<style scoped>
.basic-info-tab {
  padding: 20px 0;
}

.help-text {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.5;
}

.help-text.important-config {
  color: #606266;
  background-color: #fef0f0;
  border-left: 3px solid #f89898;
  padding: 10px 12px;
  margin-top: 8px;
  border-radius: 4px;
  line-height: 1.8;
}

.help-text.important-config strong {
  color: #f56c6c;
}

:deep(.el-divider__text) {
  font-weight: 500;
  color: #303133;
}

:deep(.el-alert) {
  border-radius: 6px;
}

:deep(.el-alert__content) {
  font-size: 13px;
}
</style>
