<script setup lang="ts">
import {ref,onMounted,watch} from 'vue'
import {foundation,type DailyPlan,type StudyTask} from '@/api/foundation'
import {messageOf} from '@/api/client'
import {loadStudyPlan} from '@/lib/studyPlan'
const props=defineProps<{assessed:boolean;refreshKey?:string}>()
const plan=ref<DailyPlan>(),tasks=ref<StudyTask[]>([]),busy=ref(false),error=ref('')
async function load(regenerate=false){if(busy.value)return;busy.value=true;error.value='';try{const data=await loadStudyPlan(foundation,props.assessed,regenerate);plan.value=data.plan;tasks.value=data.tasks}catch(e){error.value='学习记录已保留，计划暂未获取成功：'+messageOf(e)}finally{busy.value=false}}
onMounted(()=>load());watch(()=>[props.assessed,props.refreshKey],()=>load())
</script>
<template><section class="card plan-preview" aria-label="个性化复习计划"><div class="plan-heading"><div><p class="eyebrow">按测评结果安排下一步</p><h2>今日复习计划</h2></div><button class="secondary" :disabled="busy" @click="load(true)">{{busy?'正在安排…':'更新计划'}}</button></div><p v-if="error" class="notice error" role="alert">{{error}}</p><p v-else-if="busy" role="status">正在获取学习计划…</p><template v-else><p v-if="plan?.ID">{{plan.Date}} · 已安排 {{plan.AllocatedMinutes}} / {{plan.BudgetMinutes}} 分钟，包含已完成任务。</p><p v-if="!assessed" class="muted">完成前测后会根据有效作答更新；目前可以先做基础学习。</p><ol v-if="tasks.length" class="plan-items"><li v-for="task in tasks.slice(0,3)" :key="task.ID"><div><b>{{task.Title}}</b><span class="pill">{{task.Completed?'已完成':`约 ${task.EstimatedMinutes||0} 分钟`}}</span></div><p>{{task.Reason}}</p><RouterLink :to="{path:'/exam408/tasks',query:{tab:'teacher',point:task.PointID}}">老师带我学</RouterLink><RouterLink v-if="task.Kind==='question'" :to="{path:'/exam408/assessment',query:{point:task.PointID,asset:task.AssetID}}">开始练习 →</RouterLink></li></ol><p v-else>当前没有排入今日的可用任务。可能是今日预算已用完、已做对题目尚未到复习间隔，或缺少可安排资源；这不代表你已掌握全部知识点。可在个人中心调整每日预算，或进行专项练习。</p><RouterLink class="primary" to="/exam408/tasks">查看完整任务与复习顺序 →</RouterLink></template></section></template>
<style scoped>.plan-preview{margin:24px 0}.plan-heading,.plan-items li>div{display:flex;justify-content:space-between;align-items:center;gap:12px}.plan-heading h2{margin:0}.plan-items{padding-left:22px}.plan-items li{padding:14px 0;border-bottom:1px solid #edf2f7}.plan-items p{color:#64748b;font-size:13px;line-height:1.7}.plan-items a{margin-right:18px}@media(max-width:600px){.plan-items li>div{align-items:flex-start;flex-direction:column}}
</style>
