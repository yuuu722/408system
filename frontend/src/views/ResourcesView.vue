<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Dialog as TDialog, MessagePlugin } from 'tdesign-vue-next'
import { learningApi } from '@/api/learning'
import { useAuthStore } from '@/stores/auth'
import { openDocument, foundation, type ParsedDocument } from '@/api/foundation'
import { api } from '@/api/client'
import { messageOf } from '@/api/client'
import type { Document, KnowledgeBase, KnowledgePoint } from '@/api/types'

const structure=ref<ParsedDocument|null>(null),structureTitle=ref(''),structureBusy=ref(false)
async function inspectStructure(d:Document){structureBusy.value=true;try{structure.value=await foundation.structure(d.ID,d.ActiveRevisionID?'':'latest-attempt');structureTitle.value=d.OriginalFilename}catch(e){MessagePlugin.error(messageOf(e))}finally{structureBusy.value=false}}
const bases = ref<KnowledgeBase[]>([]), docs = ref<Document[]>([]), points = ref<KnowledgePoint[]>([])
const auth = useAuthStore()
const limits=ref({max_upload_bytes:200*1024*1024,index_mode:'lexical',personal_kb_enabled:false})
const replaceTarget=ref<Document|null>(null),uploadProgress=ref(0),editingBase=ref(false)
function editBase(){const kb=selectedBase.value;if(!kb)return;name.value=kb.Name;description.value=kb.Description;editingBase.value=true;showCreate.value=true}
async function retryFailed(){if(!active.value||pending.value)return;pending.value='bulk';try{const {data}=await api.post('/admin/documents/retry-failed',{KnowledgeBaseID:active.value});MessagePlugin.success(`已重新排队 ${data.queued} 份失败资料`);await loadDocs()}catch(e){MessagePlugin.error(messageOf(e))}finally{pending.value=''}}
function replaceFile(d:Document){replaceTarget.value=d;file.value=null;showUpload.value=true}
const selectedBase = computed(() => bases.value.find(b => b.ID === active.value))
const active = ref(''), search = ref(''), status = ref(''), error = ref('')
const loading = ref(false), busy = ref(false), creating = ref(false), showCreate = ref(false), showUpload = ref(false)
const name = ref(''), description = ref(''), file = ref<File | null>(null), resourceType = ref('notes')
const mapping = ref<Record<string, string>>({}), pending = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let generation = 0, disposed = false
const filteredBases = computed(() => bases.value.filter(b => `${b.Name} ${b.Description}`.toLowerCase().includes(search.value.toLowerCase())))
const filteredDocs = computed(() => docs.value.filter(d => !status.value || d.Status === status.value))
const selectedName = computed(() => bases.value.find(b => b.ID === active.value)?.Name || '未选择知识库')
const stages: Record<string, string> = { queued: '等待处理', parsing: '正在解析文字', chunking: '正在切分文本', embedding: '正在生成检索向量', saving: '正在保存索引', completed: '处理完成' }
const progressOf = (d: Document) => Math.max(0, Math.min(100, d.Progress ?? 0))
const labels: Record<string, string> = { uploaded: '等待处理', processing: '处理中', ready: '已就绪', failed: '处理失败' }

