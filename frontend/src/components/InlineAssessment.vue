<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { foundation, type Assessment } from '@/api/foundation'
import { messageOf } from '@/api/client'
import SafeAnswer from '@/components/SafeAnswer.vue'
import CitationReader from '@/components/CitationReader.vue'
import type { Citation } from '@/api/types'

const props=defineProps<{assessment:Assessment;label:string}>()
const emit=defineEmits<{submitted:[assessment:Assessment]}>()
const result=ref(props.assessment.Result), answers=ref<Record<string,number>>({})
const busy=ref(false),error=ref(''),source=ref<Citation|null>(null)
watch(()=>props.assessment.ID,()=>{result.value=props.assessment.Result;answers.value={};error.value='';busy.value=false})
const complete=computed(()=>props.assessment.Questions.every(q=>answers.value[q.ID]!==undefined))
const missed=computed(()=>result.value?.Items.filter(i=>i.Counted&&!i.Correct)??[])
function letter(value:number){return value<0?'不确定':String.fromCharCode(65+value)}
async function submit(){
 if(busy.value||result.value||!complete.value)return
 const a=props.assessment
 busy.value=true;error.value=''
 try{
  const saved=await foundation.submit(a.ID,answers.value)
  if(a.ID===props.assessment.ID){result.value=saved;emit('submitted',{...a,Result:saved})}
 }catch(e){if(a.ID===props.assessment.ID)error.value=messageOf(e)}
 finally{if(a.ID===props.assessment.ID)busy.value=false}
}
</script>
<template>
 <form class="inline-assessment" @submit.prevent="submit">
  <h4>{{label}}</h4>
  <fieldset v-for="q in assessment.Questions" :key="q.ID">
   <legend>{{q.Title}}</legend>
   <SafeAnswer :text="q.Payload.Stem"/>
   <img v-for="(img,index) in q.Payload.Images??[]" :key="index" :src="img" alt="题目原图">
   <label v-for="(option,i) in q.Payload.Options" :key="i">
    <input :checked="result ? result.Items.find(item=>item.ID===q.ID)?.Selected===i : answers[q.ID]===i" @change="answers[q.ID]=i" :name="assessment.ID+q.ID" type="radio" :value="i" :disabled="busy||!!result">
    {{letter(i)}}. {{option}}
   </label>
   <label v-if="!result"><input v-model="answers[q.ID]" :name="assessment.ID+q.ID" type="radio" :value="-1" :disabled="busy">不确定</label>
   <template v-for="item in result?.Items.filter(i=>i.ID===q.ID)??[]" :key="item.ID">
    <p>{{item.Counted?(item.Correct?'回答正确':'需要订正'):'本题暂无可靠判分'}}。你的选择 {{letter(item.Selected)}}<template v-if="item.Answer>=0"> · 参考答案 {{letter(item.Answer)}}</template></p>
    <SafeAnswer v-if="item.Explanation" :text="item.Explanation"/>
    <button v-for="(c,index) in q.Payload.Evidence??[]" :key="index" type="button" class="secondary" @click="source=c">{{c.DocumentTitle}} · 原页</button>
   </template>
  </fieldset>
  <button v-if="!result" class="primary" :disabled="busy||!complete">{{busy?'提交中…':'提交并查看订正'}}</button>
  <p v-if="error" role="alert" class="notice error">{{error}}</p>
  <section v-if="result" class="notice" aria-label="本次学习小结">
   <strong>本次学习小结</strong>
   <p>已保存作答 · 有效判分 {{result.Counted??result.Total}} 题 · 答对 {{result.Correct}} 题。</p>
   <template v-if="missed.length"><b>易错点与订正依据</b><ul><li v-for="item in missed" :key="item.ID"><b>{{assessment.Questions.find(q=>q.ID===item.ID)?.Title}}</b><SafeAnswer v-if="item.Explanation" :text="item.Explanation"/></li></ul><p>订正后再用另一题族检查同一知识点。</p></template>
   <p v-else-if="result.Correct>0">这一轮答对了有效判分题。可用不同题族检查能否再次答对。</p>
  </section>
 </form>
 <CitationReader :source="source" @close="source=null"/>
</template>
<style scoped>
.inline-assessment{margin:12px 0}
fieldset{margin:10px 0;padding:12px;border:1px solid #dce6f4;border-radius:8px}
label{display:flex;align-items:baseline;gap:8px;margin:8px 0}
label input{width:auto}
img{max-width:100%;max-height:360px}
</style>
