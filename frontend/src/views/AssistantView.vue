<script setup lang="ts">
import { nextTick, onMounted, onBeforeUnmount, ref } from 'vue'
import { learningApi } from '@/api/learning'
import { foundation, streamChat, openDocument, type Conversation } from '@/api/foundation'
import { api, messageOf } from '@/api/client'
import type { Citation, KnowledgeBase, ChatResult,RetrievalTrace } from '@/api/types'
import {useAuthStore} from '@/stores/auth'
import SafeAnswer from '@/components/SafeAnswer.vue'
import CitationReader from '@/components/CitationReader.vue'
interface Understanding {question:string;intent:string;knowledge_points:string[]}
interface Message {reasoning?:RetrievalTrace; understanding?:Understanding; role: 'user' | 'assistant'; content: string; sources?: Citation[]; incomplete?:boolean }
const bases = ref<KnowledgeBase[]>([]), active = ref(''), question = ref(''), conversation = ref('')
const busy = ref(false), loading = ref(true), error = ref(''), messages = ref<Message[]>([]), box = ref<HTMLElement>()
const history=ref<Conversation[]>([]),stage=ref(''),controller=ref<AbortController>()
const activeSource=ref<Citation|null>(null)
const auth=useAuthStore(),checking=ref(false),checkReport=ref<{passed:boolean;checks:{name:string;passed:boolean;message:string}[]}|null>(null)
async function checkConnection(){if(checking.value||busy.value)return;checking.value=true;error.value='';checkReport.value=null;try{checkReport.value=(await api.post('/admin/assistant/check',{}, {timeout:55000})).data}catch(e){error.value=messageOf(e)}finally{checking.value=false}}

const imageFile=ref<File|null>(null),imageURL=ref(''),recognizing=ref(false),recognized=ref(false),imageWarnings=ref<string[]>([]),imageMethod=ref('')
const imageController=ref<AbortController>()
function clearImage(){imageController.value?.abort();imageController.value=undefined;if(imageURL.value)URL.revokeObjectURL(imageURL.value);imageURL.value='';imageFile.value=null;recognized.value=false;imageWarnings.value=[];imageMethod.value=''}
function chooseImage(event:Event){const input=event.target as HTMLInputElement,file=input.files?.[0];input.value='';if(!file)return;clearImage();if(!['image/png','image/jpeg'].includes(file.type)||file.size>10*1024*1024){error.value='请选择不超过10 MiB的PNG/JPEG题目图片';return};error.value='';imageFile.value=file;imageURL.value=URL.createObjectURL(file)}
async function recognizeImage(){
 if(!imageFile.value||!active.value||recognizing.value||busy.value)return
 recognizing.value=true;error.value='';const file=imageFile.value;imageController.value=new AbortController()
 try{
  const body=new FormData();body.append('file',file);body.append('KnowledgeBaseID',active.value)
  const {data}=await api.post<{text:string;method:string;warnings:string[]}>('/exam408/assistant/recognize',body,{signal:imageController.value.signal,timeout:120000})
  if(imageFile.value!==file)return
  question.value=[question.value.trim(),data.text].filter(Boolean).join('\n\n');recognized.value=true;imageWarnings.value=data.warnings??[];imageMethod.value=data.method
 }catch(e){if(imageFile.value===file)error.value=messageOf(e)}finally{recognizing.value=false}
}

