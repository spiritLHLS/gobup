<template>
  <div class="captcha-container">
    <el-card class="captcha-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <span><i class="el-icon-warning"></i> B站验证码处理</span>
        </div>
      </template>

      <div v-if="!status.required" class="no-captcha">
        <el-empty description="当前无需验证码">
          <template #image>
            <i class="el-icon-success" style="font-size: 100px; color: #67C23A;"></i>
          </template>
        </el-empty>
      </div>

      <div v-else class="captcha-content">
        <el-alert
          title="检测到上传需要验证码"
          type="warning"
          :closable="false"
          show-icon
          style="margin-bottom: 20px;">
          <template #default>
            <p>文件名: <strong>{{ status.filename }}</strong></p>
            <p>Voucher: <code>{{ status.voucher }}</code></p>
            <p v-if="status.extra">详细信息: {{ JSON.stringify(status.extra) }}</p>
          </template>
        </el-alert>

        <el-tabs v-model="activeTab" type="border-card">
          <!-- 方法1: 使用脚本自动捕获 -->
          <el-tab-pane label="方法1: 自动捕获（推荐）" name="auto">
            <div class="method-content">
              <el-steps :active="currentStep" align-center finish-status="success">
                <el-step title="复制脚本"></el-step>
                <el-step title="打开投稿页"></el-step>
                <el-step title="运行脚本"></el-step>
                <el-step title="完成验证"></el-step>
              </el-steps>

              <div class="step-content">
                <div v-if="currentStep === 0" class="step-1">
                  <p class="step-desc">1. 点击下方按钮复制脚本到剪贴板</p>
                  <el-input
                    type="textarea"
                    :rows="10"
                    v-model="hookScript"
                    readonly
                    class="script-area">
                  </el-input>
                  <el-button type="primary" @click="copyScript" icon="el-icon-document-copy">
                    复制脚本
                  </el-button>
                  <el-button @click="currentStep = 1">下一步</el-button>
                </div>

                <div v-if="currentStep === 1" class="step-2">
                  <p class="step-desc">2. 在新标签页打开 B站投稿页面</p>
                  <el-button type="primary" @click="openBiliUpload" icon="el-icon-link">
                    打开 B站投稿页
                  </el-button>
                  <el-button @click="currentStep = 2">下一步</el-button>
                  <el-button @click="currentStep = 0" plain>上一步</el-button>
                </div>

                <div v-if="currentStep === 2" class="step-3">
                  <p class="step-desc">3. 在投稿页面按 F12 打开开发者工具，切换到 Console 标签，粘贴脚本并按回车</p>
                  <el-alert type="info" :closable="false">
                    脚本会自动监听B站的验证码请求，当出现验证码时会自动捕获 Token
                  </el-alert>
                  <el-button type="primary" @click="currentStep = 3" style="margin-top: 15px;">
                    已运行脚本，继续
                  </el-button>
                  <el-button @click="currentStep = 1" plain>上一步</el-button>
                </div>

                <div v-if="currentStep === 3" class="step-4">
                  <p class="step-desc">4. 上传视频触发验证码，完成验证后脚本会自动提交</p>
                  <el-alert type="success" :closable="false" show-icon>
                    <template #title>
                      <span>验证码提交后，本页面会自动检测并继续上传流程</span>
                    </template>
                  </el-alert>
                  <div style="margin-top: 15px;">
                    <el-button type="warning" @click="clearCaptcha" icon="el-icon-refresh-left">
                      重新开始
                    </el-button>
                    <el-button @click="currentStep = 2" plain>上一步</el-button>
                  </div>
                </div>
              </div>
            </div>
          </el-tab-pane>

          <!-- 方法2: 手动输入 -->
          <el-tab-pane label="方法2: 手动输入" name="manual">
            <div class="method-content">
              <el-alert
                title="手动输入验证码 Token"
                type="info"
                :closable="false"
                style="margin-bottom: 15px;">
                如果自动捕获失败，可以手动输入验证码返回的 JSON 数据
              </el-alert>

              <el-form label-width="120px">
                <el-form-item label="验证码 JSON">
                  <el-input
                    type="textarea"
                    :rows="6"
                    v-model="manualJson"
                    placeholder='例如: {"captcha_token":"xxxxx"}'>
                  </el-input>
                </el-form-item>

                <el-form-item>
                  <el-button type="primary" @click="submitManualCaptcha" :loading="submitting">
                    提交验证码
                  </el-button>
                  <el-button @click="manualJson = ''">清空</el-button>
                </el-form-item>
              </el-form>
            </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </el-card>
  </div>
</template>

<script>
import axios from '../api/index.js'

