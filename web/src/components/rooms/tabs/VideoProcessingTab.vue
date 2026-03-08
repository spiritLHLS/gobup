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
      
      <el-divider content-position="left">弹幕烧录</el-divider>
      
      <el-form-item label="启用弹幕烧录">
        <el-switch v-model="localForm.enableDanmakuBurn" />
        <div class="help-text">将弹幕烧录到视频中生成带弹幕版本（需要ffmpeg）</div>
      </el-form-item>
      
      <el-form-item label="自动更新投稿" v-if="localForm.enableDanmakuBurn">
        <el-switch v-model="localForm.autoUpdatePublished" />
        <div class="help-text important-config">
          ⭐ 弹幕版上传完成后，自动追加到已投稿视频（同时保留原版和弹幕版）
        </div>
      </el-form-item>

      <el-form-item label="弹幕烧录样式" v-if="localForm.enableDanmakuBurn">
        <el-select v-model="localForm.danmakuBurnStyle" style="width: 200px">
          <el-option value="default" label="默认样式" />
          <el-option value="compact" label="紧凑样式" />
          <el-option value="large" label="大字样式" />
        </el-select>
        <div class="help-text">控制烧录到视频中的弹幕字号和排列样式</div>
      </el-form-item>
      
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

      <el-form-item label="触发时机">
        <el-select v-model="localForm.fileOpTrigger" style="width: 100%">
          <el-option :value="0" label="不处理" />
          <el-option :value="1" label="分P上传完成后" />
          <el-option :value="2" label="全部分P上传完成、投稿前" />
          <el-option :value="3" label="投稿成功后（推荐）" />
          <el-option :value="4" label="审核通过后" />
        </el-select>
        <div class="help-text">推荐选择“投稿成功后”，投稿完成后源文件已在B站，可安全处理本地文件</div>
      </el-form-item>

      <template v-if="localForm.fileOpTrigger !== 0">
        <el-form-item label="操作类型">
          <el-select v-model="localForm.fileOpAction" style="width: 100%">
            <el-option :value="0" label="不处理" />
            <el-option :value="1" label="删除" />
            <el-option :value="2" label="移动" />
            <el-option :value="3" label="复制" />
          </el-select>
        </el-form-item>

        <template v-if="localForm.fileOpAction !== 0">
          <el-form-item label="操作范围">
            <el-checkbox-group
              :model-value="scopeChecked"
              @change="onScopeChange"
            >
              <el-checkbox :label="1">视频文件</el-checkbox>
              <el-checkbox :label="2">弹幕文件 (.xml)</el-checkbox>
              <el-checkbox :label="4">封面文件 (.jpg/.png)</el-checkbox>
            </el-checkbox-group>
            <div class="help-text">推荐仅勾选视频文件，保留弹幕和封面作为备份</div>
          </el-form-item>

          <el-form-item label="执行延迟">
            <el-select v-model="localForm.fileOpDelay" style="width: 200px">
              <el-option :value="0" label="立即执行" />
              <el-option :value="1" label="1 天后" />
              <el-option :value="2" label="2 天后" />
              <el-option :value="3" label="3 天后" />
              <el-option :value="5" label="5 天后" />
              <el-option :value="7" label="7 天后" />
              <el-option :value="14" label="14 天后" />
              <el-option :value="30" label="30 天后" />
            </el-select>
            <div class="help-text">选择延迟时可给弹幕烧录等后续处理留出足够时间</div>
          </el-form-item>

          <el-form-item
            v-if="localForm.fileOpAction === 2 || localForm.fileOpAction === 3"
            label="目标路径"
          >
            <el-input
              v-model="localForm.moveDir"
              placeholder="请输入移动/复制的目标路径"
            />
            <div class="help-text">文件将被移动或复制到此路径下的 &lt;房间ID&gt;/&lt;SessionID&gt;/ 子目录</div>
          </el-form-item>
        </template>
      </template>
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

// 将 fileOpScope 位掩码 ⇔ 复选框数组相互转换
const scopeChecked = computed(() => {
  const scope = localForm.value.fileOpScope ?? 0
  return [1, 2, 4].filter(v => (scope & v) !== 0)
})

function onScopeChange(arr) {
  const scope = arr.reduce((acc, v) => acc | v, 0)
  localForm.value = { ...localForm.value, fileOpScope: scope }
}
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