const status = ref({ mode: 'unavailable', enabled: false, message: '' })
async function load() {
 loading.value=true;error.value=''
 try { status.value=await learningApi.assistantStatus();bases.value=await learningApi.bases();history.value=await foundation.conversations();active.value=bases.value[0]?.ID||''
 const saved=history.value.find(c=>c.ID===sessionStorage.getItem('408_conversation'));if(saved)await selectConversation(saved.ID)
 }catch(e){error.value=messageOf(e)}finally{loading.value=false}
}
function reset(){if(busy.value||recognizing.value)return;clearImage();conversation.value='';messages.value=[];error.value='';sessionStorage.removeItem('408_conversation')}
async function selectConversation(id:string){if(busy.value||recognizing.value)return;clearImage();const c=history.value.find(c=>c.ID===id);if(!c)return;loading.value=true;try{const saved=await foundation.messages(id);conversation.value=id;active.value=c.KnowledgeBaseID;messages.value=saved.map(m=>({role:m.Role,content:m.Content,sources:m.Citations??[]}));sessionStorage.setItem('408_conversation',id)}catch(e){error.value=messageOf(e)}finally{loading.value=false}}
async function rename(){const c=history.value.find(c=>c.ID===conversation.value);if(!c)return;const title=prompt('修改会话标题',c.Title);if(!title?.trim())return;try{await foundation.rename(c.ID,title);history.value=await foundation.conversations()}catch(e){error.value=messageOf(e)}}
async function remove(){if(!conversation.value||!confirm('删除此会话及聊天记录？'))return;try{await foundation.removeConversation(conversation.value);reset();history.value=await foundation.conversations()}catch(e){error.value=messageOf(e)}}
function onEnter(event:KeyboardEvent){if(event.isComposing||event.shiftKey)return;event.preventDefault();void send()}
function source(c:Citation){activeSource.value=c}
async function send(){
 const text=question.value.trim();if(!text||!active.value||busy.value||recognizing.value||loading.value||!status.value.enabled||imageFile.value&&!recognized.value)return
 if(text.length>4000){error.value='问题最多4000字，请裁剪题目或精简文字';return}
 busy.value=true;error.value='';question.value='';stage.value='正在理解问题…';controller.value=new AbortController()
 messages.value.push({role:'user',content:text});const index=messages.value.length;messages.value.push({role:'assistant',content:''})
 try{await streamChat({KnowledgeBaseID:active.value,ConversationID:conversation.value,Question:text},controller.value.signal,(event,data)=>{
 if(event==='status')stage.value=({understanding:'正在理解问题…',graph:'正在规划问题并查询图谱路径…',retrieving:'正在所选知识库中检索依据…',generating:'正在根据原文组织回答…'} as Record<string,string>)[(data as {stage:string}).stage]||'正在处理…'
 if(event==='reasoning')messages.value[index]!.reasoning=data as RetrievalTrace
 if(event==='understanding')messages.value[index]!.understanding=data as Understanding
 if(event==='delta')messages.value[index]!.content+=(data as {text:string}).text
 if(event==='done'){const result=data as ChatResult;conversation.value=result.ConversationID;sessionStorage.setItem('408_conversation',result.ConversationID);messages.value[index]={role:'assistant',content:result.Answer,sources:result.Sources??[],understanding:messages.value[index]?.understanding,reasoning:result.Reasoning??messages.value[index]?.reasoning};clearImage()}
 void nextTick(()=>box.value?.scrollTo({top:box.value.scrollHeight,behavior:'smooth'}))
 });history.value=await foundation.conversations()
 }catch(e){messages.value[index]!.incomplete=true;error.value=controller.value.signal.aborted?'已停止生成，本次回答可能未完成，请刷新历史记录确认（Generation stopped）':messageOf(e);question.value=text}
 finally{busy.value=false;stage.value=''}
}
onMounted(load);onBeforeUnmount(()=>{controller.value?.abort();clearImage()})
</script>
<template>
  <div class="assistant-page">
    <header class="page-head"><div><p class="eyebrow">GROUNDED LEARNING</p><h1>智能答疑</h1><p>选择知识库后提问，AI 仅整理检索资料，并提供原文出处。</p></div></header>
    <div v-if="!loading && !error && (status.mode === 'demo' || !status.enabled)" class="notice" :class="{ error: !status.enabled }" role="status"><strong>{{ status.mode === 'demo' ? '当前为资料片段预览，内容并非 AI 生成的回答。' : status.message || '智能答疑暂不可用，请稍后重试。' }}</strong><button v-if="!status.enabled" class="secondary" @click="load">重试</button></div>
    <section v-if="!loading && !status.enabled && status.message" class="notice" role="note">
      <template v-if="auth.user?.role==='admin'"><b>管理员配置说明</b><p>请在部署服务器的 .env 中配置有效的 LLM_API_KEY，并核对 LLM_BASE_URL 和 LLM_MODEL。保存后重建后端容器使配置生效，再刷新本页并检测连接。更新源码不会自动填写模型密钥，请勿将密钥提交到 GitHub。</p></template>
      <template v-else><b>问答尚未启用</b><p>请联系管理员完成模型配置。模型由服务端统一配置，普通用户无需修改 .env；配置生效后刷新本页即可提问。已上传的知识库资料仍然保留。</p></template>
    </section>
    <section v-if="auth.user?.role==='admin'" class="notice"><button class="secondary" :disabled="checking||busy" @click="checkConnection">{{checking?'正在检测检索与回答模型…':'检测问答连接'}}</button><small>会向已配置的模型发送一次简短连接测试，可能产生模型调用费用；不发送知识库资料。</small><ul v-if="checkReport"><li v-for="c in checkReport.checks" :key="c.name"><b>{{c.passed?'✓':'×'}} {{c.name}}</b>：{{c.message}}</li></ul></section>
    <div v-if="error" class="notice error" role="alert">{{ error }}<button v-if="!bases.length" class="secondary" @click="load">重新加载</button></div>
    <section class="card" style="max-width:1024px;margin:0 auto 16px"><label>历史会话（最近100个） <select :value="conversation" :disabled="busy||recognizing||loading" @change="selectConversation(($event.target as HTMLSelectElement).value)"><option value="">选择历史会话</option><option v-for="c in history" :key="c.ID" :value="c.ID">{{c.Title}}</option></select></label><div v-if="conversation" class="toolbar-actions" style="margin-top:10px"><button class="secondary" :disabled="busy||recognizing||loading" @click="rename">重命名</button><button class="secondary" :disabled="busy||recognizing||loading" @click="remove">删除会话</button><small>展示最近200条消息</small></div></section><section class="chat">
      <header class="chat-toolbar"><h2>408 智能助教</h2><div class="toolbar-actions"><select v-model="active" aria-label="选择问答知识库" :disabled="busy || recognizing || loading || !bases.length" @change="reset"><option value="" disabled>{{ loading ? '加载中…' : '选择知识库' }}</option><option v-for="kb in bases" :key="kb.ID" :value="kb.ID">{{ kb.Name }}{{kb.IsPublic?'（公共）':'（个人）'}}</option></select><button class="secondary" :disabled="busy || recognizing || !messages.length" @click="reset">新对话</button></div></header>
      <div ref="box" class="messages" aria-live="polite" :aria-busy="busy">
        <div v-if="!messages.length" class="chat-welcome"><div class="welcome-icon">✦</div><h2>今天想探讨什么问题？</h2><p>仅检索当前选择的知识库。没有足够依据时会明确说明，不使用 AI 常识补齐答案。</p><RouterLink v-if="!loading && !bases.length" class="primary" to="/exam408/resources">先创建知识库并上传资料</RouterLink><div v-else class="suggestions"><button class="secondary" :disabled="!active || !status.enabled || loading" @click="question = '为什么需要虚拟内存？'">为什么需要虚拟内存？</button><button class="secondary" :disabled="!active || !status.enabled || loading" @click="question = 'TCP 如何实现可靠传输？'">TCP 如何实现可靠传输？</button></div></div>
        <template v-for="(m, i) in messages" :key="i"><div class="message" :class="m.role"><small>{{ m.role === 'user' ? '你' : status.mode === 'demo' ? '片段预览（非 AI 回答）' : '408 助教' }}</small><details v-if="m.understanding" class="question-understanding"><summary>问题分析</summary><div>理解问题：{{m.understanding.intent}}<span v-if="m.understanding.knowledge_points?.length"> · 检索概念：{{m.understanding.knowledge_points.join('、')}}</span><small>仅用于定位资料，答案以引用原文为依据。</small></div></details><details v-if="m.reasoning" class="question-understanding"><summary>KAG 检索路径</summary><p>{{m.reasoning.Notice}}</p><p>检索任务：{{m.reasoning.Subquestions.join('；')}}</p><ul><li v-for="(path,n) in m.reasoning.Paths.slice(0,8)" :key="n">{{path.Nodes.map(node=>node.Name).join(' ↔ ')}}<small> {{path.Edges.map(edge=>edge.RelationType).join(' / ')}}</small></li></ul></details><SafeAnswer v-if="m.role==='assistant'" :text="m.content" :citations="m.sources?.length??0" @cite="n=>m.sources?.[n]&&source(m.sources[n]!)"/><span v-else>{{m.content}}</span><small v-if="m.incomplete">回答未完成（Incomplete）</small></div><div v-if="m.sources?.length" class="sources"><h3>参考来源</h3><p class="muted">以下来源对应上方回答，点击查看文件、页码和原文。</p><details v-for="(s, n) in m.sources" :key="s.ChunkID+':'+s.Page+':'+s.Location" class="source"><summary>[{{ n + 1 }}] {{ s.DocumentTitle }} · {{ s.Location }}</summary><p>{{ s.Excerpt }}</p><button class="secondary" @click="source(s)">查看原文{{s.Page?' · 第'+s.Page+'页':''}}</button></details></div></template>
        <p v-if="busy" class="muted">{{stage}} <button class="secondary" @click="controller?.abort()">停止生成</button></p>
      </div>
      <section class="question-image-panel"><div class="question-input-tools"><label class="secondary image-picker">添加题目图片<input type="file" accept="image/png,image/jpeg" :disabled="busy||recognizing||loading||!active" @change="chooseImage" /></label><span>PNG / JPEG，最大10 MiB · 可输入文字，或先识别图片再核对题目</span></div>
        <div v-if="imageFile" class="question-image-preview"><img :src="imageURL" alt="待识别的题目图片"/><div><b>{{imageFile.name}}</b><p>先识别题目，再核对下方文字。图片不会进入知识库或作为引用来源。</p><button class="secondary" :disabled="recognizing||recognized||busy" @click="recognizeImage">{{recognizing?'识别中…':recognized?'识别完成，请核对文字':'识别图片中的题目'}}</button><button class="secondary" :disabled="recognizing||busy" @click="clearImage">移除图片</button><small v-if="recognized">{{imageMethod==='vision'?'视觉模型转写':'本地文字识别'}} · 历史会话保存核对后的文字，不保存原图。</small></div></div>
        <p v-for="warning in imageWarnings" :key="warning" class="image-warning">{{warning}}</p>
      </section>
      <form class="composer" @submit.prevent="send"><label class="sr-only" for="question">向助教提问或核对图片识别文字</label><textarea id="question" v-model="question" rows="3" :disabled="!active || busy || recognizing || loading" placeholder="输入问题；图片识别后，请在此核对题干、公式和图示条件…" @keydown.enter="onEnter" /><button class="primary" :disabled="!active || busy || recognizing || !question.trim() || !status.enabled || loading || !!imageFile&&!recognized">{{ busy ? '回答中…' : '检索并回答 ↑' }}</button></form>
      <p class="chat-footnote">仅检索所选知识库 · 点击引用编号查看文件、页码及原文片段 · Shift + Enter 换行</p>
    </section><CitationReader :source="activeSource" @close="activeSource=null"/>
  </div>
