<script setup lang="ts">
import {computed} from 'vue'
import type {PointInsight} from '@/api/coaching'
const props=defineProps<{points:PointInsight[];parent?:string;subject:string;depth?:number}>(),emit=defineEmits<{select:[id:string]}>()
const children=computed(()=>props.points.filter(p=>p.SubjectID===props.subject&&(p.ParentID===(props.parent??'')||(!props.parent&&p.ParentID&&!props.points.some(a=>a.ID===p.ParentID)))).sort((a,b)=>a.Code.localeCompare(b.Code)))
</script>
<template><ul class="mind-branch"><li v-for="p in children" :key="p.ID"><details :open="(depth??0)<1"><summary><button type="button" :class="{weak:p.State==='需要补强'||p.State==='待复查（有错题）',important:p.Importance>=3}" @click.prevent="emit('select',p.ID)">{{p.Name}} <small>{{p.State}} · {{p.AvailableQuestions}}题</small><span class="importance" :title="p.ImportanceBasis">{{'●'.repeat(Math.round(p.Importance))}}</span></button></summary><KnowledgeMindMap v-if="(depth??0)<8" :points="points" :parent="p.ID" :subject="subject" :depth="(depth??0)+1" @select="emit('select',$event)"/></details></li></ul></template>
<style scoped>.mind-branch{list-style:none;padding-left:18px;border-left:2px solid #dbeafe;margin:10px 0}.mind-branch>li{padding:6px 0}.mind-branch button{background:#f8fafc;border:1px solid #dbeafe;border-radius:12px;padding:10px;text-align:left;color:#26374c;cursor:pointer}.mind-branch small{display:block;color:#64748b}.mind-branch .weak{border-color:#f59e0b}.mind-branch .important{box-shadow:inset 3px 0 #2563eb}.importance{color:#2563eb;font-size:9px}</style>