async function loadDocs() {
  clearTimeout(timer)
  const current = active.value, request = ++generation
  if (!current) { docs.value = []; return }
  try {
    const result = await learningApi.documents(current)
    if (disposed || request !== generation || current !== active.value) return
    docs.value = result; error.value = ''
    if (result.some(d => d.Status === 'processing' || d.Status === 'uploaded')) timer = setTimeout(loadDocs, 2500)
  } catch (e) { if (!disposed && request === generation) { error.value = messageOf(e); timer = setTimeout(loadDocs, 5000) } }
}
async function selectBase(base: string) { active.value = base; docs.value = []; await loadDocs() }
async function refresh() {
  loading.value = true; error.value = ''
  try {
    bases.value = await learningApi.bases()
    if (!bases.value.some(b => b.ID === active.value)) active.value = bases.value[0]?.ID || ''
    await loadDocs()
  } catch (e) { error.value = messageOf(e) } finally { loading.value = false }
}
async function create() {
  if (!name.value.trim() || creating.value) return
  creating.value = true
  try {
    const base = editingBase.value?(await api.patch(`/admin/knowledge-bases/${active.value}`,{Name:name.value.trim(),Description:description.value.trim()})).data:await learningApi.createBase(name.value.trim(), description.value.trim())
    active.value = base.ID; showCreate.value = false; name.value = ''; description.value = ''
    await refresh(); MessagePlugin.success('知识库已创建，可以上传资料了')
  } catch (e) { MessagePlugin.error(messageOf(e)) } finally { creating.value = false }
}
async function upload() {
  if (!active.value || !file.value || busy.value) return
  if (file.value.size > limits.value.max_upload_bytes) { MessagePlugin.error(`文件不能超过 ${Math.floor(limits.value.max_upload_bytes/1024/1024)} MB`); return }
  const target = active.value
  busy.value = true
  try {
    const body=new FormData();body.append('file',file.value);body.append('resource_type',resourceType.value)
    uploadProgress.value=0
    await api.request({method:replaceTarget.value?'PUT':'POST',url:replaceTarget.value?`/documents/${replaceTarget.value.ID}/file`:`/knowledge-bases/${target}/documents`,data:body,timeout:1800000,onUploadProgress:e=>{if(e.total)uploadProgress.value=Math.round(e.loaded/e.total*100)}})
    replaceTarget.value=null
    showUpload.value = false; file.value = null; MessagePlugin.success('已上传，正在处理')
    if (target === active.value) await loadDocs()
  } catch (e) { MessagePlugin.error(messageOf(e)) } finally { busy.value = false }
}
async function documentAction(document: Document, action: 'map' | 'retry' | 'delete') {
  if (pending.value) return
  if (action === 'delete' && !window.confirm(`确定删除“${document.OriginalFilename}”及其检索内容吗？`)) return
  pending.value = document.ID
  try {
    if (action === 'map') await learningApi.mapPoint(document.ID, mapping.value[document.ID]!)
    if (action === 'retry') await learningApi.reprocess(document.ID)
    if (action === 'delete') await learningApi.removeDocument(document.ID)
    MessagePlugin.success(action === 'map' ? '知识点关联成功' : action === 'delete' ? '已删除' : '已重新提交处理')
    await loadDocs()
  } catch (e) { MessagePlugin.error(messageOf(e)) } finally { pending.value = '' }
}
onMounted(async () => {
  try{limits.value=(await api.get('/system/capabilities')).data}catch(e){error.value=messageOf(e)}
  await refresh()
  try { points.value = await learningApi.points() } catch (e) { error.value = messageOf(e) }
})
onUnmounted(() => { disposed = true; generation++; clearTimeout(timer) })
</script>

