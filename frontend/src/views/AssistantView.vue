<script setup lang="ts">
import { nextTick, onMounted, onBeforeUnmount, ref } from 'vue'
import { learningApi } from '@/api/learning'
import { foundation, streamChat, openDocument, type Conversation } from '@/api/foundation'
import { messageOf } from '@/api/client'
import type { Citation, KnowledgeBase, ChatResult } from '@/api/types'
import SafeAnswer from '@/components/SafeAnswer.vue'
import CitationReader from '@/components/CitationReader.vue'
interface Message { role: 'user' | 'assistant'; content: string; sources?: Citation[]; incomplete?:boolean }
const bases = ref<KnowledgeBase[]>([]), active = ref(''), question = ref(''), conversation = ref('')
const busy = ref(false), loading = ref(true), error = ref(''), messages = ref<Message[]>([]), box = ref<HTMLElement>()
const history=ref<Conversation[]>([]),stage=ref(''),controller=ref<AbortController>()
const activeSource=ref<Citation|null>(null)
const includePersonal=ref(false)
const status = ref({ mode: 'unavailable', enabled: false, message: '' })
async function load() {
 loading.value=true;error.value=''
 try { status.value=await learningApi.assistantStatus();bases.value=await learningApi.bases();history.value=await foundation.conversations();active.value=bases.value[0]?.ID||''
 const saved=history.value.find(c=>c.ID===sessionStorage.getItem('408_conversation'));if(saved)await selectConversation(saved.ID)
 }catch(e){error.value=messageOf(e)}finally{loading.value=false}
}
function reset(){if(busy.value)return;conversation.value='';messages.value=[];error.value='';sessionStorage.removeItem('408_conversation')}
async function selectConversation(id:string){if(busy.value)return;const c=history.value.find(c=>c.ID===id);if(!c)return;loading.value=true;try{const saved=await foundation.messages(id);conversation.value=id;active.value=c.KnowledgeBaseID;messages.value=saved.map(m=>({role:m.Role,content:m.Content,sources:m.Citations??[]}));sessionStorage.setItem('408_conversation',id)}catch(e){error.value=messageOf(e)}finally{loading.value=false}}
async function rename(){const c=history.value.find(c=>c.ID===conversation.value);if(!c)return;const title=prompt('修改会话标题',c.Title);if(!title?.trim())return;try{await foundation.rename(c.ID,title);history.value=await foundation.conversations()}catch(e){error.value=messageOf(e)}}
async function remove(){if(!conversation.value||!confirm('删除此会话及聊天记录？'))return;try{await foundation.removeConversation(conversation.value);reset();history.value=await foundation.conversations()}catch(e){error.value=messageOf(e)}}
function onEnter(event:KeyboardEvent){if(event.isComposing||event.shiftKey)return;event.preventDefault();void send()}
function source(c:Citation){activeSource.value=c}
async function send(){
 const text=question.value.trim();if(!text||!active.value||busy.value||loading.value||!status.value.enabled)return
 busy.value=true;error.value='';question.value='';stage.value='正在检索资料…';controller.value=new AbortController()
 messages.value.push({role:'user',content:text});const index=messages.value.length;messages.value.push({role:'assistant',content:''})
 try{await streamChat({KnowledgeBaseID:active.value,ConversationID:conversation.value,Question:text,IncludePersonal:includePersonal.value},controller.value.signal,(event,data)=>{
 if(event==='status')stage.value=(data as {stage:string}).stage==='retrieving'?'正在检索资料…':'正在生成回答…'
 if(event==='delta')messages.value[index]!.content+=(data as {text:string}).text
 if(event==='done'){const result=data as ChatResult;conversation.value=result.ConversationID;sessionStorage.setItem('408_conversation',result.ConversationID);messages.value[index]={role:'assistant',content:result.Answer,sources:result.Sources??[]}}
 void nextTick(()=>box.value?.scrollTo({top:box.value.scrollHeight,behavior:'smooth'}))
 });history.value=await foundation.conversations()
 }catch(e){messages.value[index]!.incomplete=true;error.value=controller.value.signal.aborted?'已停止生成，本次回答可能未完成，请刷新历史记录确认（Generation stopped）':messageOf(e);question.value=text}
 finally{busy.value=false;stage.value=''}
}
onMounted(load);onBeforeUnmount(()=>controller.value?.abort())
</script>
<template>
  <div class="assistant-page">
    <header class="page-head"><div><p class="eyebrow">GROUNDED LEARNING</p><h1>智能答疑</h1><p>无需上传即可查询公共学习资料，也可加入个人资料。</p></div></header>
    <div v-if="!loading && !error && (status.mode === 'demo' || !status.enabled)" class="notice" :class="{ error: !status.enabled }" role="status"><strong>{{ status.mode === 'demo' ? '当前为资料片段预览，内容并非 AI 生成的回答。' : '智能答疑暂不可用，请稍后重试。' }}</strong><button v-if="!status.enabled" class="secondary" @click="load">重试</button></div>
    <div v-if="error" class="notice error" role="alert">{{ error }}<button v-if="!bases.length" class="secondary" @click="load">重新加载</button></div>
    <section class="card" style="max-width:1024px;margin:0 auto 16px"><label>历史会话（最近100个） <select :value="conversation" :disabled="busy||loading" @change="selectConversation(($event.target as HTMLSelectElement).value)"><option value="">选择历史会话</option><option v-for="c in history" :key="c.ID" :value="c.ID">{{c.Title}}</option></select></label><div v-if="conversation" class="toolbar-actions" style="margin-top:10px"><button class="secondary" :disabled="busy||loading" @click="rename">重命名</button><button class="secondary" :disabled="busy||loading" @click="remove">删除会话</button><small>展示最近200条消息</small></div></section><section class="chat">
      <header class="chat-toolbar"><h2>408 智能助教</h2><div class="toolbar-actions"><select v-model="active" aria-label="选择问答知识库" :disabled="busy || loading || !bases.length" @change="reset"><option value="" disabled>{{ loading ? '加载中…' : '选择知识库' }}</option><option v-for="kb in bases" :key="kb.ID" :value="kb.ID">{{ kb.Name }}{{kb.IsPublic?'（公共）':'（个人）'}}</option></select><label v-if="bases.some(b=>!b.IsPublic)"><input v-model="includePersonal" type="checkbox" :disabled="busy || loading" @change="reset" /> 同时检索所选个人库</label><button class="secondary" :disabled="busy || !messages.length" @click="reset">新对话</button></div></header>
      <div ref="box" class="messages" aria-live="polite" :aria-busy="busy">
        <div v-if="!messages.length" class="chat-welcome"><div class="welcome-icon">✦</div><h2>今天想探讨什么问题？</h2><p>默认检索公共资源。需要个人资料时，选择个人库并勾选同时检索。</p><RouterLink v-if="!loading && !bases.length" class="primary" to="/exam408/resources">先创建知识库并上传资料</RouterLink><div v-else class="suggestions"><button class="secondary" :disabled="!active || !status.enabled || loading" @click="question = '为什么需要虚拟内存？'">为什么需要虚拟内存？</button><button class="secondary" :disabled="!active || !status.enabled || loading" @click="question = 'TCP 如何实现可靠传输？'">TCP 如何实现可靠传输？</button></div></div>
        <template v-for="(m, i) in messages" :key="i"><div class="message" :class="m.role"><small>{{ m.role === 'user' ? '你' : status.mode === 'demo' ? '片段预览（非 AI 回答）' : '408 助教' }}</small><SafeAnswer v-if="m.role==='assistant'" :text="m.content" :citations="m.sources?.length??0" @cite="n=>m.sources?.[n]&&source(m.sources[n]!)"/><span v-else>{{m.content}}</span><small v-if="m.incomplete">回答未完成（Incomplete）</small></div><div v-if="m.sources?.length" class="sources"><details v-for="(s, n) in m.sources" :key="s.ChunkID" class="source"><summary>[{{ n + 1 }}] {{ s.DocumentTitle }} · {{ s.Location }}</summary><p>{{ s.Excerpt }}</p><button class="secondary" @click="source(s)">查看原文{{s.Page?' · 第'+s.Page+'页':''}}</button></details></div></template>
        <p v-if="busy" class="muted">{{stage}} <button class="secondary" @click="controller?.abort()">停止生成</button></p>
      </div>
      <form class="composer" @submit.prevent="send"><label class="sr-only" for="question">向助教提问</label><textarea id="question" v-model="question" rows="2" :disabled="!active || busy || !status.enabled || loading" placeholder="发送问题给 408 助教…" @keydown.enter="onEnter" /><button class="primary" :disabled="!active || busy || !question.trim() || !status.enabled || loading">{{ busy ? '回答中…' : '发送 ↑' }}</button></form>
      <p class="chat-footnote">优先检索系统公共资源；勾选后加入所选个人库 · 请结合引用原文复核 · Shift + Enter 换行</p>
    </section><CitationReader :source="activeSource" @close="activeSource=null"/>
  </div>
</template>
