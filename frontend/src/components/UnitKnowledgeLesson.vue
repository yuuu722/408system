<script setup lang="ts">
import {ref,watch,onMounted,onBeforeUnmount} from 'vue'
import {teachingApi} from '@/api/foundation'
import {messageOf} from '@/api/client'
import type {ChatResult,Citation} from '@/api/types'
import SafeAnswer from '@/components/SafeAnswer.vue'
import SpeakButton from '@/components/SpeakButton.vue'
import CitationReader from '@/components/CitationReader.vue'
const props=defineProps<{pointId:string;resourceKey:string;sources:Citation[]}>(),emit=defineEmits<{ready:[value:boolean]}>()
const root=ref<HTMLElement|null>(null),lesson=ref<ChatResult|null>(null),busy=ref(false),error=ref(''),source=ref<Citation|null>(null)
let version=0,visible=false,observer:IntersectionObserver|null=null
async function load(){const id=++version;busy.value=true;error.value='';emit('ready',false);try{let response=await teachingApi.lesson(props.pointId);if(!response.Lesson)response=await teachingApi.lesson(props.pointId,true);if(id!==version)return;if(!response.Lesson)throw Error('讲解尚未整理完成，请重试');lesson.value=response.Lesson;emit('ready',true)}catch(e){if(id===version)error.value=messageOf(e)}finally{if(id===version)busy.value=false}}
watch(()=>[props.pointId,props.resourceKey],()=>{version++;lesson.value=null;source.value=null;error.value='';emit('ready',false);if(visible)load()})
onMounted(()=>{if(!('IntersectionObserver'in window)){visible=true;load();return}observer=new IntersectionObserver(entries=>{if(entries.some(e=>e.isIntersecting)){visible=true;observer?.disconnect();load()}},{rootMargin:'100px'});if(root.value)observer.observe(root.value)})
onBeforeUnmount(()=>{version++;observer?.disconnect()})
</script>
<template><section ref="root" class="knowledge-lesson" aria-label="完整知识讲解"><p v-if="busy" role="status">正在整理本节完整讲解，完成后会保存，下次可直接阅读。</p><p v-if="error" class="notice error" role="alert">知识讲解暂未完成：{{error}} <button class="secondary" :disabled="busy" @click="load">重试讲解</button></p><template v-if="lesson"><SafeAnswer :text="lesson.Answer" :citations="lesson.Sources.length" @cite="n=>source=lesson!.Sources[n]??null"/><SpeakButton :text="lesson.Answer"/></template><details v-if="lesson?.Sources.length||sources.length"><summary>教材来源与原文（{{lesson?.Sources.length||sources.length}}）</summary><button v-for="(c,n) in lesson?.Sources??sources" :key="n" class="secondary" @click="source=c">{{c.DocumentTitle}} · {{c.Location||`第${c.Page}页`}}</button></details><CitationReader :source="source" @close="source=null"/></section></template>
<style scoped>.knowledge-lesson{padding:16px;background:#f8fafc;border-radius:12px}.knowledge-lesson details{margin-top:16px}.knowledge-lesson summary{cursor:pointer;color:#2563eb}.knowledge-lesson button{margin:8px 8px 0 0;max-width:100%}</style>
