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
import { User, Lock } from '@element-plus/icons-vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const router = useRouter()
const formRef = ref(null)
const errorMsg = ref('')
const loading = ref(false)

const form = reactive({
  username: localStorage.getItem('username') || '',
  password: ''
})

const rules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 2, message: '用户名至少2个字符', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 4, message: '密码至少4个字符', trigger: 'blur' }
  ]
}

const handleLogin = async () => {
  try {
    await formRef.value.validate()
    errorMsg.value = ''
    loading.value = true
    
    // 测试认证
    const response = await axios.get('/api/biliUser/list', {
      headers: {
        Authorization: 'Basic ' + btoa(form.username + ':' + form.password)
      }
    })
    
    // 认证成功，保存凭证
    localStorage.setItem('username', form.username)
    localStorage.setItem('password', form.password)
    
    ElMessage.success('登录成功，欢迎回来！')
    
    // 延迟一下再跳转，让用户看到成功提示
    setTimeout(() => {
      router.push('/dashboard')
    }, 500)
  } catch (error) {
    if (error.response?.status === 401) {
      errorMsg.value = '用户名或密码错误，请检查后重试'
    } else {
      errorMsg.value = error.message || '登录失败，请检查服务是否正常运行'
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
.login-container {
  min-height: 100vh;
  background: var(--bg-gradient-light);
  display: flex;
  flex-direction: column;
}

.login-header {
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(20px);
  box-shadow: 0 2px 20px rgba(22, 163, 74, 0.1);
  border-bottom: 1px solid rgba(22, 163, 74, 0.1);
}

.header-content {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 70px;
}

.logo h1 {
  font-size: 28px;
  color: var(--primary-color);
  margin: 0;
  font-weight: 700;
  background: var(--primary-gradient);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.login-main {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 24px;
}

.login-form-wrapper {
  width: 100%;
  max-width: 450px;
  animation: fadeIn 0.5s ease;
}

.login-card {
  background: rgba(255, 255, 255, 0.98);
  backdrop-filter: blur(10px);
  border-radius: var(--border-radius-2xl);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
  border: 1px solid rgba(22, 163, 74, 0.1);
  
  :deep(.el-card__body) {
    padding: 40px;
  }
}

.form-header {
  text-align: center;
  margin-bottom: 40px;
  
  h2 {
    font-size: var(--font-size-3xl);
    color: var(--text-color-primary);
    margin-bottom: 12px;
    font-weight: 700;
  }
  
  p {
    font-size: var(--font-size-base);
    color: var(--text-color-secondary);
    margin: 0;
  }
}

.el-form {
  .el-form-item {
    margin-bottom: 24px;
  }
  
  .login-button {
    width: 100%;
    height: 48px;
    font-size: var(--font-size-base);
    font-weight: var(--font-weight-semibold);
    margin-top: 8px;
  }
}

.error-alert {
  margin-top: 16px;
  border-radius: var(--border-radius-medium);
}

.login-tips {
  margin-top: 24px;
  
  .el-alert {
    background-color: #f0f9ff;
    border-color: #bae6fd;
    border-radius: var(--border-radius-medium);
  }
  
  .tips-content {
    p {
      margin: 4px 0;
      font-size: var(--font-size-sm);
      color: var(--text-color-secondary);
      line-height: 1.6;
      
      &:first-child {
        font-weight: var(--font-weight-semibold);
        color: var(--text-color-primary);
        margin-bottom: 8px;
      }
    }
  }
}

.login-footer {
  padding: 20px 24px;
  text-align: center;
  background: rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(10px);
  border-top: 1px solid rgba(22, 163, 74, 0.1);
  
  p {
    margin: 0;
    color: var(--text-color-secondary);
    font-size: var(--font-size-sm);
  }
}

/* 响应式设计 */
@media (max-width: 768px) {
  .login-main {
    padding: 24px 16px;
  }
  
  .login-card {
    :deep(.el-card__body) {
      padding: 32px 24px;
    }
  }
  
  .form-header h2 {
    font-size: var(--font-size-2xl);
  }
}

@media (max-width: 480px) {
  .header-content {
    padding: 0 16px;
    height: 60px;
  }
  
  .logo h1 {
    font-size: 24px;
  }
  
  .login-card {
    :deep(.el-card__body) {
      padding: 28px 20px;
    }
  }
  
  .form-header h2 {
    font-size: var(--font-size-xl);
  }
}
</style>