</template>

<style scoped>
.question-image-panel{padding:14px 20px 0;border-top:1px solid #e7edf5}.question-input-tools{display:flex;gap:12px;align-items:center;flex-wrap:wrap}.question-input-tools>span{font-size:12px;color:#8997a9}.image-picker{position:relative;cursor:pointer;overflow:hidden}.image-picker input{position:absolute;inset:0;opacity:0;width:100%;height:100%;cursor:pointer}.image-picker:focus-within{outline:2px solid #2563eb;outline-offset:2px}.question-image-preview{display:flex;gap:16px;padding:14px;background:#f5f8fd;border-radius:10px;margin:12px 0}.question-image-preview img{width:140px;height:130px;object-fit:contain;background:white}.question-image-preview>div{min-width:0}.question-image-preview b{overflow-wrap:anywhere}.question-image-preview p,.image-warning{font-size:12px;color:#77869b;line-height:1.7}.question-image-preview button{margin:0 8px 8px 0}.question-image-preview small{display:block;color:#8594a8}.question-understanding{padding:9px 12px;margin:8px 0 12px;border-left:3px solid #9ebae3;background:#f1f6ff;color:#577495;font-size:12px}.question-understanding small{display:block;margin-top:6px;color:#8a99ab}@media(max-width:600px){.question-image-preview{flex-direction:column}.question-image-panel{padding:12px}}
</style>
