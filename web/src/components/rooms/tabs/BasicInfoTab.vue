<template>
  <div class="basic-info-tab">
    <el-form :model="localForm" label-width="120px">
      <el-form-item label="房间ID" required>
        <el-input 
          v-model="localForm.roomId" 
          placeholder="请输入B站直播间房间号"
        />
      </el-form-item>
      
      <el-form-item label="是否上传">
        <el-switch v-model="localForm.upload" />
        <div class="help-text">开启后将自动上传录制的视频到B站</div>
      </el-form-item>
      
      <el-form-item label="自动上传分P">
        <el-switch v-model="localForm.autoUpload" />
        <div class="help-text">开启后，录制完成的分P将自动上传到B站（需要先开启"是否上传"）</div>
      </el-form-item>
      
      <el-form-item label="自动投稿">
        <el-switch v-model="localForm.autoPublish" />
        <div class="help-text">开启后，所有分P上传完成后将自动提交投稿</div>
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

:deep(.el-divider__text) {
  font-weight: 500;
  color: #303133;
}
</style>
