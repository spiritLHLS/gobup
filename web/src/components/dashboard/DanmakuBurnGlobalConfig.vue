<template>
  <div class="form-section">
    <div class="section-title">弹幕烧录默认值（全局）</div>

    <el-form-item label="默认样式">
      <el-select v-model="config.danmakuBurnStyle" style="width: 220px">
        <el-option value="default" label="默认样式" />
        <el-option value="compact" label="紧凑样式" />
        <el-option value="large" label="大字样式" />
      </el-select>
      <span class="help-text">房间保持默认样式时继承此配置</span>
    </el-form-item>

    <el-form-item label="默认字号">
      <div class="number-input-wrapper">
        <el-input-number
          v-model="config.danmakuFontSize"
          :min="0"
          :max="72"
          :step="1"
          size="large"
        />
        <span class="help-text">0 表示跟随默认样式；房间设置字号后覆盖全局</span>
      </div>
    </el-form-item>

    <el-form-item label="默认颜色">
      <div class="button-group">
        <el-color-picker v-model="config.danmakuFontColor" />
        <el-button @click="config.danmakuFontColor = ''">保留原色</el-button>
        <span class="help-text">留空则保留原弹幕颜色；房间设置颜色后覆盖全局</span>
      </div>
    </el-form-item>

    <el-form-item label="滚动区域">
      <el-slider
        v-model="config.danmakuScrollArea"
        :min="0.1"
        :max="1"
        :step="0.05"
        show-input
        style="width: 420px"
      />
    </el-form-item>

    <el-form-item label="显示区域">
      <el-slider
        v-model="config.danmakuDisplayArea"
        :min="0.1"
        :max="1"
        :step="0.05"
        show-input
        style="width: 420px"
      />
    </el-form-item>
  </div>
</template>

<script setup>
defineProps({
  config: {
    type: Object,
    required: true
  }
})
</script>
