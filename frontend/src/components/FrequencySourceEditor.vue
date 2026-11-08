<script setup lang="ts">
import {ref} from 'vue'
import {coaching,type FrequencySource} from '@/api/coaching'
import {messageOf} from '@/api/client'
const props=defineProps<{pointId:string}>(),emit=defineEmits<{saved:[]}>(),busy=ref(false),error=ref('')
const value=ref<FrequencySource>({PointID:'',Organization:'',Title:'',URL:'',Excerpt:'',StartYear:2016,EndYear:2025,Exams:10,Appearances:0})
async function save(){busy.value=true;error.value='';try{await coaching.source({...value.value,PointID:props.pointId});emit('saved')}catch(e){error.value=messageOf(e)}finally{busy.value=false}}
</script>
<template><details><summary>管理员：录入考频来源</summary><p>只录入核对过的公开报告；保留年份、分子分母和原文摘录。</p><p v-if="error" class="notice error">{{error}}</p><form class="section" @submit.prevent="save"><label class="field">机构<input v-model="value.Organization" required maxlength="100"/></label><label class="field">报告标题<input v-model="value.Title" required maxlength="300"/></label><label class="field">HTTPS原文链接<input v-model="value.URL" type="url" required/></label><label class="field">统计摘录<textarea v-model="value.Excerpt" required minlength="10" maxlength="3000"/></label><div class="grid two-col"><label>起始年<input v-model.number="value.StartYear" type="number" min="2009" required/></label><label>结束年<input v-model.number="value.EndYear" type="number" required/></label><label>统计年度数<input v-model.number="value.Exams" type="number" min="1" required/></label><label>涉及年度数<input v-model.number="value.Appearances" type="number" min="0" required/></label></div><button class="secondary" :disabled="busy">保存来源</button></form></details></template>