<template>
  <div>
    <header class="page-head"><div><p class="eyebrow">PERSONAL KNOWLEDGE BASE</p><h1>{{auth.user?.role==='admin'?'408 公共知识库管理':'408 公共学习资料'}}</h1><p>管理员负责建立和维护公共资料，普通用户只读浏览；文件解析后自动更新索引和图谱。</p></div></header>
    <div v-if="error" class="notice error" role="alert">{{ error }} <button class="secondary" @click="refresh">重试</button></div>
    <p class="notice">索引方式：{{limits.index_mode==='lexical'?'本地词法向量（无需 API Key，不是语义模型）':limits.index_mode==='demo'?'演示向量':'已配置的向量模型服务'}}。替换成功后更新图谱，删除文件同步移除其索引和图谱证据。</p><button v-if="auth.user?.role==='admin'" class="secondary" :disabled="!active" @click="editBase">修改所选知识库信息</button><button v-if="auth.user?.role==='admin'" class="secondary" :disabled="!active||!!pending" @click="retryFailed">重试本库所有失败任务</button><RouterLink v-if="active" class="secondary" :to="{path:auth.user?.role==='admin'?'/exam408/admin/graph':'/exam408/graph',query:{kb:active}}">查看所选库图谱</RouterLink><section class="grid action-grid">
      <button v-if="auth.user?.role === &quot;admin&quot;" class="action-tile blue" :disabled="!active || busy" @click="file = null; replaceTarget=null; showUpload = true"><span aria-hidden="true">↑</span><b>上传文件</b><small>{{ active ? 'TXT / Markdown / PDF / DOCX' : '请先创建或选择知识库' }}</small></button>
      <button v-if="auth.user?.role==='admin'" class="action-tile purple" @click="editingBase=false;name='';description='';showCreate = true"><span aria-hidden="true">＋</span><b>新建公共知识库</b><small>按学科或专题整理</small></button>
      <button class="action-tile orange" :disabled="loading" @click="refresh"><span aria-hidden="true">↻</span><b>{{ loading ? '刷新中…' : '刷新列表' }}</b><small>查看最新处理状态</small></button>
    </section>
    <section class="card section"><div class="section-title"><h2>可用知识库 <span class="muted">{{ bases.length }}</span></h2><input v-model="search" aria-label="搜索知识库" placeholder="搜索知识库…" /></div>
      <div v-if="loading && !bases.length" class="empty">正在加载知识库…</div>
      <div v-else-if="!bases.length" class="empty"><h3>你的学习空间，从一份资料开始</h3><p>先创建一个知识库，再上传教材或笔记。</p><button v-if="auth.user?.role==='admin'" class="primary" @click="editingBase=false;name='';description='';showCreate = true">创建第一个知识库</button></div>
      <div v-else-if="!filteredBases.length" class="empty">没有匹配的知识库。</div>
      <div class="grid kb-grid"><button v-for="kb in filteredBases" :key="kb.ID" class="kb-card" :class="{ selected: active === kb.ID }" :aria-pressed="active === kb.ID" @click="selectBase(kb.ID)"><span class="folder-mark" aria-hidden="true">▤</span><h3>{{ kb.Name }}</h3><p>{{ kb.Description || '个人学习资料库' }}</p><span class="muted">{{ active === kb.ID ? '当前选中' : '查看资料 →' }}</span></button></div>
    </section>
    <section class="card section"><div class="section-title"><div><h2>知识文件</h2><p class="muted">{{ selectedName }}</p></div><select v-model="status" aria-label="筛选文件状态"><option value="">全部状态</option><option value="ready">已就绪</option><option value="processing">处理中</option><option value="uploaded">等待处理</option><option value="failed">处理失败</option></select></div>
      <div v-if="!active" class="empty">请先创建或选择一个知识库。</div><div v-else-if="!filteredDocs.length" class="empty">{{ docs.length ? '没有符合筛选条件的文件。' : '当前知识库还没有资料，点击上方“上传文件”开始。' }}</div>
      <div class="document-list"><article v-for="d in filteredDocs" :key="d.ID" class="document-card"><div class="document-heading"><div class="grow"><h3>{{ d.OriginalFilename }}</h3><p class="muted">{{ (d.Size / 1024).toFixed(1) }} KB · {{ new Date(d.CreatedAt).toLocaleString() }}</p></div><span class="pill" :class="`status-${d.Status}`">{{ labels[d.Status] }}</span></div><div v-if="d.Status === 'processing' || d.Status === 'uploaded' || (d.Status === 'failed' && d.ProcessingStage)" class="processing-progress" :class="{ failed: d.Status === 'failed' }">
