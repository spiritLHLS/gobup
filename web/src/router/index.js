import { createRouter, createWebHistory } from 'vue-router'
import Layout from '@/views/Layout.vue'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: '登录', requiresAuth: false }
  },
  {
    path: '/',
    component: Layout,
    redirect: '/dashboard',
    meta: { requiresAuth: true },
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard.vue'),
        meta: { title: '控制面板', requiresAuth: true }
      },
      {
        path: 'rooms',
        name: 'Rooms',
        component: () => import('@/views/Rooms.vue'),
        meta: { title: '房间管理', requiresAuth: true }
      },
      {
        path: 'history',
        name: 'History',
        component: () => import('@/views/History.vue'),
        meta: { title: '录制历史', requiresAuth: true }
      },
      {
        path: 'users',
        name: 'Users',
        component: () => import('@/views/Users.vue'),
        meta: { title: '用户管理', requiresAuth: true }
      },
      {
        path: 'logs',
        name: 'Logs',
        component: () => import('@/views/Log.vue'),
        meta: { title: '系统日志', requiresAuth: true }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// 路由守卫：检查认证
router.beforeEach((to, from, next) => {
  const username = localStorage.getItem('username')
  const password = localStorage.getItem('password')
  
  // 如果需要认证且没有凭证，跳转到登录页
  if (to.meta.requiresAuth !== false && (!username || !password)) {
    if (to.path !== '/login') {
      next('/login')
    } else {
      next()
    }
  } else if (to.path === '/login' && username && password) {
    // 已登录用户访问登录页，跳转到首页
    next('/dashboard')
  } else {
    next()
  }
})

export default router
