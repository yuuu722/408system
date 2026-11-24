<script setup lang="ts">
import {onMounted,ref} from 'vue'
import {foundation,type StudySummary,type DailyPlan} from '@/api/foundation'
import {messageOf} from '@/api/client'
const summary=ref<StudySummary>(),plan=ref<DailyPlan|null>()
const summaryLoading=ref(false),planLoading=ref(false),summaryError=ref(''),planError=ref('')
async function loadSummary(){
 summaryLoading.value=true;summaryError.value=''
 try{summary.value=await foundation.studySummary(12000)}
 catch(e){summaryError.value=messageOf(e)}
 finally{summaryLoading.value=false}
}
async function loadPlan(){
 planLoading.value=true;planError.value=''
 try{plan.value=await foundation.dailyPlan(12000)}
 catch(e){planError.value=messageOf(e)}
 finally{planLoading.value=false}
}
onMounted(()=>{void loadSummary();void loadPlan()})
</script>
<template><div><header class="page-head"><div><p class="eyebrow">408 学习概览</p><h1>今天，从下一步开始</h1><p>先测评找漏洞，再按计划学习；资料、练习和答疑各有入口。</p></div></header><p v-if="summary?.NeedsPretest" class="notice">还没有完成有效前测。<RouterLink to="/exam408/assessment">开始前测</RouterLink></p><section class="grid metrics-grid"><article class="card section"><h2>有效练习</h2><p v-if="summaryLoading" role="status">正在加载练习统计…</p><p v-else-if="summaryError" class="notice error" role="alert">{{summaryError}} <button class="secondary" @click="loadSummary">重试统计</button></p><p v-else-if="summary">{{summary.ValidFamilies}} 个独立题组</p><RouterLink to="/exam408/assessment">测评与学习报告 →</RouterLink></article><article class="card section"><h2>今日安排</h2><p v-if="planLoading" role="status">正在加载今日计划…</p><p v-else-if="planError" class="notice error" role="alert">{{planError}} <button class="secondary" @click="loadPlan">重试计划</button></p><p v-else-if="plan">{{plan.AllocatedMinutes}} / {{plan.BudgetMinutes}} 分钟</p><p v-else>今日还没有学习计划。</p><RouterLink to="/exam408/tasks">执行学习任务 →</RouterLink></article><article class="card section"><h2>需要老师帮助？</h2><p>自由提问与来源核对，或选择一对一课堂。</p><RouterLink to="/exam408/assistant">智能答疑 →</RouterLink></article></section><section class="grid two-col section"><article class="card section"><h2>测评学情</h2><p>前测、练习、报告、薄弱点和学情统计统一查看。</p><RouterLink to="/exam408/assessment">进入测评学情</RouterLink></article><article class="card section"><h2>学习任务</h2><p>今日计划、个性化方案和老师课堂统一安排。</p><RouterLink to="/exam408/tasks">进入学习任务</RouterLink></article><article class="card section"><h2>题库练习</h2><p>按知识点寻找可用题目与参考解析。</p><RouterLink to="/exam408/assets">查看题库</RouterLink></article><article class="card section"><h2>知识体系与资料</h2><p>思维导图帮助理解结构；资料知识库管理文件和检索范围。</p><div class="toolbar-actions"><RouterLink to="/exam408/knowledge">知识体系</RouterLink><RouterLink to="/exam408/resources">资料知识库</RouterLink></div></article></section></div></template>
