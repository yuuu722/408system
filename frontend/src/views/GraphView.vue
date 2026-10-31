<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {useAuthStore} from '@/stores/auth'
import { MessagePlugin } from 'tdesign-vue-next'
import KnowledgeGraphCanvas from '@/components/KnowledgeGraphCanvas.vue'
import { openDocument } from '@/api/foundation'
import { learningApi } from '@/api/learning'
import { messageOf } from '@/api/client'
import { relationLabels } from '@/lib/graphLayout'
import type { KnowledgeBase, KnowledgeGraph, KnowledgePoint, GraphNode, GraphEdge } from '@/api/types'
const auth=useAuthStore()
const route = useRoute(), bases = ref<KnowledgeBase[]>([]), active = ref(''), graph = ref<KnowledgeGraph | null>(null)
const error=ref('')
const loading = ref(false), points = ref<KnowledgePoint[]>([]), focused = ref(''), showRecords = ref(false)
const focusNode = computed(() => graph.value?.Nodes?.find(n => n.ID === focused.value))
const focusEdges = computed(() => graph.value?.Edges?.filter(e => e.FromEntityID === focused.value || e.ToEntityID === focused.value) ?? [])
const nodeByID = computed(() => new Map((graph.value?.Nodes ?? []).map(n => [n.ID, n])))
const sourceName = (source: string) => source === 'llm_candidate' ? '模型候选（已校验证据）' : source === 'rule_candidate' ? '规则候选' : '规范目录'
function showSource(item: GraphNode | GraphEdge) { if (item.EvidenceDocumentID) void openDocument(item.EvidenceDocumentID).catch(e => MessagePlugin.error(messageOf(e))) }
async function loadGraph() {
  focused.value = ''; showRecords.value = false
  if (!active.value) { graph.value = null; return }
  loading.value = true;error.value=''
  try { graph.value = await learningApi.graph(active.value) }
  catch (e) { if(graph.value?.KnowledgeBaseID!==active.value)graph.value=null;error.value=messageOf(e) }
  finally { loading.value = false }
}
async function initialize(){
 try {
  bases.value=(await learningApi.bases()).filter(b=>auth.user?.role!=='admin'||b.IsPublic)
  active.value=bases.value.find(b=>b.ID===route.query.kb)?.ID||bases.value.find(b=>b.IsPublic)?.ID||bases.value[0]?.ID||''
  await Promise.all([loadGraph(),learningApi.points().then(value=>points.value=value).catch(()=>{error.value='四科目录暂未加载，图谱节点和关系仍可查看。'})])
 }catch(e){error.value=messageOf(e)}
}
function retryGraph(){if(active.value)void loadGraph();else void initialize()}
onMounted(initialize)

