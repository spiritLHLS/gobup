<template>
  <el-dialog
    :model-value="visible"
    :title="title"
    width="800px"
    @update:model-value="$emit('update:visible', $event)"
  >
    <el-form :model="localForm" label-width="120px">
      <el-form-item label="房间ID" required>
        <el-input v-model="localForm.roomId" />
      </el-form-item>
      <el-form-item label="是否上传">
        <el-switch v-model="localForm.upload" />
      </el-form-item>
      <el-form-item label="上传用户">
        <el-select v-model="localForm.uploadUserId" placeholder="请选择用户">
          <el-option
            v-for="user in users"
            :key="user.id"
            :label="user.name"
            :value="user.id"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="标题模板">
        <el-input v-model="localForm.titleTemplate" type="textarea" :rows="2" />
        <div class="help-text">支持变量: ${uname} ${title} ${yyyy年MM月dd日HH点mm分} ${roomId} ${areaName}</div>
      </el-form-item>
      <el-form-item label="简介模板">
        <el-input v-model="localForm.descTemplate" type="textarea" :rows="3" />
      </el-form-item>
      <el-form-item label="标签">
        <el-input v-model="localForm.tags" placeholder="多个标签用逗号分隔" />
      </el-form-item>
      <el-form-item label="分区ID">
        <el-input-number v-model="localForm.tid" :min="1" />
      </el-form-item>
      <el-form-item label="版权">
        <el-radio-group v-model="localForm.copyright">
          <el-radio :label="1">自制</el-radio>
          <el-radio :label="2">转载</el-radio>
        </el-radio-group>
      </el-form-item>
      
      <!-- 上传线路 -->
      <LineSelector
        v-model:line="localForm.line"
        :line-stats="lineStats"
        :line-speeds="lineSpeeds"
        :upload-lines="uploadLines"
        :testing-lines="testingLines"
        :testing-deep-speed="testingDeepSpeed"
        @test-lines="$emit('test-lines')"
        @test-deep-speed="$emit('test-deep-speed')"
      />
      
      <el-form-item label="封面配置">
        <el-radio-group v-model="localForm.coverType">
          <el-radio label="default">不使用封面</el-radio>
          <el-radio label="live">使用直播首帧</el-radio>
          <el-radio label="diy">自定义封面</el-radio>
        </el-radio-group>
        <div v-if="localForm.coverType === 'diy'" style="margin-top: 10px;">
          <el-upload
            :action="`/api/rooms/${localForm.id}/cover`"
            :show-file-list="false"
            :on-success="handleCoverUploadSuccess"
            :before-upload="beforeCoverUpload"
            accept="image/*"
          >
            <el-button size="small">
              <el-icon><Upload /></el-icon>
              上传封面
            </el-button>
          </el-upload>
          <div class="help-text">支持jpg/png，建议尺寸：960x600</div>
          <img v-if="localForm.coverUrl" :src="localForm.coverUrl" style="max-width: 200px; margin-top: 10px;" />
        </div>
      </el-form-item>
      
      <el-form-item label="高能剪辑">
        <el-switch v-model="localForm.highEnergyCut" />
        <div class="help-text">基于弹幕密度自动剪辑高能片段（需要ffmpeg）</div>
        <div v-if="localForm.highEnergyCut" style="margin-top: 10px;">
          <el-form-item label="窗口大小(秒)" label-width="120px">
            <el-input-number v-model="localForm.windowSize" :min="10" :max="300" />
          </el-form-item>
          <el-form-item label="阈值百分位" label-width="120px">
            <el-input-number v-model="localForm.percentileRank" :min="50" :max="99" />
            <div class="help-text">值越大，筛选越严格（75推荐）</div>
          </el-form-item>
          <el-form-item label="最小片段(秒)" label-width="120px">
            <el-input-number v-model="localForm.minSegmentDuration" :min="5" :max="60" />
          </el-form-item>
        </div>
      </el-form-item>
      
      <el-form-item label="弹幕过滤">
        <el-checkbox v-model="localForm.dmDistinct">去除重复弹幕</el-checkbox>
        <div style="margin-top: 10px;">
          <el-form-item label="最低用户等级" label-width="120px">
            <el-input-number v-model="localForm.dmUlLevel" :min="0" :max="6" />
            <div class="help-text">0表示不过滤，1-6对应B站等级</div>
          </el-form-item>
          <el-form-item label="粉丝勋章过滤" label-width="120px">
            <el-select v-model="localForm.dmMedalLevel">
              <el-option :value="0" label="不过滤" />
              <el-option :value="1" label="仅佩戴粉丝勋章" />
              <el-option :value="2" label="仅主播粉丝勋章" />
            </el-select>
          </el-form-item>
          <el-form-item label="关键词屏蔽" label-width="120px">
            <el-input 
              v-model="localForm.dmKeywordBlacklist" 
              type="textarea" 
              :rows="3"
              placeholder="每行一个关键词"
            />
            <div class="help-text">包含这些关键词的弹幕将被过滤</div>
          </el-form-item>
        </div>
      </el-form-item>
      
      <el-form-item label="文件处理">
        <el-select v-model="localForm.deleteType" style="width: 100%">
          <el-option :value="0" label="不删除/移动" />
          <el-option :value="1" label="录制完成后删除" />
          <el-option :value="2" label="录制完成后移动" />
          <el-option :value="3" label="上传完成后删除" />
          <el-option :value="4" label="上传完成后移动" />
          <el-option :value="5" label="上传完成后复制" />
          <el-option :value="6" label="上传完成后复制且30分钟后删除" />
          <el-option :value="7" label="立即删除" />
          <el-option :value="8" label="定时删除(每日凌晨)" />
          <el-option :value="9" label="投稿成功后删除" />
          <el-option :value="10" label="投稿成功后移动" />
          <el-option :value="11" label="投稿成功后复制" />
        </el-select>
        <div v-if="[2,4,5,6,10,11].includes(localForm.deleteType)" style="margin-top: 10px;">
          <el-input v-model="localForm.moveDir" placeholder="目标路径">
            <template #prepend>移动到</template>
          </el-input>
        </div>
      </el-form-item>
      
      <el-form-item label="分P标题模板">
        <el-input v-model="localForm.partTitleTemplate" />
        <div class="help-text">支持变量: ${index} ${MM月dd日HH点mm分} ${areaName} ${fileName}</div>
      </el-form-item>
      
      <el-form-item label="动态模板">
        <el-input v-model="localForm.dynamicTemplate" type="textarea" :rows="3" />
        <div class="help-text">投稿成功后发送动态，支持变量: ${uname} ${title} ${roomId} ${bvid}</div>
        <el-button size="small" @click="$emit('preview-template', localForm)" style="margin-top: 5px;">
          <el-icon><View /></el-icon>
          预览效果
        </el-button>
      </el-form-item>
      
      <el-form-item label="WxPusher UID">
        <el-input v-model="localForm.wxuid" placeholder="填写后将推送通知" />
        <div class="help-text">获取地址: https://wxpusher.zjiecode.com/</div>
      </el-form-item>
      
      <el-form-item label="推送消息类型">
        <el-checkbox-group v-model="pushTags">
          <el-checkbox label="开播">开播通知</el-checkbox>
          <el-checkbox label="上传">上传通知</el-checkbox>
          <el-checkbox label="投稿">投稿通知</el-checkbox>
        </el-checkbox-group>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button type="primary" @click="handleSave" :loading="saving">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Upload, View } from '@element-plus/icons-vue'
