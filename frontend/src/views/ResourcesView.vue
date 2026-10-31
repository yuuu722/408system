<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Dialog as TDialog, MessagePlugin } from 'tdesign-vue-next'
import { learningApi } from '@/api/learning'
import { useAuthStore } from '@/stores/auth'
import { openDocument, foundation, type ParsedDocument } from '@/api/foundation'
import { api } from '@/api/client'
import { messageOf } from '@/api/client'
import type { Document, KnowledgeBase, KnowledgePoint } from '@/api/types'

interface Catalog {DocumentID:string;ResourceType:string;SubjectCode:string;Purpose:string;ReviewState:string;Reason:string}
const catalogs=ref<Record<string,Catalog>>({}),catalogDraft=ref<Catalog|null>(null),organizing=ref(false),organizeMessage=ref(''),organizeErrors=ref<string[]>([]),typeFilter=ref(''),subjectFilter=ref(''),uploadPurpose=ref('')
const typeNames:Record<string,string>={textbook:'教材',past_exam:'历年真题',exercise:'习题题集',mock_exam:'模拟试卷',solution:'答案解析',courseware:'课件讲义',notes:'笔记总结',outline:'考纲',other:'待分类'}
const subjectNames:Record<string,string>={ds:'数据结构',co:'计算机组成原理',os:'操作系统',cn:'计算机网络',general:'综合 / 跨科'}
const purposeNames:Record<string,string>={knowledge:'知识库',questions:'题库',both:'知识库与题库'}
const catalogVisible=computed({get:()=>catalogDraft.value!==null,set:(value:boolean)=>{if(!value)catalogDraft.value=null}})
function editCatalog(d:Document){catalogDraft.value={...(catalogs.value[d.ID]||{DocumentID:d.ID,ResourceType:d.ResourceType||'other',SubjectCode:'',Purpose:'knowledge',ReviewState:'pending',Reason:''})}}
async function saveCatalog(){if(!catalogDraft.value)return;try{const c={...catalogDraft.value,ReviewState:'confirmed',Reason:'管理员核对分类'};await api.patch(`/documents/${c.DocumentID}/catalog`,c);catalogDraft.value=null;await loadDocs();MessagePlugin.success('分类已确认；题库用途可点击整理已有资料生成候选题')}catch(e){MessagePlugin.error(messageOf(e))}}
async function organizeExisting(){if(organizing.value||!active.value)return;organizing.value=true;organizeErrors.value=[];const targets=docs.value.filter(d=>d.Status==='ready');let done=0,failed=0;try{for(const d of targets){organizeMessage.value=`整理 ${done+failed+1}/${targets.length}：${d.OriginalFilename}`;try{await api.post(`/documents/${d.ID}/organize`,{}, {timeout:120000});done++}catch(e){failed++;organizeErrors.value.push(`${d.OriginalFilename}：${messageOf(e)}`)}}await loadDocs();organizeMessage.value=`整理完成 ${done}/${targets.length}，失败 ${failed}。未确认的分类仍需核对；抽取题目在待审核题库。`}finally{organizing.value=false}}
const selectedDocs=ref<string[]>([]),batchType=ref('textbook'),batchSubject=ref('os'),batchPurpose=ref('knowledge'),batchBusy=ref(false)
async function batchClassify(){if(!selectedDocs.value.length||batchBusy.value)return;batchBusy.value=true;try{const {data}=await api.post(`/knowledge-bases/${active.value}/catalog/batch`,{DocumentIDs:selectedDocs.value,ResourceType:batchType.value,SubjectCode:batchSubject.value,Purpose:batchPurpose.value});organizeMessage.value=`已确认 ${data.updated} 份，失败 ${Object.keys(data.failed).length} 份`;organizeErrors.value=Object.entries(data.failed).map(([id,e])=>`${id}：${e}`);selectedDocs.value=[];await loadDocs()}catch(e){MessagePlugin.error(messageOf(e))}finally{batchBusy.value=false}}
const structure=ref<ParsedDocument|null>(null),structureTitle=ref(''),structureBusy=ref(false)
async function inspectStructure(d:Document){structureBusy.value=true;try{structure.value=await foundation.structure(d.ID,d.ActiveRevisionID?'':'latest-attempt');structureTitle.value=d.OriginalFilename}catch(e){MessagePlugin.error(messageOf(e))}finally{structureBusy.value=false}}
const bases = ref<KnowledgeBase[]>([]), docs = ref<Document[]>([]), points = ref<KnowledgePoint[]>([])
const auth = useAuthStore()
const limits=ref({max_upload_bytes:200*1024*1024,index_mode:'lexical',personal_kb_enabled:false})
const replaceTarget=ref<Document|null>(null),uploadProgress=ref(0),editingBase=ref(false)
function editBase(){const kb=selectedBase.value;if(!kb)return;name.value=kb.Name;description.value=kb.Description;editingBase.value=true;showCreate.value=true}
async function upgradeIndex(){if(!active.value||pending.value||!confirm('将本库资料排队升级为保留章节和前提的新分块。解析版本未变时复用检查点；已有可用版本在处理期间仍可检索。是否继续？'))return;pending.value='upgrade';try{const {data}=await api.post(`/knowledge-bases/${active.value}/rebuild-index`,{}, {timeout:120000});MessagePlugin.success(`已排队 ${data.queued} 份资料，失败 ${data.failed.length} 份`);await loadDocs()}catch(e){MessagePlugin.error(messageOf(e))}finally{pending.value=''}}
async function retryFailed(){if(!active.value||pending.value)return;pending.value='bulk';try{const {data}=await api.post('/admin/documents/retry-failed',{KnowledgeBaseID:active.value});MessagePlugin.success(`已重新排队 ${data.queued} 份失败资料`);await loadDocs()}catch(e){MessagePlugin.error(messageOf(e))}finally{pending.value=''}}
function replaceFile(d:Document){uploadPurpose.value=catalogs.value[d.ID]?.Purpose||'knowledge';replaceTarget.value=d;file.value=null;showUpload.value=true}
const newBasePublic=ref(false)
const canMaintain=computed(()=>!!selectedBase.value&&(auth.user?.role==='admin'||!selectedBase.value.IsPublic))
async function deletePersonalBase(){if(!selectedBase.value||selectedBase.value.IsPublic||!confirm('删除此个人知识库及其资料、索引与图谱？'))return;try{await api.delete(`/knowledge-bases/${active.value}`);active.value='';await refresh()}catch(e){MessagePlugin.error(messageOf(e))}}
const selectedBase = computed(() => bases.value.find(b => b.ID === active.value))
const active = ref(''), search = ref(''), status = ref(''), error = ref('')
const loading = ref(false), busy = ref(false), creating = ref(false), showCreate = ref(false), showUpload = ref(false)
const name = ref(''), description = ref(''), file = ref<File | null>(null), resourceType = ref('auto')
const mapping = ref<Record<string, string>>({}), pending = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let generation = 0, disposed = false
const filteredBases = computed(() => bases.value.filter(b => `${b.Name} ${b.Description}`.toLowerCase().includes(search.value.toLowerCase())))
const filteredDocs = computed(() => docs.value.filter(d => (canMaintain.value || catalogs.value[d.ID]?.ReviewState==='confirmed')&&(!status.value || d.Status === status.value)&&(!typeFilter.value||(catalogs.value[d.ID]?.ResourceType||d.ResourceType||'other')===typeFilter.value)&&(!subjectFilter.value||(subjectFilter.value==='unassigned'?!(catalogs.value[d.ID]?.SubjectCode):(catalogs.value[d.ID]?.SubjectCode||'')===subjectFilter.value))))
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
    docs.value = result; error.value = ''; const {data}=await api.get<Catalog[]>(`/knowledge-bases/${current}/catalog`);if(request===generation&&current===active.value)catalogs.value=Object.fromEntries(data.map(c=>[c.DocumentID,c]))
    if (result.some(d => d.Status === 'processing' || d.Status === 'uploaded')) timer = setTimeout(loadDocs, 2500)
  } catch (e) { if (!disposed && request === generation) { error.value = messageOf(e); timer = setTimeout(loadDocs, 5000) } }
}
async function selectBase(base: string) { active.value = base; selectedDocs.value=[]; docs.value = []; await loadDocs() }
async function refresh() {
  loading.value = true; error.value = ''
  try {
    bases.value = (await learningApi.bases()).filter(b=>auth.user?.role!=='admin'||b.IsPublic)
    if (!bases.value.some(b => b.ID === active.value)) active.value = bases.value[0]?.ID || ''
    await loadDocs()
  } catch (e) { error.value = messageOf(e) } finally { loading.value = false }
}
async function create() {
  if (!name.value.trim() || creating.value) return
  creating.value = true
  try {
    const body={Name:name.value.trim(),Description:description.value.trim(),IsPublic:auth.user?.role==='admin'&&newBasePublic.value};const base=editingBase.value?(await api.patch(selectedBase.value?.IsPublic?`/admin/knowledge-bases/${active.value}`:`/knowledge-bases/${active.value}`,body)).data:(await api.post('/knowledge-bases',body)).data
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
    const body=new FormData();body.append('file',file.value);body.append('resource_type',resourceType.value);body.append('purpose',uploadPurpose.value)
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
    <header class="page-head"><div><p class="eyebrow">PERSONAL KNOWLEDGE BASE</p><h1>{{auth.user?.role==='admin'?'408 知识库管理':'公共与我的知识库'}}</h1><p>公共知识库由管理员维护；你可以建立个人知识库，上传、管理自己的资料。个人资料仅自己可访问。</p></div></header>
    <div v-if="error" class="notice error" role="alert">{{ error }} <button class="secondary" @click="refresh">重试</button></div>
    <p class="notice">索引方式：{{limits.index_mode==='lexical'?'本地词法向量（无需 API Key，不是语义模型）':limits.index_mode==='demo'?'演示向量':'已配置的向量模型服务'}}。替换成功后更新图谱，删除文件同步移除其索引和图谱证据。</p><button v-if="canMaintain" class="secondary" :disabled="!active" @click="editBase">修改所选知识库信息</button><button v-if="canMaintain" class="secondary" :disabled="!!pending" @click="upgradeIndex">升级本库分块与索引</button><button v-if="selectedBase && !selectedBase.IsPublic" class="secondary danger" @click="deletePersonalBase">删除个人知识库</button><button v-if="auth.user?.role==='admin'" class="secondary" :disabled="!active||!!pending" @click="retryFailed">重试本库所有失败任务</button><RouterLink v-if="active" class="secondary" :to="{path:auth.user?.role==='admin'?'/exam408/admin/graph':'/exam408/graph',query:{kb:active}}">查看所选库图谱</RouterLink><section v-if="canMaintain" class="card"><button class="secondary" :disabled="organizing||!active||!!pending" @click="organizeExisting">{{organizing?'正在整理…':'整理本库已有资料'}}</button><p>复用已保存的解析结果，不重新上传或执行 OCR；保留管理员确认的分类。只有题库或两者用途才抽题，所有候选题先审核。</p><p role="status">{{organizeMessage}}</p><ul v-if="organizeErrors.length"><li v-for="message in organizeErrors" :key="message">{{message}}</li></ul></section><section class="grid action-grid">
      <button v-if="canMaintain" class="action-tile blue" :disabled="!active || busy" @click="file = null; replaceTarget=null; showUpload = true"><span aria-hidden="true">↑</span><b>上传文件</b><small>{{ active ? 'TXT / Markdown / PDF / DOCX' : '请先创建或选择知识库' }}</small></button>
      <button v-if="auth.user?.role==='admin'||limits.personal_kb_enabled" class="action-tile purple" @click="editingBase=false;newBasePublic=auth.user?.role==='admin';name='';description='';showCreate = true"><span aria-hidden="true">＋</span><b>{{auth.user?.role==='admin'?'新建知识库':'新建个人知识库'}}</b><small>按学科或专题整理</small></button>
      <button class="action-tile orange" :disabled="loading" @click="refresh"><span aria-hidden="true">↻</span><b>{{ loading ? '刷新中…' : '刷新列表' }}</b><small>查看最新处理状态</small></button>
    </section>
    <section class="card section"><div class="section-title"><h2>可用知识库 <span class="muted">{{ bases.length }}</span></h2><input v-model="search" aria-label="搜索知识库" placeholder="搜索知识库…" /></div>
      <div v-if="loading && !bases.length" class="empty">正在加载知识库…</div>
      <div v-else-if="!bases.length" class="empty"><h3>你的学习空间，从一份资料开始</h3><p>先创建一个知识库，再上传教材或笔记。</p><button v-if="auth.user?.role==='admin'||limits.personal_kb_enabled" class="primary" @click="editingBase=false;newBasePublic=auth.user?.role==='admin';name='';description='';showCreate = true">创建第一个知识库</button></div>
      <div v-else-if="!filteredBases.length" class="empty">没有匹配的知识库。</div>
      <div class="grid kb-grid"><button v-for="kb in filteredBases" :key="kb.ID" class="kb-card" :class="{ selected: active === kb.ID }" :aria-pressed="active === kb.ID" @click="selectBase(kb.ID)"><span class="folder-mark" aria-hidden="true">▤</span><h3>{{ kb.Name }} <small>{{kb.IsPublic?'公共':'仅自己可见'}}</small></h3><p>{{ kb.Description || '个人学习资料库' }}</p><span class="muted">{{ active === kb.ID ? '当前选中' : '查看资料 →' }}</span></button></div>
    </section>
    <section class="card section"><div class="section-title"><div><h2>资料文件与分类</h2><p class="muted">{{ selectedName }}</p></div><select v-model="status" aria-label="筛选文件状态"><option value="">全部状态</option><option value="ready">已就绪</option><option value="processing">处理中</option><option value="uploaded">等待处理</option><option value="failed">处理失败</option></select></div>
      <p v-if="selectedBase?.IsPublic&&!canMaintain&&docs.some(d=>catalogs[d.ID]?.ReviewState!=='confirmed')" class="notice">部分公共资料正在等待管理员核对分类，审核完成后显示。</p><div v-if="!active" class="empty">请先创建或选择一个知识库。</div><div v-else-if="!filteredDocs.length" class="empty">{{ docs.length ? '没有符合筛选条件的文件。' : '当前知识库还没有资料，点击上方“上传文件”开始。' }}</div>
      <div class="toolbar-actions"><label>资料类型<select v-model="typeFilter"><option value="">全部类型</option><option v-for="(label,key) in typeNames" :key="key" :value="key">{{label}}</option></select></label><label>学科<select v-model="subjectFilter"><option value="">全部学科</option><option value="unassigned">待确认学科</option><option v-for="(label,key) in subjectNames" :key="key" :value="key">{{label}}</option></select></label></div><div v-if="canMaintain" class="card"><b>批量确认分类（已选 {{selectedDocs.length}} 份）</b><button class="secondary" @click="selectedDocs=filteredDocs.filter(d=>d.Status!=='processing'&&d.Status!=='uploaded').slice(0,200).map(d=>d.ID)">选择当前筛选结果（最多200份）</button><select v-model="batchType" aria-label="批量资料类型"><option v-for="(label,key) in typeNames" :key="key" :value="key">{{label}}</option></select><select v-model="batchSubject" aria-label="批量学科"><option v-for="(label,key) in subjectNames" :key="key" :value="key">{{label}}</option></select><select v-model="batchPurpose" aria-label="批量用途"><option v-for="(label,key) in purposeNames" :key="key" :value="key">{{label}}</option></select><button class="primary" :disabled="batchBusy||!selectedDocs.length" @click="batchClassify">确认所选资料</button><p>先按学科、类型筛选再确认。题目用途整理为候选题后仍需审核；不会把选择题选项作为定义。</p></div><div class="document-list"><article v-for="d in filteredDocs" :key="d.ID" class="document-card"><div class="document-heading"><div class="grow"><label v-if="canMaintain"><input type="checkbox" v-model="selectedDocs" :value="d.ID" :disabled="d.Status==='processing'||d.Status==='uploaded'" />选择此资料</label><h3>{{ d.OriginalFilename }}</h3><p class="muted">{{ (d.Size / 1024).toFixed(1) }} KB · {{ new Date(d.CreatedAt).toLocaleString() }}</p></div><span class="pill" :class="`status-${d.Status}`">{{ labels[d.Status] }}</span></div><div v-if="d.Status === 'processing' || d.Status === 'uploaded' || (d.Status === 'failed' && d.ProcessingStage)" class="processing-progress" :class="{ failed: d.Status === 'failed' }">
<div class="progress-caption"><span>{{ d.Status === 'failed' ? '处理已停止 · ' : '' }}{{ stages[d.ProcessingStage || 'queued'] || '处理中' }}</span><strong>{{ progressOf(d) }}%</strong></div>
<progress :value="progressOf(d)" max="100" :aria-label="`${d.OriginalFilename} 处理进度`">{{ progressOf(d) }}%</progress>
<small v-if="d.TotalChunks">已向量化 {{ d.ProcessedChunks || 0 }} / {{ d.TotalChunks }} 个文本块</small>
<small>阶段进度，非耗时预测；页面每 2.5 秒自动更新。</small>
</div><p v-if="d.ActiveRevisionID&&d.Status!=='ready'" class="notice">已有可用版本仍可检索；本次处理从保存的检查点继续。</p><p v-if="d.ErrorMessage" class="notice error">{{ d.ErrorMessage }}</p><p>{{typeNames[catalogs[d.ID]?.ResourceType||d.ResourceType]||'待分类'}} · {{subjectNames[catalogs[d.ID]?.SubjectCode||'']||'待确认学科'}} · {{purposeNames[catalogs[d.ID]?.Purpose||'knowledge']}} · {{catalogs[d.ID]?.ReviewState==='confirmed'?'分类已确认':'分类待核对'}}</p><div class="document-actions"><button v-if="canMaintain" class="secondary" :disabled="organizing||d.Status==='processing'" @click="editCatalog(d)">分类与用途</button><button class="secondary" :disabled="structureBusy" @click="inspectStructure(d)">查看解析结果</button><button class="secondary" @click="openDocument(d.ID).catch(e=>MessagePlugin.error(messageOf(e)))">查看原文</button><select v-if="!selectedBase?.IsPublic || auth.user?.role==='admin'" v-model="mapping[d.ID]" :aria-label="`为 ${d.OriginalFilename} 选择知识点`"><option :value="undefined">关联知识点</option><option v-for="p in points" :key="p.ID" :value="p.ID">{{ p.SubjectName }} · {{ p.Name }}</option></select><button v-if="!selectedBase?.IsPublic || auth.user?.role==='admin'" class="secondary" :disabled="!mapping[d.ID] || !!pending" @click="documentAction(d, 'map')">关联</button><button v-if="canMaintain" class="secondary" :disabled="!!pending" @click="replaceFile(d)">替换文件</button><button v-if="(!selectedBase?.IsPublic || auth.user?.role==='admin') && (d.Status === 'failed' || d.Status === 'ready')" class="secondary" :disabled="!!pending" @click="documentAction(d, 'retry')">{{d.Status==='failed'?'继续处理':'检查并更新处理'}}</button><button v-if="!selectedBase?.IsPublic || auth.user?.role==='admin'" class="secondary danger" :disabled="!!pending" @click="documentAction(d, 'delete')">删除</button></div></article></div>
    </section>
    <TDialog v-model:visible="catalogVisible" header="核对资料分类与用途" :footer="false"><form v-if="catalogDraft" @submit.prevent="saveCatalog"><label class="field">资料类型<select v-model="catalogDraft.ResourceType"><option v-for="(label,key) in typeNames" :key="key" :value="key">{{label}}</option></select></label><label class="field">学科<select aria-label="分类学科" v-model="catalogDraft.SubjectCode"><option value="">待确认</option><option v-for="(label,key) in subjectNames" :key="key" :value="key">{{label}}</option></select></label><label class="field">用途<select aria-label="分类用途" v-model="catalogDraft.Purpose"><option v-for="(label,key) in purposeNames" :key="key" :value="key">{{label}}</option></select></label><p>切换为仅知识库会将该文件自动抽取的题目撤回待审核，保留内容和历史测评。确认后点击“整理本库已有资料”刷新候选题。</p><button class="primary">确认分类</button><button type="button" class="secondary" @click="catalogDraft=null">取消</button></form></TDialog>
    <TDialog v-model:visible="showCreate" :header="editingBase?'修改知识库':'新建知识库'" :footer="false"><form @submit.prevent="create"><p v-if="auth.user?.role==='admin'&&!editingBase">此处新建公共知识库；个人知识库通过成员配额管理。</p><div class="field"><label for="base-name">名称</label><input id="base-name" v-model="name" required maxlength="100" placeholder="例如：操作系统知识库" /></div><div class="field"><label for="base-description">说明</label><textarea id="base-description" v-model="description" rows="3" placeholder="这个知识库用于什么？" /></div><button class="primary" :disabled="creating || !name.trim()">{{ creating ? '保存中…' : editingBase?'保存修改':'创建知识库' }}</button></form></TDialog>
    <TDialog v-model:visible="showUpload" :header="replaceTarget?'替换资料文件':'上传资料'" :footer="false" :close-on-overlay-click="!busy"><form @submit.prevent="upload"><p class="muted">上传至：{{ selectedName }}</p><div class="field"><label for="resource-type">资料类型</label><select id="resource-type" v-model="resourceType"><option value="auto">按文件名建议分类</option><option value="exercise">习题题集</option><option value="mock_exam">模拟试卷</option><option value="notes">笔记</option><option value="textbook">教材</option><option value="past_exam">真题</option><option value="solution">解析</option><option value="courseware">课件</option><option value="outline">考纲</option><option value="other">待分类</option></select></div><label class="field">资料用途<select v-model="uploadPurpose"><option value="">按类型选择用途（推荐）</option><option value="knowledge">知识库：检索与引用，不自动发布题目</option><option value="questions">题库：抽取候选题，不参与问答检索</option><option value="both">两者：检索并抽取候选题</option></select></label><div class="upload-zone"><label for="resource-file">选择 TXT、Markdown、PDF（含扫描件）或 Word（DOC/DOCX，含图片OCR）</label><input id="resource-file" type="file" required accept=".txt,.md,.markdown,.pdf,.doc,.docx" @change="file = ($event.target as HTMLInputElement).files?.[0] || null" /><p class="muted">最大 {{Math.floor(limits.max_upload_bytes/1024/1024)}} MB · Docker 内置 PDF 文字提取与扫描页 OCR</p></div><button class="primary" :disabled="busy || !file || !active">{{ busy ? `上传中 ${uploadProgress}%` : replaceTarget?'替换并重建索引图谱':'上传并处理' }}</button></form></TDialog>
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