</script>
<template>
  <div class="graph-page">
    <header class="page-head"><div><p class="eyebrow">KNOWLEDGE CONNECTIONS</p><h1>知识图谱</h1><p v-if="graph?.Storage" class="muted">图谱存储：{{graph.Storage==='neo4j'?'Neo4j':graph.Storage==='postgres-recovery'?'已保存图谱（恢复模式）':'PostgreSQL（兼容模式）'}}</p><p>从知识点出发，看见四科之间的联系，沿着关系回到资料原文。</p></div><div class="graph-actions"><select v-model="active" :disabled="loading" aria-label="选择知识库" @change="loadGraph"><option v-for="b in bases" :key="b.ID" :value="b.ID">{{ b.Name }}{{ b.IsPublic ? '（公共）' : '（个人）' }}</option></select><button class="secondary" :disabled="loading || !active" @click="loadGraph">{{ loading ? '加载中…' : '刷新图谱' }}</button></div></header>
    <p v-if="error" class="notice error" role="alert">{{error}} <button class="secondary" @click="retryGraph">重新加载图谱</button></p><p v-if="graph?.Warning" class="notice" role="status">{{graph.Warning}}</p><section v-if="loading" class="card" role="status">正在加载知识点和关系……</section>
    <template v-else-if="graph">
      <KnowledgeGraphCanvas :nodes="graph.Nodes ?? []" :edges="graph.Edges ?? []" :points="points" @select="id => focused = id">
        <template #details>
          <template v-if="focusNode"><p class="eyebrow">KNOWLEDGE DETAIL</p><h2 class="detail-name">{{ focusNode.Name }}</h2><span class="pill">{{ sourceName(focusNode.Source) }}</span><p class="detail-description">{{ focusNode.Description || '暂无详细说明，可查看相邻关系及来源。' }}</p>
            <h3>来源依据</h3><blockquote v-if="focusNode.EvidenceExcerpt">{{ focusNode.EvidenceExcerpt }}</blockquote><p v-else class="muted">{{ focusNode.Source === 'canonical' ? '管理员维护的规范目录节点。' : '此节点暂无可展示的原文片段。' }}</p><button v-if="focusNode.EvidenceDocumentID" class="secondary evidence-button" @click="showSource(focusNode)">查看原文 · {{ focusNode.EvidenceDocumentTitle }}</button>
            <h3>相邻关系 <small>{{ focusEdges.length }}</small></h3><p v-if="!focusEdges.length" class="muted">当前尚无相邻关系，未添加无依据的连线。</p><article v-for="e in focusEdges" :key="e.ID" class="relation-evidence"><b>{{ nodeByID.get(e.FromEntityID)?.Name || '未知节点' }} <span>→ {{ relationLabels[e.RelationType] || e.RelationType }} →</span> {{ nodeByID.get(e.ToEntityID)?.Name || '未知节点' }}</b><small>{{ sourceName(e.Source) }}<span v-if="(e.EvidenceCount??0)>1"> · 合并 {{e.EvidenceCount}} 条来源记录，展示一条代表依据</span>{{ e.Source === 'rule_candidate' ? ' · 共现不代表因果' : '' }}</small><blockquote v-if="e.EvidenceExcerpt">{{ e.EvidenceExcerpt }}</blockquote><button v-if="e.EvidenceDocumentID" class="secondary evidence-button" @click="showSource(e)">来源 · {{ e.EvidenceDocumentTitle }}</button><p v-else class="muted">{{ e.Source === 'canonical' ? '来自规范目录关系，无上传原文。' : '暂无可访问的原文件。' }}</p></article>
          </template>
          <template v-else><p class="eyebrow">EXPLORE THE GRAPH</p><h2 class="detail-name">让知识关联一目了然</h2><p class="detail-description">每个圆点代表一个知识点或资料概念，连线代表已保存的关系。关联越多，节点越醒目。</p><ol class="explore-steps"><li>点击节点，高亮直接相连的知识点。</li><li>搜索名称，快速定位相关知识簇。</li><li>选择学科颜色，专注查看单科。</li><li>查看来源依据，核对节点与关系。</li></ol><p class="muted">节点和连线数量来自当前知识库实际数据。资料还在处理时，可稍后刷新查看新增结果。</p></template>
        </template>
      </KnowledgeGraphCanvas>
      <details class="card graph-records" @toggle="showRecords = ($event.target as HTMLDetailsElement).open"><summary>查看全部节点与关系记录（{{ graph.Nodes?.length ?? 0 }} 个节点 / {{ graph.Edges?.length ?? 0 }} 条关系）</summary><template v-if="showRecords"><h2>知识点</h2><div class="list"><article v-for="n in graph.Nodes" :key="n.ID" class="list-item"><div class="grow"><b>{{ n.Name }}</b><p class="reason">{{ n.Description || n.EntityType }} · {{ sourceName(n.Source) }}</p><blockquote v-if="n.EvidenceExcerpt">{{ n.EvidenceExcerpt }}</blockquote><button v-if="n.EvidenceDocumentID" class="secondary" @click="showSource(n)">来源：{{ n.EvidenceDocumentTitle }}</button></div></article></div><h2>关系</h2><div class="list"><article v-for="e in graph.Edges" :key="e.ID" class="list-item"><div class="grow"><b>{{ nodeByID.get(e.FromEntityID)?.Name }} → {{ relationLabels[e.RelationType] || e.RelationType }} → {{ nodeByID.get(e.ToEntityID)?.Name }}</b><p class="reason">{{ sourceName(e.Source) }}</p><blockquote v-if="e.EvidenceExcerpt">{{ e.EvidenceExcerpt }}</blockquote><button v-if="e.EvidenceDocumentID" class="secondary" @click="showSource(e)">来源：{{ e.EvidenceDocumentTitle }}</button></div></article></div></template></details>
    </template>
    <section v-else class="card"><h2>{{error?'图谱暂未加载':'暂无可展示的图谱'}}</h2><p>{{error?'请根据上方提示重试，已有资料无需重新上传。':'请选择可访问的知识库，或等待资料处理完成。'}}</p></section>
  </div>
</template>
<style scoped>
.graph-actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.detail-name{font-size:21px;color:#263a52;margin:10px 0 15px;line-height:1.5;overflow-wrap:anywhere}.detail-description{line-height:1.9;margin:20px 0;color:#65768c}.graph-page :deep(.graph-details h3){font-size:14px;color:#354b65;margin-top:24px}.graph-page :deep(.graph-details h3 small){color:#8fa0b5;margin-left:7px}.graph-page blockquote{border-left:3px solid #c9d8eb;background:#f0f5fb;padding:10px 12px;margin:10px 0;font-size:12px;line-height:1.8;white-space:pre-wrap;overflow-wrap:anywhere}.evidence-button{white-space:normal;text-align:left;font-size:12px;max-width:100%;overflow-wrap:anywhere}.relation-evidence{border-top:1px solid #e3eaf3;margin-top:14px;padding-top:14px}.relation-evidence b{line-height:1.8}.relation-evidence b span{font-weight:normal;color:#8b9bb0}.relation-evidence>small{display:block;margin-top:8px;color:#91a0b3;font-size:11px}.explore-steps{padding-left:20px;line-height:2.4;color:#63758d}.graph-records{margin:20px 0}.graph-records summary{cursor:pointer;color:#5d718c;font-size:13px;padding:4px}.graph-records h2{font-size:18px;margin-top:25px}
</style>
