<script setup lang="ts">
import {ref,watch,onBeforeUnmount} from 'vue'
import {pointStudyApi,teachers,type PointStudy} from '@/api/coaching'
import {learningApi} from '@/api/learning'
import {messageOf,clearPageReads} from '@/api/client'
import type {Citation,ChatResult,KnowledgeBase} from '@/api/types'
import ExampleExercise from '@/components/ExampleExercise.vue'
import InlineAssessment from '@/components/InlineAssessment.vue'
import {foundation,type Assessment,type Asset} from '@/api/foundation'
import SafeAnswer from '@/components/SafeAnswer.vue'
import SpeakButton from '@/components/SpeakButton.vue'
import CitationReader from '@/components/CitationReader.vue'
const props=defineProps<{pointId:string}>(),emit=defineEmits<{select:[id:string]}>()
const data=ref<PointStudy>(),loading=ref(false),error=ref(''),teacher=ref('encouraging'),bases=ref<KnowledgeBase[]>([]),kb=ref(''),teaching=ref(false),lesson=ref<ChatResult|null>(null),teachError=ref(''),source=ref<Citation|null>(null)
const practice=ref<Assessment|null>(null),practiceBusy=ref(false),practiceError=ref('')
let request=0
async function openPractice(q:Asset){
 const id=request
 practiceBusy.value=true;practiceError.value=''
 try{const a=await foundation.start('practice',q.PointID,q.ID);if(id===request)practice.value=a}
 catch(e){if(id===request)practiceError.value=messageOf(e)}
 finally{if(id===request)practiceBusy.value=false}
}
async function load(){const id=++request;loading.value=true;error.value='';data.value=undefined;lesson.value=null;source.value=null;teachError.value='';teaching.value=false;practice.value=null;practiceBusy.value=false;practiceError.value=''
 try{const pack=await pointStudyApi.load(props.pointId);if(id===request){data.value=pack;if(pack.PreferredKnowledgeBaseID&&bases.value.some(b=>b.ID===pack.PreferredKnowledgeBaseID))kb.value=pack.PreferredKnowledgeBaseID}}catch(e){if(id===request)error.value=messageOf(e)}finally{if(id===request)loading.value=false}
}
watch(()=>props.pointId,()=>load(),{immediate:true})
async function prepareBases(){try{bases.value=await learningApi.bases();kb.value=bases.value.find(b=>b.ID===data.value?.PreferredKnowledgeBaseID)?.ID??bases.value.find(b=>b.IsPublic)?.ID??bases.value[0]?.ID??''}catch(e){teachError.value='教材内容仍可阅读；讲解资料范围读取失败：'+messageOf(e)}}
prepareBases()
function openSource(c:Citation){
 if(c.WebURL){
  try{const url=new URL(c.WebURL);if(url.protocol==='https:')window.open(url.href,'_blank','noopener,noreferrer')}catch{}
  return
 }
 source.value=c
}
async function teach(){
 if(!kb.value||!data.value)return
 const id=request
 teaching.value=true;teachError.value='';lesson.value=null
 try{const answer=await pointStudyApi.lesson(props.pointId,kb.value,teacher.value);if(id===request)lesson.value=answer}
 catch(e){if(id===request)teachError.value='老师讲解暂未完成：'+messageOf(e)}
 finally{if(id===request)teaching.value=false}
}
async function refreshStudy(){const id=request;try{const next=await pointStudyApi.load(props.pointId);if(id===request)data.value=next}catch(e){if(id===request)error.value=messageOf(e)}}

