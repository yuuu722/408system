<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { DashboardIcon, TreeListIcon, FolderOpenIcon, ChatIcon, ChartBubbleIcon } from 'tdesign-icons-vue-next'
const route = useRoute(), router = useRouter(), auth = useAuthStore()
const studentItems = [
  { path: '/exam408/assessment', label: '测评学情', icon: ChartBubbleIcon },
  { path: '/exam408/tasks', label: '学习任务', icon: TreeListIcon },
  { path: '/exam408/assets', label: '题目与资源', icon: FolderOpenIcon },
  { path: '/exam408', label: '学情分析', icon: DashboardIcon },
  { path: '/exam408/assistant', label: '智能答疑', icon: ChatIcon },
  { path: '/exam408/resources', label: '我的知识库与公共资料', icon: FolderOpenIcon },
  { path: '/exam408/knowledge', label: '知识体系', icon: TreeListIcon },
  { path: '/exam408/graph', label: '知识图谱', icon: TreeListIcon },
  { path: '/exam408/weakness', label: '薄弱点复习', icon: ChartBubbleIcon },
]
const items=computed(()=>auth.user?.role==='admin'?[
 {path:'/exam408/admin',label:'管理概览',icon:DashboardIcon},
 {path:'/exam408/admin/resources',label:'公共资料管理',icon:FolderOpenIcon},
 {path:'/exam408/admin/knowledge',label:'考纲目录管理',icon:TreeListIcon},
 {path:'/exam408/admin/assets',label:'题库与课程审核',icon:FolderOpenIcon},
 {path:'/exam408/admin/graph',label:'知识图谱',icon:TreeListIcon},
 {path:'/exam408/admin/users',label:'用户管理',icon:ChartBubbleIcon},
 {path:'/exam408/assistant',label:'智能答疑与连接检测',icon:ChatIcon},
]:studentItems)
function logout() { auth.logout(); router.push('/login') }
</script>
<template>
  <div class="shell">
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <header class="topbar">
      <RouterLink to="/exam408" class="brand"><span class="brand-mark">408</span><b>408 Assistant</b></RouterLink>
      <nav aria-label="主导航"><RouterLink v-for="item in items" :key="item.path" :to="item.path" :class="{ active: route.path === item.path }" :aria-current="route.path === item.path ? 'page' : undefined"><component :is="item.icon" size="18" />{{ item.label }}</RouterLink></nav>
      <div class="account-menu"><RouterLink to="/exam408/settings" class="account-link" aria-label="账号设置"><span class="account"><b>{{ auth.user?.username || '同学' }}</b><small>{{ !auth.user ? '正在读取身份…' : auth.user.role === 'admin' ? '管理员 · 公共资源维护' : '普通用户 · 公共学习空间' }}</small></span><span class="avatar">{{ auth.user?.username?.slice(0, 1).toUpperCase() || 'U' }}</span></RouterLink><button class="text-btn" @click="logout">退出</button></div>
    </header>
    <main id="main-content" class="main" tabindex="-1"><RouterView /></main>
    <footer class="site-footer">408 Assistant · 让资料、理解与复习连接起来 · 版本 2026.10.31.2</footer>
  </div>
</template>