import LineSelector from './LineSelector.vue'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  title: {
    type: String,
    default: '编辑房间'
  },
  form: {
    type: Object,
    required: true
  },
  users: {
    type: Array,
    default: () => []
  },
  uploadLines: {
    type: Array,
    default: () => []
  },
  lineStats: {
    type: Object,
    default: () => ({})
  },
  lineSpeeds: {
    type: Object,
    default: () => ({})
  },
  testingLines: {
    type: Boolean,
    default: false
  },
  testingDeepSpeed: {
    type: Boolean,
    default: false
  },
  saving: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits([
  'update:visible',
  'save',
  'test-lines',
  'test-deep-speed',
  'preview-template'
])

const localForm = ref({ ...props.form })
const pushTags = ref([])

watch(() => props.form, (val) => {
  localForm.value = { ...val }
  pushTags.value = val.pushMsgTags ? val.pushMsgTags.split(',') : []
}, { deep: true })

const handleSave = () => {
  if (!localForm.value.roomId) {
    ElMessage.warning('请输入房间ID')
    return
  }
  
  localForm.value.pushMsgTags = pushTags.value.join(',')
  emit('save', localForm.value)
}

const handleCoverUploadSuccess = (response) => {
  if (response.code === 0) {
    localForm.value.coverUrl = response.data.url
    ElMessage.success('封面上传成功')
  } else {
    ElMessage.error(response.msg || '封面上传失败')
  }
}

const beforeCoverUpload = (file) => {
  const isImage = file.type.startsWith('image/')
  if (!isImage) {
    ElMessage.error('只能上传图片文件')
    return false
  }
  const isLt2M = file.size / 1024 / 1024 < 2
  if (!isLt2M) {
    ElMessage.error('图片大小不能超过2MB')
    return false
  }
  return true
}
</script>

<style scoped>
.help-text {
  font-size: 12px;
  color: #999;
  margin-top: 5px;
}
</style>