export default {
  name: 'Captcha',
  data() {
    return {
      status: {
        required: false,
        voucher: '',
        filename: '',
        extra: null,
        timestamp: 0
      },
      activeTab: 'auto',
      currentStep: 0,
      manualJson: '',
      submitting: false,
      timer: null,
      hookScript: ''
    }
  },
  mounted() {
    this.generateHookScript()
    this.checkStatus()
    this.timer = setInterval(this.checkStatus, 3000) // 每3秒检查一次
  },
  beforeUnmount() {
    if (this.timer) {
      clearInterval(this.timer)
    }
  },
  methods: {
    generateHookScript() {
      const serverUrl = window.location.origin
      this.hookScript = `(function(){
    var targetUrl = "${serverUrl}/api/captcha/submit";
    console.log("正在监听 B站验证码请求...");
    
    // 监听 fetch
    var originalFetch = window.fetch;
    window.fetch = function(input, init) {
        if (typeof input === 'string' && (input.includes('add/v3') || input.includes('validate'))) {
            try {
                var token = null;
                if (input.includes('captcha_token=')) {
                    token = input.match(/captcha_token=([^&]+)/)[1];
                } else if (init && init.body) {
                    if (init.body.includes('captcha_token')) {
                        var body = JSON.parse(init.body);
                        token = body.captcha_token;
                    }
                }
                
                if (token) {
                    console.log("捕获到 Token: " + token);
                    var data = { captcha_token: token };
                    
                    // 自动发送到服务器
                    fetch(targetUrl, {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify(data)
                    }).then(function(res) {
                        console.log("验证码已自动提交到服务器");
                        alert("验证码已自动提交！");
                    }).catch(function(err) {
                        console.error("提交失败:", err);
                        alert("验证码提交失败，请手动复制: " + JSON.stringify(data));
                    });
                }
            } catch(e) { console.error(e); }
        }
        return originalFetch.apply(this, arguments);
    };
    
    alert("脚本注入成功！请现在上传视频触发验证。");
})();`
    },
    async checkStatus() {
      try {
        const res = await axios.get('/api/captcha/status')
        
        // 如果之前需要验证码，现在不需要了，说明已经成功
        if (this.status.required && !res.required) {
          this.$message.success('验证码已成功提交，上传流程继续')
          this.currentStep = 0
        }
        
        this.status = res
      } catch (error) {
        console.error('检查验证码状态失败:', error)
      }
    },
    copyScript() {
      const el = document.createElement('textarea')
      el.value = this.hookScript
      document.body.appendChild(el)
      el.select()
      document.execCommand('copy')
      document.body.removeChild(el)
      this.$message.success('脚本已复制到剪贴板')
    },
    openBiliUpload() {
      window.open('https://member.bilibili.com/platform/upload/video/frame', '_blank')
      this.$message.info('请在新页面中按 F12 打开开发者工具')
    },
    async submitManualCaptcha() {
      if (!this.manualJson.trim()) {
        this.$message.warning('请输入验证码 JSON')
        return
      }

      try {
        this.submitting = true
        const data = JSON.parse(this.manualJson)
        
        await axios.post('/api/captcha/submit', data)
        
        this.$message.success('验证码已提交')
        this.manualJson = ''
        await this.checkStatus()
      } catch (error) {
        console.error('提交验证码失败:', error)
        this.$message.error('提交失败: ' + (error.response?.data?.message || error.message))
      } finally {
        this.submitting = false
      }
    },
    async clearCaptcha() {
      try {
        await axios.post('/api/captcha/clear')
        this.$message.success('验证码状态已清除')
        this.currentStep = 0
        await this.checkStatus()
      } catch (error) {
        console.error('清除状态失败:', error)
      }
    }
  }
}
</script>

<style scoped>
.captcha-container {
  padding: 20px;
  max-width: 1200px;
  margin: 0 auto;
}

.captcha-card {
  min-height: 500px;
}

.card-header {
  display: flex;
  align-items: center;
  font-size: 18px;
  font-weight: bold;
}

.card-header i {
  margin-right: 8px;
  color: #E6A23C;
}

.no-captcha {
  padding: 60px 0;
  text-align: center;
}

.captcha-content {
  padding: 20px;
}

.method-content {
  padding: 20px;
}

.step-content {
  margin-top: 30px;
}

.step-desc {
  font-size: 16px;
  color: #409EFF;
  margin-bottom: 15px;
  font-weight: bold;
}

.script-area {
  margin: 15px 0;
  font-family: 'Courier New', monospace;
  font-size: 12px;
}

code {
  background: #f5f5f5;
  padding: 2px 6px;
  border-radius: 3px;
  font-family: monospace;
}

.el-steps {
  margin-bottom: 30px;
}

.el-button {
  margin-right: 10px;
}
</style>
