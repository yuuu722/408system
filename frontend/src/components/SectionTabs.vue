<script setup lang="ts">
import {useRoute} from 'vue-router'
defineProps<{base:string;active:string;tabs:{key:string;label:string}[];label:string}>()
const route=useRoute()
</script>
<template><nav class="section-tabs" :aria-label="label"><RouterLink v-for="tab in tabs" :key="tab.key" :to="{path:base,query:{...route.query,tab:tab.key}}" :class="{selected:active===tab.key}" :aria-current="active===tab.key?'page':undefined">{{tab.label}}</RouterLink></nav></template>
<style scoped>.section-tabs{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:24px;padding:6px;background:#eaf0f8;border-radius:14px;width:fit-content;max-width:100%}.section-tabs a{padding:11px 18px;border-radius:10px;text-decoration:none;color:#52657d;font-weight:600}.section-tabs .selected{background:white;color:#245ce5;box-shadow:0 2px 8px #0f172a0c}.section-tabs a:focus-visible{outline:2px solid #2563eb;outline-offset:2px}@media(max-width:650px){.section-tabs{width:100%}.section-tabs a{flex:1;text-align:center;padding:10px}}</style>
