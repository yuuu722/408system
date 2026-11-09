<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { DashboardIcon, TreeListIcon, FolderOpenIcon, ChatIcon, ChartBubbleIcon } from 'tdesign-icons-vue-next'
const route = useRoute(), router = useRouter(), auth = useAuthStore()
const studentItems = [
 {path:'/exam408',label:'学习概览',icon:DashboardIcon},
 {path:'/exam408/assessment',label:'测评与学情',icon:ChartBubbleIcon},
 {path:'/exam408/weakness',label:'薄弱点复习',icon:TreeListIcon},
 {path:'/exam408/tasks',label:'学习任务',icon:TreeListIcon},
 {path:'/exam408/assets',label:'题库课程',icon:FolderOpenIcon},
 {path:'/exam408/knowledge',label:'知识体系',icon:TreeListIcon},
 {path:'/exam408/assistant',label:'智能答疑',icon:ChatIcon},
 {path:'/exam408/resources',label:'资料知识库',icon:FolderOpenIcon},
]

const items=computed(()=>auth.user?.role==='admin'?[
 {path:'/exam408/admin',label:'管理概览',icon:DashboardIcon},
 {path:'/exam408/tasks',label:'学习任务体验',icon:TreeListIcon},
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
  <div class="shell" :class="{'shell-assistant':route.path==='/exam408/assistant'}">
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <header class="topbar">
      <RouterLink to="/exam408" class="brand"><span class="brand-mark">408</span><b>408 Assistant</b></RouterLink>
      <nav class="main-navigation" aria-label="主导航"><RouterLink v-for="item in items" :key="item.path" :to="item.path" :class="{ active: route.path === item.path }" :aria-current="route.path === item.path ? 'page' : undefined"><component :is="item.icon" size="18" />{{ item.label }}</RouterLink></nav>
      <div class="account-menu"><RouterLink to="/exam408/settings" class="account-link" aria-label="个人中心"><span class="account"><b>{{ auth.user?.username || '同学' }}</b><small>{{ !auth.user ? '正在读取身份…' : auth.user.role === 'admin' ? '管理员 · 公共资源维护' : '个人中心' }}</small></span><span class="avatar">{{ auth.user?.username?.slice(0, 1).toUpperCase() || 'U' }}</span></RouterLink><button class="text-btn" @click="logout">退出</button></div>
    </header>
    <main id="main-content" class="main" :class="{'main-assistant':route.path==='/exam408/assistant','main-wide':['/exam408/assessment','/exam408/weakness','/exam408/statistics'].includes(route.path)}" tabindex="-1"><RouterView /></main>
    <footer v-if="route.path!=='/exam408/assistant'" class="site-footer">408 Assistant · 让资料、理解与复习连接起来 · 版本 2026.11.09.2</footer>
  </div>
</template>

<style scoped>
.shell-assistant{height:100dvh;min-height:0;overflow:hidden}.shell-assistant>.topbar{flex-shrink:0}.main.main-assistant{display:flex;flex:1;min-height:0;max-width:none;padding:0;margin:0}
</style>

<style scoped>
.topbar{gap:18px;padding-left:24px;padding-right:24px}
.main-navigation{min-width:0;flex:1;flex-wrap:nowrap;overflow-x:auto;scrollbar-width:thin;white-space:nowrap}
.main-navigation>a{flex:1 0 auto;justify-content:center;padding:10px 9px;font-size:14px}
.main.main-wide{max-width:none}
@media(max-width:1280px) and (min-width:1101px){.brand b{display:none}.main-navigation>a{font-size:13px;padding:10px 7px}.account-menu{gap:8px}}
@media(max-width:1100px){.topbar{flex-wrap:wrap}.main-navigation{order:3;flex-basis:100%;justify-content:flex-start}.account-menu{margin-left:auto}.main-navigation>a{flex:1 0 auto}}
</style>