<div class="progress-caption"><span>{{ d.Status === 'failed' ? '处理已停止 · ' : '' }}{{ stages[d.ProcessingStage || 'queued'] || '处理中' }}</span><strong>{{ progressOf(d) }}%</strong></div>
<progress :value="progressOf(d)" max="100" :aria-label="`${d.OriginalFilename} 处理进度`">{{ progressOf(d) }}%</progress>
<small v-if="d.TotalChunks">已向量化 {{ d.ProcessedChunks || 0 }} / {{ d.TotalChunks }} 个文本块</small>
<small>阶段进度，非耗时预测；页面每 2.5 秒自动更新。</small>
</div><p v-if="d.ActiveRevisionID&&d.Status!=='ready'" class="notice">已有可用版本仍可检索；本次处理从保存的检查点继续。</p><p v-if="d.ErrorMessage" class="notice error">{{ d.ErrorMessage }}</p><div class="document-actions"><button class="secondary" :disabled="structureBusy" @click="inspectStructure(d)">查看解析结果</button><button class="secondary" @click="openDocument(d.ID).catch(e=>MessagePlugin.error(messageOf(e)))">查看原文</button><select v-if="!selectedBase?.IsPublic || auth.user?.role==='admin'" v-model="mapping[d.ID]" :aria-label="`为 ${d.OriginalFilename} 选择知识点`"><option :value="undefined">关联知识点</option><option v-for="p in points" :key="p.ID" :value="p.ID">{{ p.SubjectName }} · {{ p.Name }}</option></select><button v-if="!selectedBase?.IsPublic || auth.user?.role==='admin'" class="secondary" :disabled="!mapping[d.ID] || !!pending" @click="documentAction(d, 'map')">关联</button><button v-if="auth.user?.role==='admin'" class="secondary" :disabled="!!pending" @click="replaceFile(d)">替换文件</button><button v-if="(!selectedBase?.IsPublic || auth.user?.role==='admin') && (d.Status === 'failed' || d.Status === 'ready')" class="secondary" :disabled="!!pending" @click="documentAction(d, 'retry')">{{d.Status==='failed'?'继续处理':'检查并更新处理'}}</button><button v-if="!selectedBase?.IsPublic || auth.user?.role==='admin'" class="secondary danger" :disabled="!!pending" @click="documentAction(d, 'delete')">删除</button></div></article></div>
    </section>
    <TDialog v-model:visible="showCreate" :header="editingBase?'修改公共知识库':'新建公共知识库'" :footer="false"><form @submit.prevent="create"><div class="field"><label for="base-name">名称</label><input id="base-name" v-model="name" required maxlength="100" placeholder="例如：操作系统知识库" /></div><div class="field"><label for="base-description">说明</label><textarea id="base-description" v-model="description" rows="3" placeholder="这个知识库用于什么？" /></div><button class="primary" :disabled="creating || !name.trim()">{{ creating ? '保存中…' : editingBase?'保存修改':'创建公共知识库' }}</button></form></TDialog>
    <TDialog v-model:visible="showUpload" :header="replaceTarget?'替换资料文件':'上传公共资料'" :footer="false" :close-on-overlay-click="!busy"><form @submit.prevent="upload"><p class="muted">上传至：{{ selectedName }}</p><div class="field"><label for="resource-type">资料类型</label><select id="resource-type" v-model="resourceType"><option value="notes">笔记</option><option value="textbook">教材</option><option value="past_exam">真题</option><option value="solution">解析</option><option value="courseware">课件</option><option value="other">其他</option></select></div><div class="upload-zone"><label for="resource-file">选择 TXT、Markdown、PDF（含扫描件）或 DOCX</label><input id="resource-file" type="file" required accept=".txt,.md,.markdown,.pdf,.docx" @change="file = ($event.target as HTMLInputElement).files?.[0] || null" /><p class="muted">最大 {{Math.floor(limits.max_upload_bytes/1024/1024)}} MB · Docker 内置 PDF 文字提取与扫描页 OCR</p></div><button class="primary" :disabled="busy || !file || !active">{{ busy ? `上传中 ${uploadProgress}%` : replaceTarget?'替换并重建索引图谱':'上传并处理' }}</button></form></TDialog>
  </div>
<TDialog :visible="!!structure" :header="'解析结果 · '+structureTitle" width="900px" :footer="false" @close="structure=null"><p v-if="structure?.State==='staging'" class="notice">本次处理尚未启用，以下为已保存的解析成果，不进入检索与测评。</p><p class="muted">原文内容由用户负责；以下为系统解析结果，异常题目不会进入测评。页码按PDF物理页记录。</p><section v-for="page in structure?.Pages??[]" :key="`${page.Number}:${page.Segment??0}`"><h3>{{page.Number?'第 '+page.Number+' 页':'文档内容 · 段'+((page.Segment??0)+1)}}</h3><p v-for="warning in page.Warnings??[]" :key="warning" class="notice error">{{warning}}</p><pre style="white-space:pre-wrap;max-height:360px;overflow:auto">{{page.Text}}</pre><details v-for="block in page.Blocks?.filter(b=>b.Kind!=='text')??[]" :key="block.ID"><summary>{{block.Kind}} · {{block.Box?.join(', ')||'无页面坐标'}}</summary><img v-if="block.Image" :src="block.Image" alt="解析区域原图" style="max-width:100%"/><pre style="white-space:pre-wrap">{{block.Text}}</pre></details></section></TDialog>
</template>

<style scoped>
.processing-progress { margin: 16px 0; padding: 14px 16px; background: #f4f7ff; border-radius: 10px; }
.progress-caption { display: flex; justify-content: space-between; gap: 12px; margin-bottom: 8px; color: #334155; font-size: 14px; }
.processing-progress progress { display: block; width: 100%; height: 8px; border: 0; border-radius: 8px; overflow: hidden; background: #e2e8f0; accent-color: #3364ef; }
progress::-webkit-progress-bar { background: #e2e8f0; border-radius: 8px; }
progress::-webkit-progress-value { background: #3364ef; border-radius: 8px; transition: width .25s ease; }
progress::-moz-progress-bar { background: #3364ef; border-radius: 8px; }
.processing-progress small { display: block; margin-top: 8px; color: #64748b; }
.failed { background: #fff4f4; }
.failed progress { accent-color: #dc2626; }
.failed progress::-webkit-progress-value, .failed progress::-moz-progress-bar { background: #dc2626; }
</style>
