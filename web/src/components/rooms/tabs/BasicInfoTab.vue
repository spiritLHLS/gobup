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
            :label="user.name"
            :value="user.id"
          />
        </el-select>
        <div class="help-text">选择用于上传视频的B站账号</div>
      </el-form-item>
      
      <el-divider content-position="left">自动化流程</el-divider>
      
      <el-alert
        title="自动化流程说明"
        type="info"
        :closable="false"
        style="margin-bottom: 20px"
      >
        <div style="line-height: 1.8">
          <strong>完整自动化流程：</strong>录制 → 自动上传 → 自动投稿 → 状态同步 → 审核通过后发送弹幕<br>
          <strong>SessionID合并：</strong>同一场直播的所有分P将自动合并到一个投稿，避免创建多个视频<br>
          <strong>推荐配置：</strong>全部开启（除了"自动发送弹幕"可根据需要选择）
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
      
      <el-form-item label="自动发送弹幕">
        <el-switch v-model="localForm.autoSendDanmaku" />
        <div class="help-text">🎯 审核通过后自动将弹幕发送到视频（需要先开启"定时同步信息"）</div>
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
          支持变量: ${uname} ${title} ${yyyy年MM月dd日HH点mm分} ${roomId} ${areaName}
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
      
      <el-form-item label="分区ID">
        <el-input-number 
          v-model="localForm.tid" 
          :min="1" 
          controls-position="right"
          style="width: 200px"
        />
        <div class="help-text">B站视频分区ID（21为日常分区）</div>
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
          支持变量: ${index} ${MM月dd日HH点mm分} ${areaName} ${fileName}
        </div>
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
