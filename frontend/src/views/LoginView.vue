<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { messageOf } from '@/api/client'
const mode = ref<'login' | 'register'>('login'), login = ref(''), username = ref(''), email = ref(''), password = ref(''), error = ref(''), busy = ref(false)
const router = useRouter(), auth = useAuthStore()
function switchMode(value: 'login' | 'register') { mode.value = value; error.value = ''; password.value = '' }
async function submit() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    if (mode.value === 'login') await auth.login(login.value.trim(), password.value)
    else await auth.register(username.value.trim(), email.value.trim(), password.value)
    await router.push('/exam408')
  } catch (e) { error.value = messageOf(e) } finally { busy.value = false }
}
</script>
<template>
  <main class="login-page"><section class="login-box">
    <header class="login-header"><span class="brand-mark">408</span><h1>408 Assistant</h1><p>你的考研学习空间</p></header>
    <div class="tabs" aria-label="登录方式"><button type="button" :disabled="busy" :class="{ active: mode === 'login' }" @click="switchMode('login')">登录</button><button type="button" :disabled="busy" :class="{ active: mode === 'register' }" @click="switchMode('register')">注册</button></div>
    <form class="login-form" @submit.prevent="submit"><h2>{{ mode === 'login' ? '欢迎回来' : '开启你的学习旅程' }}</h2><p class="muted">整理资料，理解知识，找到下一步方向。</p><div v-if="error" class="notice error" role="alert">{{ error }}</div>
      <template v-if="mode === 'register'"><div class="field"><label for="username">用户名</label><input id="username" v-model="username" required minlength="2" autocomplete="username" placeholder="你的昵称" /></div><div class="field"><label for="email">邮箱</label><input id="email" v-model="email" type="email" required autocomplete="email" placeholder="name@example.com" /></div></template>
      <div v-else class="field"><label for="login">用户名或邮箱</label><input id="login" v-model="login" required autocomplete="username" placeholder="用户名 / 邮箱" /></div>
      <div class="field"><label for="password">密码</label><input id="password" v-model="password" type="password" required minlength="8" :autocomplete="mode === 'login' ? 'current-password' : 'new-password'" placeholder="至少 8 位" /></div>
      <button class="primary full-width" :disabled="busy">{{ busy ? '请稍候…' : mode === 'login' ? '登录' : '注册并开始' }}</button>
    </form>
  </section><p class="login-caption">资料有出处，学习有方向。</p></main>
</template>
