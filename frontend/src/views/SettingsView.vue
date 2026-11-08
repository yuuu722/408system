<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {foundation,type Profile} from '@/api/foundation'
import {useAuthStore} from '@/stores/auth'
import {messageOf} from '@/api/client'
const auth=useAuthStore(),loading=ref(false),saving=ref(false),error=ref(''),notice=ref(''),profileReady=ref(false)
const profile=ref<Profile>({DisplayName:'',Bio:'',GoalScore:100,DailyMinutes:60,ExamDate:'',Stage:'beginner',Preference:'mixed'})
async function load(){
 loading.value=true;error.value='';notice.value='';profileReady.value=false
 try{const p=await foundation.profile();if(p)profile.value={...profile.value,...p};profileReady.value=true}catch(e){error.value='个人信息读取失败：'+messageOf(e)}
 loading.value=false
}
async function save(){
 if(!profileReady.value||saving.value)return
 saving.value=true;error.value='';notice.value=''
 try{await foundation.saveProfile(profile.value);notice.value='个人信息和学习目标已保存。自评与目标不会直接改变掌握度。'}catch(e){error.value=messageOf(e)}finally{saving.value=false}
}
onMounted(load)
</script>
<template><div>
<header class="page-head"><div><p class="eyebrow">MY LEARNING SPACE</p><h1>个人中心</h1><p>管理账号信息、学习目标和偏好；测评统计统一到测评学情查看。</p></div><button class="secondary" :disabled="loading||saving" @click="load">{{loading?'读取中…':'刷新数据'}}</button></header>
<p v-if="error" role="alert" class="notice error">{{error}}</p><p v-if="notice" role="status" class="notice">{{notice}}</p>
<div class="grid two-col">
<section class="card"><h2>个人信息与学习目标</h2><p class="reason">登录账号：{{auth.user?.username}} · {{auth.user?.email}} · {{auth.user?.role==='admin'?'管理员':'学生'}}。昵称不改变登录账号。</p>
<form @submit.prevent="save"><label class="field">昵称<input v-model="profile.DisplayName" maxlength="32" autocomplete="nickname" :disabled="!profileReady"/></label><label class="field">个人简介<textarea v-model="profile.Bio" maxlength="200" rows="3" :disabled="!profileReady" placeholder="记录你的学习方向和复习计划"/></label>
<div class="grid two-col"><label class="field">408目标分数<input v-model.number="profile.GoalScore" type="number" min="0" max="150" required/></label><label class="field">每天学习分钟数<input v-model.number="profile.DailyMinutes" type="number" min="10" max="600" required/></label><label class="field">考试日期（可暂不填写）<input v-model="profile.ExamDate" type="date"/></label><label class="field">复习阶段<select v-model="profile.Stage"><option value="beginner">基础学习</option><option value="review">强化复习</option><option value="sprint">冲刺</option></select></label><label class="field">资料偏好<select v-model="profile.Preference"><option value="text">教材文字</option><option value="video">视频</option><option value="mixed">均可</option></select></label></div>
<button class="primary" :disabled="loading||saving||!profileReady">{{saving?'保存中…':'保存个人信息'}}</button></form></section>
<section class="card section"><h2>学习功能入口</h2><p>学情、课堂和资料各自在对应分区维护，个人中心只管理你的基本信息与目标。</p><div class="toolbar-actions"><RouterLink class="primary" to="/exam408/assessment">测评学情</RouterLink><RouterLink class="secondary" to="/exam408/tasks">学习任务</RouterLink><RouterLink class="secondary" :to="{path:'/exam408/tasks',query:{tab:'teacher'},hash:'#learning-memory'}">管理老师记忆</RouterLink></div></section></div>
</div></template>
<style scoped>
.study-metrics{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.study-metrics div{padding:14px;background:#f5f8ff;border-radius:12px}.study-metrics dt{color:#64748b;font-size:14px}.study-metrics dd{margin:8px 0 0;font-size:26px;font-weight:700}.study-metrics small{font-size:13px;font-weight:400}.study-subject{padding:12px 0;border-bottom:1px solid #edf0f5}.study-subject>div{display:flex;justify-content:space-between}.toolbar-actions{display:flex;flex-wrap:wrap;gap:10px;margin-top:18px}textarea{width:100%;resize:vertical}
</style>
