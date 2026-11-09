<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {foundation,unitApi,type DailyPlan,type LearningUnit} from '@/api/foundation'
import {loadStudyPlan} from '@/lib/studyPlan'
import {messageOf} from '@/api/client'
import LearningUnitCard from '@/components/LearningUnitCard.vue'
const units=ref<LearningUnit[]>([]),plan=ref<DailyPlan|null>(null),busy=ref(false),error=ref('')
async function load(regenerate=false){busy.value=true;error.value='';try{const history=await foundation.assessments();const data=await loadStudyPlan(foundation,history.some(a=>a.Kind==='diagnostic'&&a.Result!==null),regenerate);plan.value=data.plan;units.value=await unitApi.list()}catch(e){error.value=messageOf(e)}finally{busy.value=false}}
onMounted(()=>load())
</script>
<template><div><header class="page-head"><div><h1>我的学习任务</h1><p>围绕知识点安排完整学习单元：教材复习、典型例题、独立练习与新题检查。</p></div><button class="primary" :disabled="busy" @click="load(true)">更新今日学习单元</button></header><p v-if="error" class="notice error" role="alert">{{error}}</p><p v-if="busy" role="status">正在读取学习单元…</p><section v-if="plan" class="card"><h2>今日计划 · {{plan.Date}}</h2><p>预计安排 {{plan.AllocatedMinutes}} / {{plan.BudgetMinutes}} 分钟，包含已完成任务；完整单元可按预算分次完成。</p><p class="muted">时间为估计值。已测薄弱内容结合参考重要性优先安排；待测内容不直接视为薄弱点。学习进度与掌握度分开记录。当天已安排单元会保留；降低预算时以已安排时间为下限，后续新计划按新预算安排。</p></section><LearningUnitCard v-for="unit in units" :key="unit.PointID" :unit="unit" @refresh="load()"/><section v-if="!busy&&!error&&!units.length" class="card empty"><h2>暂无可安排的学习单元</h2><p>请检查每日预算和可用资料；没有可靠例题或练习时不会编造内容。</p><RouterLink to="/exam408/assessment">前往前测与学情</RouterLink></section></div></template>
