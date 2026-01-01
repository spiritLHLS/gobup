<template>
  <div class="login-container">
    <!-- 页面头部 -->
    <div class="login-header">
      <div class="header-content">
        <div class="logo">
          <h1>GoBup</h1>
        </div>
      </div>
    </div>

    <!-- 登录主体 -->
    <div class="login-main">
      <div class="login-form-wrapper">
        <el-card class="login-card">
          <div class="form-header">
            <h2>欢迎回来</h2>
            <p>登录 GoBup 直播录制管理系统</p>
          </div>
          
          <el-form 
            ref="formRef" 
            :model="form" 
            :rules="rules" 
            size="large"
            @keyup.enter="handleLogin"
          >
            <el-form-item prop="username">
              <el-input 
                v-model="form.username" 
                placeholder="请输入用户名"
                :prefix-icon="User"
                clearable
              />
            </el-form-item>
            
            <el-form-item prop="password">
              <el-input 
                v-model="form.password" 
                type="password" 
                placeholder="请输入密码"
                :prefix-icon="Lock"
                show-password
                clearable
              />
            </el-form-item>
            
            <el-form-item>
              <el-button 
                type="primary" 
                :loading="loading"
                @click="handleLogin" 
                class="login-button"
              >
                {{ loading ? '登录中...' : '登录' }}
              </el-button>
            </el-form-item>
          </el-form>
          
          <el-alert 
            v-if="errorMsg" 
            :title="errorMsg" 
            type="error" 
            :closable="true"
            @close="errorMsg = ''"
            class="error-alert"
          />
          
          <div class="login-tips">
            <el-alert
              type="info"
              :closable="false"
            >
              <template #title>
                <div class="tips-content">
                  <p><strong>提示：</strong></p>
                  <p>• 使用启动服务时设置的用户名和密码</p>
                  <p>• 启动参数：-username 和 -password</p>
                  <p>• 或环境变量：USERNAME 和 PASSWORD</p>
                </div>
              </template>
            </el-alert>
          </div>
        </el-card>
      </div>
    </div>

    <!-- 页面底部 -->
    <div class="login-footer">
      <p>© 2024 GoBup. All rights reserved.</p>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const router = useRouter()
const formRef = ref(null)
const errorMsg = ref('')

const form = reactive({
  username: localStorage.getItem('username') || '',
  password: ''
})

const rules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' }
  ]
}

const handleLogin = async () => {
  try {
    await formRef.value.validate()
    errorMsg.value = ''
    
    // 测试认证
    const response = await axios.get('/api/biliUser/list', {
      headers: {
        Authorization: 'Basic ' + btoa(form.username + ':' + form.password)
      }
    })
    
    // 认证成功，保存凭证
    localStorage.setItem('username', form.username)
    localStorage.setItem('password', form.password)
    
    ElMessage.success('登录成功')
    router.push('/rooms')
  } catch (error) {
    if (error.response?.status === 401) {
      errorMsg.value = '用户名或密码错误'
    } else {
      errorMsg.value = error.message || '登录失败，请检查服务是否正常'
    }
  }
}
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 400px;
  border-radius: 10px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
}

.card-header {
  text-align: center;
}

.card-header h2 {
  margin: 0;
  color: #333;
  font-size: 24px;
}

.login-tips {
  margin-top: 20px;
  padding: 15px;
  background-color: #f5f7fa;
  border-radius: 4px;
  font-size: 12px;
  color: #909399;
  line-height: 1.8;
}

.login-tips p {
  margin: 5px 0;
}
</style>
