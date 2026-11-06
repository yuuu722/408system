<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {foundation,type Profile,type StudySummary} from '@/api/foundation'
import {learningApi} from '@/api/learning'
import {useAuthStore} from '@/stores/auth'
import {messageOf} from '@/api/client'
const auth=useAuthStore(),loading=ref(false),saving=ref(false),error=ref(''),notice=ref(''),profileReady=ref(false)
const profile=ref<Profile>({DisplayName:'',Bio:'',GoalScore:100,DailyMinutes:60,ExamDate:'',Stage:'beginner',Preference:'mixed'})
const summary=ref<StudySummary|null>(null),modelStatus=ref('正在读取答疑状态…')
const coverage=computed(()=>summary.value?.TotalPoints?Math.round(summary.value.CoveredPoints/summary.value.TotalPoints*100):0)
const kinds:Record<string,string>={diagnostic:'诊断测评',practice:'知识点练习',retest:'复测'}
async function load(){
 loading.value=true;error.value='';notice.value='';profileReady.value=false
 const values=await Promise.allSettled([foundation.profile(),foundation.studySummary(),learningApi.assistantStatus()])
 const p=values[0],s=values[1],m=values[2]
 if(p.status==='fulfilled'){if(p.value)profile.value={...profile.value,...p.value};profileReady.value=true}else error.value='个人信息读取失败：'+messageOf(p.reason)
 if(s.status==='fulfilled')summary.value=s.value;else{summary.value=null;error.value+=(error.value?'；':'')+'学情读取失败：'+messageOf(s.reason)}
 modelStatus.value=m.status==='fulfilled'?m.value.message:'答疑状态暂时无法获取'
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
<header class="page-head"><div><p class="eyebrow">MY LEARNING SPACE</p><h1>个人中心</h1><p>管理个人信息，查看真实练习记录和下一步学习方向。</p></div><button class="secondary" :disabled="loading||saving" @click="load">{{loading?'读取中…':'刷新数据'}}</button></header>
<p v-if="error" role="alert" class="notice error">{{error}}</p><p v-if="notice" role="status" class="notice">{{notice}}</p>
<div class="grid two-col">
<section class="card"><h2>个人信息与学习目标</h2><p class="reason">登录账号：{{auth.user?.username}} · {{auth.user?.email}} · {{auth.user?.role==='admin'?'管理员':'学生'}}。昵称不改变登录账号。</p>
<form @submit.prevent="save"><label class="field">昵称<input v-model="profile.DisplayName" maxlength="32" autocomplete="nickname" :disabled="!profileReady"/></label><label class="field">个人简介<textarea v-model="profile.Bio" maxlength="200" rows="3" :disabled="!profileReady" placeholder="记录你的学习方向和复习计划"/></label>
<div class="grid two-col"><label class="field">408目标分数<input v-model.number="profile.GoalScore" type="number" min="0" max="150" required/></label><label class="field">每天学习分钟数<input v-model.number="profile.DailyMinutes" type="number" min="10" max="600" required/></label><label class="field">考试日期（可暂不填写）<input v-model="profile.ExamDate" type="date"/></label><label class="field">复习阶段<select v-model="profile.Stage"><option value="beginner">基础学习</option><option value="review">强化复习</option><option value="sprint">冲刺</option></select></label><label class="field">资料偏好<select v-model="profile.Preference"><option value="text">教材文字</option><option value="video">视频</option><option value="mixed">均可</option></select></label></div>
<button class="primary" :disabled="loading||saving||!profileReady">{{saving?'保存中…':'保存个人信息'}}</button></form></section>
<section class="card"><h2>学情检测</h2><template v-if="summary"><p class="notice">{{summary.Notice}}</p><dl class="study-metrics"><div><dt>已完成测评</dt><dd>{{summary.Submitted}}</dd></div><div><dt>有效题族</dt><dd>{{summary.ValidFamilies}}</dd></div><div><dt>有效作答正确率</dt><dd>{{summary.Accuracy===null?'待测':Math.round(summary.Accuracy*100)+'%'}}</dd></div><div><dt>已录入目录覆盖率</dt><dd>{{coverage}}%<small>（{{summary.CoveredPoints}} / {{summary.TotalPoints}}）</small></dd></div></dl><p class="reason">未完成测评 {{summary.Unsubmitted}} 次 · 待确认或未计入 {{summary.UncountedAttempts}} 题次。重复题族按最近有效作答统计，不累加刷题次数。</p>
<div v-for="s in summary.Subjects" :key="s.ID" class="study-subject"><div><b>{{s.Name}}</b><span>{{s.Accuracy===null?'待测':Math.round(s.Accuracy*100)+'%'}}</span></div><p class="reason">有效题族 {{s.Tested}} · 答对 {{s.Correct}} · 知识点 {{s.CoveredPoints}} / {{s.TotalPoints}} · {{s.Tested===0?'尚无有效作答':s.Tested<5?'样本较少，暂不判断整体水平':'已有作答样本，仍需结合覆盖范围判断'}}</p></div>
</template><p v-else class="reason">{{loading?'正在读取学情…':'学情暂不可用，请刷新重试。'}}</p>
<div class="toolbar-actions"><RouterLink class="primary" to="/exam408/assessment">进入测评与练习</RouterLink><RouterLink class="secondary" to="/exam408/weakness">查看薄弱点</RouterLink><RouterLink class="secondary" to="/exam408/tasks">学习任务</RouterLink></div>
</section></div>
<section v-if="summary" class="card section"><h2>最近测评</h2><p v-if="!summary.Recent.length" class="empty">尚无测评记录。先完成资料有据的练习，再查看学情。</p><div v-for="a in summary.Recent" :key="a.ID" class="list-item"><div class="grow"><b>{{kinds[a.Kind]||'测评'}}</b><p class="reason">{{new Date(a.CreatedAt).toLocaleString()}} · {{a.SubmittedAt?'已提交':'尚未提交'}}<template v-if="a.SubmittedAt"> · 共 {{a.Total}} 题，计入 {{a.Counted}} 题，答对 {{a.Correct}} 题</template></p></div><RouterLink class="secondary" :to="{path:'/exam408/assessment',query:{assessment:a.ID}}">{{a.SubmittedAt?'查看结果':'继续作答'}}</RouterLink></div></section>
<section class="card section"><h2>智能答疑状态</h2><p>{{modelStatus}}</p></section>
</div></template>
<style scoped>
.study-metrics{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.study-metrics div{padding:14px;background:#f5f8ff;border-radius:12px}.study-metrics dt{color:#64748b;font-size:14px}.study-metrics dd{margin:8px 0 0;font-size:26px;font-weight:700}.study-metrics small{font-size:13px;font-weight:400}.study-subject{padding:12px 0;border-bottom:1px solid #edf0f5}.study-subject>div{display:flex;justify-content:space-between}.toolbar-actions{display:flex;flex-wrap:wrap;gap:10px;margin-top:18px}textarea{width:100%;resize:vertical}
</style>
