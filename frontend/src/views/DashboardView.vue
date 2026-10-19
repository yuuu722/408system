<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { learningApi } from '@/api/learning'
import { messageOf } from '@/api/client'
import type { Dashboard } from '@/api/types'
const data = ref<Dashboard | null>(null), error = ref(''), loading = ref(false)
async function load() {
  loading.value = true; error.value = ''
  try { data.value = await learningApi.dashboard() } catch (e) { error.value = messageOf(e) } finally { loading.value = false }
}
onMounted(load)
</script>
<template>
  <div>
    <header class="page-head"><div><p class="eyebrow">LEARNING OVERVIEW</p><h1>学情深度看板</h1><p>从真实的资料和学习记录中，找到下一步方向。</p></div><button class="secondary" :disabled="loading" @click="load">{{ loading ? '加载中…' : '刷新数据' }}</button></header>
    <div v-if="error" class="notice error" role="alert">{{ error }} <button class="secondary" @click="load">重试</button></div>
    <div v-if="loading && !data" class="card empty">正在加载学习数据…</div>
    <template v-if="data">
      <section class="grid metrics-grid">
        <div class="card metric blue"><span>个人知识库</span><b>{{ data.KnowledgeBaseCount }} <small>个</small></b><p>将课程资料整理成自己的学习空间</p></div>
        <div class="card metric purple"><span>已就绪资料</span><b>{{ data.ReadyDocumentCount }} <small>份</small></b><p>已完成处理，可用于资料问答</p></div>
        <div class="card metric orange"><span>待复习薄弱点</span><b>{{ data.Weaknesses.length }} <small>个</small></b><p>根据当前学习事件计算</p></div>
      </section>
      <section class="grid overview-grid section">
        <div class="card"><div class="section-title"><h2>四科学习进度</h2><RouterLink to="/exam408/knowledge">查看知识体系 →</RouterLink></div><p class="muted">按当前已录入知识点统计，尚未覆盖完整考纲。</p>
          <div class="subject-rows"><div v-for="s in data.Subjects" :key="s.ID" class="subject-row"><div class="section-title"><div><span class="subject-code">{{ s.Code }}</span><h3>{{ s.Name }}</h3></div><b>{{ s.TouchedCount ? Math.round(s.Progress * 100) + '%' : '待测' }}</b></div><div class="progress"><i :style="{ width: `${Math.round(s.Progress * 100)}%` }"></i></div><span class="muted">有记录 {{ s.TouchedCount }} / {{ s.KnowledgePointCount }} 个知识点 · 规则掌握度</span></div></div><div v-if="!data.Subjects.length" class="empty">暂无学科数据。</div>
        </div>
        <aside class="card advice-card"><h2>你的学习建议</h2><div class="advice-panel"><span class="pill">下一步行动</span><h3>把难点变成新的理解</h3><p>上传你的 408 学习资料，通过带引用的问答理解概念，再回到原文复习。</p><div v-if="data.Recommendations.length" class="advice-target"><b>{{ data.Recommendations[0]?.KnowledgePointName }}</b><p>{{ data.Recommendations[0]?.Reason }}</p></div><p v-else class="advice-target">目前还没有推荐。积累学习记录后，可在薄弱点页面生成建议。</p><RouterLink class="primary" to="/exam408/weakness">查看复习建议 →</RouterLink></div></aside>
      </section>
      <section class="grid two-col section"><div class="card"><div class="section-title"><h2>优先复习</h2><RouterLink to="/exam408/weakness">查看全部 →</RouterLink></div><div v-if="!data.Weaknesses.length" class="empty">完成测评后，这里会显示需要复习的知识点。</div><div class="list"><div v-for="w in data.Weaknesses.slice(0, 4)" :key="w.KnowledgePointID" class="list-item"><div class="grow"><b>{{ w.KnowledgePointName }}</b><div class="reason">{{ w.SubjectName }} · {{ w.InteractionCount }} 次交互</div></div><span class="pill warn">薄弱 {{ Math.round(w.WeaknessScore * 100) }}%</span></div></div></div>
        <div class="card"><div class="section-title"><h2>最近的学习资料</h2><RouterLink to="/exam408/resources">管理资料 →</RouterLink></div><div v-if="!data.RecentDocuments.length" class="empty">还没有资料，先创建知识库并上传一份笔记。</div><div class="list"><div v-for="d in data.RecentDocuments.slice(0, 4)" :key="d.ID" class="list-item"><div class="grow"><b>{{ d.OriginalFilename }}</b><div class="reason">{{ new Date(d.CreatedAt).toLocaleDateString() }}</div></div><span class="pill">{{ d.Status === 'ready' ? '已就绪' : d.Status === 'failed' ? '处理失败' : '处理中' }}</span></div></div></div>
      </section>
    </template>
  </div>
</template>

