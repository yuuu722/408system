<script setup lang="ts">
import KnowledgeGraphCanvas from '@/components/KnowledgeGraphCanvas.vue'
import type { KnowledgePoint } from '@/api/types'
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { openDocument } from '@/api/foundation'
import { learningApi } from '@/api/learning'
import { messageOf } from '@/api/client'
import type { KnowledgeBase, KnowledgeGraph } from '@/api/types'

const route=useRoute()
const bases = ref<KnowledgeBase[]>([])
const active = ref('')
const graph = ref<KnowledgeGraph | null>(null)
const loading = ref(false),points=ref<KnowledgePoint[]>([]),focused=ref('')
const focusNode=computed(()=>graph.value?.Nodes?.find(n=>n.ID===focused.value))
const focusEdges=computed(()=>graph.value?.Edges?.filter(e=>e.FromEntityID===focused.value||e.ToEntityID===focused.value)||[])
const selected = computed(() => bases.value.find(b => b.ID === active.value))
const nodeByID = computed(() => new Map((graph.value?.Nodes ?? []).map(n => [n.ID, n] as const)))
async function loadGraph() {
  if (!active.value) { graph.value = null; return }
  loading.value = true
  try { graph.value = await learningApi.graph(active.value) }
  catch (e) { graph.value = null; MessagePlugin.error(messageOf(e)) }
  finally { loading.value = false }
}
async function init() {
  try {
    bases.value = await learningApi.bases();points.value=await learningApi.points()
    active.value = bases.value.find(b=>b.ID===route.query.kb)?.ID || bases.value.find(b => b.IsPublic)?.ID || bases.value[0]?.ID || ''
    await loadGraph()
  } catch (e) { MessagePlugin.error(messageOf(e)) }
}
onMounted(init)
</script>

<template>
  <div>
    <header class="page-head">
      <div>
        <p class="eyebrow">KNOWLEDGE GRAPH</p>
        <h1>知识图谱</h1>
        <p>公共目录先建立基础图谱；资料只补充通过原文校验的节点和关系，个人上传不会修改公共图谱。</p>
      </div>
      <select v-model="active" @change="loadGraph" :disabled="loading" aria-label="选择知识库">
        <option v-for="b in bases" :key="b.ID" :value="b.ID">{{ b.Name }}{{ b.IsPublic ? '（公共）' : '（个人）' }}</option>
      </select>
    </header>

    <button class="secondary" :disabled="loading" @click="loadGraph">刷新图谱</button><section class="card" v-if="loading">正在加载图谱……</section>
    <template v-else-if="graph">
      <KnowledgeGraphCanvas :nodes="graph.Nodes??[]" :edges="graph.Edges??[]" :points="points" @select="id=>focused=id"/><section v-if="focusNode" class="card section"><h2>{{focusNode.Name}}</h2><p>{{focusNode.Description}}</p><blockquote>{{focusNode.EvidenceExcerpt||'此节点来自权威目录。'}}</blockquote><article v-for="e in focusEdges" :key="e.ID"><b>{{nodeByID.get(e.FromEntityID)?.Name}} → {{nodeByID.get(e.ToEntityID)?.Name}} · {{e.RelationType}}</b><p>{{e.EvidenceExcerpt||'基础目录关系'}}</p><button v-if="e.EvidenceDocumentID" class="secondary" @click="openDocument(e.EvidenceDocumentID).catch(err=>MessagePlugin.error(messageOf(err)))">查看来源：{{e.EvidenceDocumentTitle}}</button></article></section><div class="stats-grid" style="margin-bottom:18px">
        <section class="card"><p class="eyebrow">NODES</p><h2>{{ graph.Nodes?.length ?? 0 }}</h2><p class="reason">已识别的知识点节点</p></section>
        <section class="card"><p class="eyebrow">EDGES</p><h2>{{ graph.Edges?.length ?? 0 }}</h2><p class="reason">带证据的关系记录</p></section>
      </div>
      <section class="card" style="margin-bottom:18px">
        <h2>节点</h2>
        <p class="reason" v-if="!graph.Nodes?.length">当前知识库尚无已识别节点。处理一份包含现有 408 知识点名称或关键词的资料后，这里会显示匹配结果。</p>
        <div class="list">
          <div class="list-item" v-for="n in graph.Nodes" :key="n.ID">
            <div class="grow"><b>{{ n.Name }}</b><div class="reason">{{ n.Description || n.EntityType }}</div><blockquote v-if="n.EvidenceExcerpt" style="margin:8px 0 0;padding:8px 10px;border-left:3px solid var(--td-brand-color);white-space:pre-wrap">{{ n.EvidenceExcerpt }}</blockquote><button v-if="n.EvidenceDocumentID" class="secondary" @click="openDocument(n.EvidenceDocumentID).catch(err=>MessagePlugin.error(messageOf(err)))">来源：{{n.EvidenceDocumentTitle}}</button></div>
            <span class="pill">{{ n.Source === "llm_candidate" ? `AI 候选 · ${Math.round((n.Confidence || 0) * 100)}%` : n.Source === "rule_candidate" ? "规则定义候选" : "权威目录" }}</span>
          </div>
        </div>
      </section>
      <section class="card">
        <h2>关系与来源证据</h2>
        <p class="reason" v-if="!graph.Edges?.length">当前没有可展示的关系。开启模型抽取后，仅补充通过类型、端点与原文证据校验的候选关系。</p>
        <div class="list">
          <article class="list-item" v-for="e in graph.Edges" :key="e.ID" style="align-items:flex-start">
            <div class="grow">
              <b>{{ nodeByID.get(e.FromEntityID)?.Name || '未知节点' }} → {{ nodeByID.get(e.ToEntityID)?.Name || '未知节点' }}</b>
              <div class="reason">关系类型：{{ e.RelationType }} · 来源：{{ e.Source === "llm_candidate" ? "AI 候选（已校验）" : e.Source === "rule_candidate" ? "规则共现（非因果关系）" : "权威目录" }} · 置信度：{{ Math.round((e.Confidence || 0) * 100) }}%</div>
              <blockquote v-if="e.EvidenceExcerpt" style="margin:10px 0 0;padding:10px 12px;border-left:3px solid var(--td-brand-color);background:var(--td-bg-color-container-hover);white-space:pre-wrap">{{ e.EvidenceExcerpt }}</blockquote>
              <button v-if="e.EvidenceDocumentID" class="secondary" @click="openDocument(e.EvidenceDocumentID).catch(err=>MessagePlugin.error(messageOf(err)))">来源：{{e.EvidenceDocumentTitle}}</button><div v-else class="reason">来自管理员维护的基础目录，无上传原文。</div>
            </div>
          </article>
        </div>
      </section>
    </template>
    <section class="card" v-else>
      <h2>还没有可用知识库</h2>
      <p>请先创建个人知识库，或等待管理员初始化公共知识库。</p>
    </section>
  </div>
</template>
