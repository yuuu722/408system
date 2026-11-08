<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { learningApi } from '@/api/learning'
import { messageOf } from '@/api/client'
import type { Mastery, Recommendation } from '@/api/types'
const weak = ref<Mastery[]>([]), recs = ref<Recommendation[]>([]), busy = ref(false), loading = ref(false), error = ref('')
async function load() {
  loading.value = true; error.value = ''
  try { [weak.value, recs.value] = await Promise.all([learningApi.weaknesses(), learningApi.recommendations()]) }
  catch (e) { error.value = messageOf(e) } finally { loading.value = false }
}
async function generate() {
  busy.value = true; error.value = ''
  try { recs.value = await learningApi.generateRecommendations() }
  catch (e) { error.value = messageOf(e) } finally { busy.value = false }
}
const actionNames: Record<string, string> = { review_material: '回顾学习资料', practice_and_review: '练习与复习',follow_daily_plan:'按学习计划复习' }
onMounted(load)
</script>
<template><div>
  <header class="page-head"><div><p class="eyebrow">REVIEW & REFLECTION</p><h1>薄弱点复习</h1><p>根据已有学习记录安排复习；提问本身并不代表未掌握。</p></div><button class="primary" :disabled="busy || loading" @click="generate">{{ busy ? '计算中…' : '重新生成建议' }}</button></header>
  <div v-if="error" class="notice error" role="alert">{{ error }} <button class="secondary" @click="load">重试</button></div>
  <div v-if="loading" class="card empty">正在加载复习数据…</div>
  <section v-else class="grid two-col"><div class="card"><div class="section-title"><h2>薄弱度排序</h2><span class="pill warn">{{ weak.length }} 个知识点</span></div><div v-if="!weak.length" class="empty"><h3>还没有足够的学习记录</h3><p>先完成前测或可靠题目的练习；浏览和提问不会直接生成掌握度。</p><RouterLink class="secondary" to="/exam408/assessment">开始测评 →</RouterLink></div><div class="list"><div v-for="w in weak" :key="w.KnowledgePointID" class="list-item"><div class="grow"><b>{{ w.KnowledgePointName }}</b><div class="reason">{{ w.SubjectName }} · 样本充分度（规则） {{ Math.round(w.Confidence * 100) }}% · {{ w.InteractionCount }} 个有效作答样本</div><div class="progress"><i :style="{ width: `${Math.round(w.WeaknessScore * 100)}%`, background: '#f59e0b' }"></i></div></div><span class="score">{{ Math.round(w.WeaknessScore * 100) }}%</span></div></div></div>
    <div class="card"><div class="section-title"><h2>可解释学习建议</h2></div><div v-if="!recs.length" class="empty">暂无推荐。积累学习记录后，点击“重新生成建议”。</div><div class="list"><div v-for="r in recs" :key="r.ID" class="list-item"><div class="grow"><b>{{ r.KnowledgePointName }}</b><div class="reason">{{ r.Reason }}</div><span class="pill">{{ actionNames[r.RecommendedAction] || r.RecommendedAction }}</span></div><span class="score">{{ Math.round(r.Score * 100) }}</span></div></div></div>
  </section>
</div></template>
