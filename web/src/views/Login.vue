<template>
  <div class="login-container">
    <el-card class="login-card">
      <template #header>
        <div class="card-header">
          <h2>GoBup 登录</h2>
        </div>
      </template>
      
      <el-form :model="form" :rules="rules" ref="formRef" label-width="80px">
        <el-form-item label="用户名" prop="username">
          <el-input 
            v-model="form.username" 
            placeholder="请输入用户名"
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        
        <el-form-item label="密码" prop="password">
          <el-input 
            v-model="form.password" 
            type="password" 
            placeholder="请输入密码"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="handleLogin" style="width: 100%">
            登录
          </el-button>
        </el-form-item>
      </el-form>
      
      <el-alert 
        v-if="errorMsg" 
        :title="errorMsg" 
        type="error" 
        :closable="false"
        style="margin-top: 10px"
      />
      
      <div class="login-tips">
        <p>提示：请使用启动服务时设置的用户名和密码</p>
        <p>启动参数：-username 和 -password</p>
        <p>或环境变量：USERNAME 和 PASSWORD</p>
      </div>
    </el-card>
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
