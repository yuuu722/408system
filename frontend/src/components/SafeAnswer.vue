<script setup lang="ts">
import { computed } from 'vue'
import { renderAnswer } from '@/lib/answer'
import 'katex/dist/katex.min.css'
const props=defineProps<{text:string;citations?:number}>()
const emit=defineEmits<{cite:[index:number]}>()
const html=computed(()=>renderAnswer(props.text,props.citations??0))
function click(event:MouseEvent){const button=(event.target as Element).closest<HTMLButtonElement>('button[data-citation]');if(button&&(event.currentTarget as Element).contains(button))emit('cite',Number(button.dataset.citation)-1)}
</script>
<template><div class="answer-markdown" @click="click" v-html="html"></div></template>
<style>
.answer-markdown{white-space:normal;overflow-wrap:anywhere;line-height:1.7}.answer-markdown>:first-child{margin-top:0}.answer-markdown>:last-child{margin-bottom:0}.answer-markdown pre{white-space:pre;overflow:auto;background:#f1f5f9;padding:12px;border-radius:8px;color:#0f172a}.answer-markdown table{border-collapse:collapse;display:block;overflow:auto}.answer-markdown td,.answer-markdown th{border:1px solid #cbd5e1;padding:6px 10px}.answer-markdown .katex-display{overflow:auto;padding:8px 0}.citation-number{padding:0 5px;color:#1d4ed8;background:#dbeafe;border:0;border-radius:4px;cursor:pointer}.answer-markdown a{color:#1d4ed8;text-decoration:underline}
</style>
