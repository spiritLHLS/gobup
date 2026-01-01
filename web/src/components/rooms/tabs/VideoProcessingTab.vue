<template>
  <div class="video-processing-tab">
    <el-form :model="localForm" label-width="120px">
      <el-divider content-position="left">高能剪辑</el-divider>
      
      <el-form-item label="启用高能剪辑">
        <el-switch v-model="localForm.highEnergyCut" />
        <div class="help-text">基于弹幕密度自动剪辑高能片段（需要ffmpeg）</div>
      </el-form-item>
      
      <template v-if="localForm.highEnergyCut">
        <el-form-item label="窗口大小">
          <el-input-number 
            v-model="localForm.windowSize" 
            :min="10" 
            :max="300"
            controls-position="right"
            style="width: 200px"
          />
          <span style="margin-left: 10px;">秒</span>
          <div class="help-text">分析弹幕密度的时间窗口大小</div>
        </el-form-item>
        
        <el-form-item label="阈值百分位">
          <el-input-number 
            v-model="localForm.percentileRank" 
            :min="50" 
            :max="99"
            controls-position="right"
            style="width: 200px"
          />
          <span style="margin-left: 10px;">%</span>
          <div class="help-text">值越大，筛选越严格（推荐75）</div>
        </el-form-item>
        
        <el-form-item label="最小片段">
          <el-input-number 
            v-model="localForm.minSegmentDuration" 
            :min="5" 
            :max="60"
            controls-position="right"
            style="width: 200px"
          />
          <span style="margin-left: 10px;">秒</span>
          <div class="help-text">剪辑片段的最小长度</div>
        </el-form-item>
      </template>
      
      <el-divider content-position="left">弹幕过滤</el-divider>
      
      <el-form-item label="去除重复弹幕">
        <el-switch v-model="localForm.dmDistinct" />
      </el-form-item>
      
      <el-form-item label="最低用户等级">
        <el-input-number 
          v-model="localForm.dmUlLevel" 
          :min="0" 
          :max="6"
          controls-position="right"
          style="width: 200px"
        />
        <div class="help-text">0表示不过滤，1-6对应B站用户等级</div>
      </el-form-item>
      
      <el-form-item label="粉丝勋章过滤">
        <el-select 
          v-model="localForm.dmMedalLevel"
          style="width: 300px"
        >
          <el-option :value="0" label="不过滤" />
          <el-option :value="1" label="仅保留佩戴粉丝勋章的用户" />
          <el-option :value="2" label="仅保留主播粉丝勋章的用户" />
        </el-select>
      </el-form-item>
      
      <el-form-item label="关键词屏蔽">
        <el-input 
          v-model="localForm.dmKeywordBlacklist" 
          type="textarea" 
          :rows="4"
          placeholder="每行一个关键词，包含这些关键词的弹幕将被过滤"
        />
        <div class="help-text">支持正则表达式，一行一个关键词</div>
      </el-form-item>
      
      <el-divider content-position="left">文件处理</el-divider>
      
      <el-form-item label="处理方式">
        <el-select v-model="localForm.deleteType" style="width: 100%">
          <el-option :value="0" label="0 - 不处理" />
          <el-option :value="1" label="1 - 上传前删除" />
          <el-option :value="2" label="2 - 上传前移动" />
          <el-option :value="3" label="3 - 上传后删除" />
          <el-option :value="4" label="4 - 上传后移动" />
          <el-option :value="5" label="5 - 上传前复制" />
          <el-option :value="6" label="6 - 上传后复制" />
          <el-option :value="7" label="7 - 上传完成后立即删除" />
          <el-option :value="8" label="8 - N天后删除移动" />
          <el-option :value="9" label="9 - 投稿成功后删除（推荐）" />
          <el-option :value="10" label="10 - 投稿成功后移动" />
          <el-option :value="11" label="11 - 审核通过后复制" />
          <el-option :value="12" label="12 - 审核通过后删除" />
        </el-select>
        <div class="help-text">
          推荐使用"9-投稿成功后删除"，只删除视频文件，保留弹幕和封面
        </div>
      </el-form-item>
      
      <el-form-item 
        v-if="[2, 4, 6, 10, 11].includes(localForm.deleteType)" 
        label="目标路径"
      >
        <el-input 
          v-model="localForm.moveDir" 
          placeholder="请输入移动/复制的目标路径"
        />
        <div class="help-text">文件将被移动或复制到此路径</div>
      </el-form-item>
      
      <el-form-item 
        v-if="localForm.deleteType === 8" 
        label="延迟天数"
      >
        <el-input-number 
          v-model="localForm.deleteDay" 
          :min="1" 
          :max="30"
          controls-position="right"
          style="width: 200px"
        />
        <span style="margin-left: 10px;">天后删除移动</span>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup>
import { computed } from 'vue'

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
</script>

<style scoped>
.video-processing-tab {
  padding: 20px 0;
}

.help-text {
  font-size: 12px;
  color: #909399;
  margin-top: 5px;
  line-height: 1.5;
}

:deep(.el-divider__text) {
  font-weight: 500;
  color: #303133;
}
</style>