onBeforeUnmount(()=>request++)
</script>
<template><section class="point-study" aria-label="知识点学习内容"><p v-if="loading" role="status">正在定位教材、错题与练习…</p><p v-if="error" class="notice error" role="alert">{{error}} <button class="secondary" @click="clearPageReads();load()">重试</button></p><template v-if="data">
<div v-if="data.Ancestors.length" class="path"><span>知识目录：</span><button v-for="p in data.Ancestors" :key="p.ID" class="text-link" @click="emit('select',p.ID)">{{p.Name}} ›</button><b>{{data.Point.Name}}</b></div>
<p class="coverage">本点 {{data.DirectQuestions}} 个可靠题组 · 含下级共 {{data.SubtreeQuestions}} 个；当前可选新练习 {{data.Exercises.length}} 题。</p>
<details v-if="data.Children.length"><summary>继续细分学习（{{data.Children.length}} 个下级知识点）</summary><div class="actions"><button v-for="child in data.Children" :key="child.ID" class="secondary" @click="emit('select',child.ID)">{{child.Name}}</button></div></details>
<details v-if="data.Related?.length"><summary>相关知识点（{{data.Related.length}}）</summary><div class="actions"><button v-for="item in data.Related" :key="item.ID" class="secondary" @click="emit('select',item.ID)">{{item.Name}}</button></div></details>
<section class="study-section"><h3>先复查：最近有效错题</h3><p v-if="!data.Mistakes.length" class="muted">本点及下级暂无当前有效错题记录；这不代表全部掌握，未测内容仍需诊断。</p><details v-for="(wrong,n) in data.Mistakes" :open="n===0" :key="wrong.AssessmentID+wrong.Question.ID"><summary>{{wrong.Question.Title}}</summary><p class="stem">{{wrong.Question.Payload.Stem}}</p><p v-if="data.PointNames?.[wrong.Question.PointID]" class="muted">所属知识点：{{data.PointNames[wrong.Question.PointID]}}</p><p v-for="(o,i) in wrong.Question.Payload.Options" :key="i">{{String.fromCharCode(65+i)}}. {{o}}</p><img v-for="(img,i) in wrong.Question.Payload.Images??[]" :key="i" :src="img" alt="错题原图"/><p>你的答案：{{wrong.Result.Selected<0?'未掌握 / 不确定':String.fromCharCode(65+wrong.Result.Selected)}} · 参考答案：{{wrong.Result.Answer<0?'尚无可核对答案':String.fromCharCode(65+wrong.Result.Answer)}}</p><p v-if="wrong.Result.AnswerOrigin&&wrong.Result.AnswerOrigin!=='source'" class="muted">本次记录含用户确认或AI辅助判断，并非资料标准答案，请结合原文核对。</p><SafeAnswer :text="wrong.Result.Explanation"/><RouterLink class="secondary" :to="{path:'/exam408/assessment',query:{assessment:wrong.AssessmentID}}">查看完整报告与题目</RouterLink></details></section>
<section class="study-section"><h3>{{data.Point.Name}}</h3>
<SafeAnswer v-if="data.Teachings?.length" :text="data.Teachings[0].Payload.Stem+'\n\n'+data.Teachings[0].Payload.Explanation"/>
<template v-else><b>考纲范围</b><p>{{data.Point.Description||data.Point.Name}}</p></template>
<details v-if="data.Knowledge.length"><summary>教材原页</summary><button v-for="c in data.Knowledge" :key="c.ChunkID" class="secondary" @click="source=c">{{c.DocumentTitle}} · {{(c.Page??0)>0?'第'+c.Page+'页':'查看原文'}}</button></details>
<div class="teacher-controls"><label>知识讲解风格<select v-model="teacher" :disabled="teaching"><option v-for="t in teachers" :key="t.id" :value="t.id">{{t.name}}</option></select></label><label>资料范围<select v-model="kb" :disabled="teaching"><option v-for="b in bases" :key="b.ID" :value="b.ID">{{b.Name}}{{b.IsPublic?'':' ＋ 公共教材'}}</option></select></label><button class="primary" :disabled="teaching||!kb" @click="teach()">{{teaching?'老师正在整理…':'请老师在这里讲解'}}</button></div><p v-if="teaching" role="status">老师正在检索教材与网页并整理讲解，原文与练习仍可查看。</p><p v-if="!kb" class="muted">暂无可用的讲解知识库，请先检查资料知识库。</p><p v-if="teachError" class="notice error" role="alert">{{teachError}}</p><article v-if="lesson" class="teacher-answer"><h4>{{data.Point.Name}} · 老师讲解</h4><p v-if="lesson.Reasoning?.WebStatus==='no_usable_results'||lesson.Reasoning?.WebStatus==='failed'" class="muted">联网没有提供可用证据，本次以教材为依据。</p><p v-else-if="lesson.Reasoning?.WebStatus==='disabled'" class="muted">联网尚未配置。</p><p v-if="lesson.Generation&&!lesson.Generation.Verified" class="muted">本次尚未形成通过核验的老师讲解，以下保留服务返回的教材或诊断信息，请核对来源。</p><SafeAnswer :text="lesson.Answer" :citations="lesson.Sources.length" @cite="n=>{const c=lesson?.Sources[n];if(c)openSource(c)}"/><SpeakButton :key="pointId+lesson.ConversationID" :text="lesson.Answer"/><details v-if="lesson.Sources.length"><summary>讲解来源</summary><button v-for="(c,n) in lesson.Sources" :key="n" class="secondary" @click="openSource(c)">{{c.DocumentTitle}} · {{c.WebURL?'网页':c.Location||`第${c.Page}页`}}</button></details></article></section>
<section class="study-section"><h3>例题：先作答，再核对讲解</h3><p v-if="!data.Example" class="muted">{{data.Missing.find(s=>s.includes('例题'))??'暂无有可靠答案与解析的参考例题。'}} 可先阅读教材或复查已有错题。</p><template v-else><p v-if="data.Example.PointID!==pointId" class="muted">例题所属知识点：{{data.PointNames?.[data.Example.PointID]??'下级概念'}}</p><ExampleExercise :point-id="pointId" :question="data.Example" @answered="refreshStudy"/></template></section>
<section class="study-section"><h3>再练习：独立完成新题</h3><p>练习与例题分开；提交后查看解析，作答结果会更新学情。</p><p v-if="!data.Exercises.length" class="muted">暂无未接触的可靠练习题组；可先复查错题。已有题目不冒充新题。</p><button v-for="q in data.Exercises" :key="q.ID" class="secondary practice-link" :disabled="practiceBusy" @click="openPractice(q)"><b>{{q.Title}}</b><small>{{data.PointNames?.[q.PointID]}}</small></button>
<p v-if="practiceBusy" role="status">正在打开练习…</p>
<p v-if="practiceError" class="notice error" role="alert">{{practiceError}}</p>
<InlineAssessment v-if="practice" :assessment="practice" label="知识点练习" @submitted="practice=$event;refreshStudy()"/><div class="actions"><RouterLink v-if="data.DirectQuestions>0||data.Example" class="secondary" :to="{path:'/exam408/tasks',query:{tab:'teacher',point:data.DirectQuestions>0?pointId:data.Example?.PointID}}">{{data.DirectQuestions>0?'进入随堂教学与提问':'进入例题知识点的随堂教学'}}</RouterLink><RouterLink v-if="data.SubtreeQuestions>0" class="secondary" :to="{path:'/exam408/assets',query:{point:pointId,kind:'question'}}">查看本点与下级题库</RouterLink><RouterLink class="secondary" to="/exam408/tasks">查看复习单元与新题检查</RouterLink></div></section>
</template><CitationReader :source="source" @close="source=null"/></section></template>
<style scoped>.point-study{min-width:0;overflow-wrap:anywhere}.study-section{padding:20px 0;border-bottom:1px solid #e2e8f0}.study-section h3{margin-top:0}.coverage{background:#eff6ff;padding:12px;border-radius:9px}.point-study details{padding:12px;margin:10px 0;background:#f8fafc;border-radius:10px}.point-study summary{cursor:pointer;font-weight:600}.stem{white-space:pre-wrap;line-height:1.9}.point-study img{max-width:100%;max-height:400px;object-fit:contain}.actions,.path{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0}.text-link{border:0;background:none;color:#2563eb;padding:0}.teacher-controls{display:flex;gap:10px;align-items:end;flex-wrap:wrap}.teacher-controls label{display:grid;gap:6px;flex:1;min-width:150px}.teacher-controls select{width:100%;min-width:0}.teacher-answer{padding:16px;background:#fff;border:1px solid #dbeafe;border-radius:12px;margin-top:16px}.practice-link{display:grid;gap:8px;border:1px solid #dbeafe;padding:16px;border-radius:12px;margin:12px 0;color:#26374c}.practice-link span{white-space:pre-wrap}.practice-link strong{color:#2563eb}.point-study .secondary{max-width:100%}@media(max-width:600px){.teacher-controls{display:grid}.teacher-controls label{min-width:0}}</style>
