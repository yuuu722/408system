<script setup lang="ts">
import {computed} from 'vue'
import {useRoute} from 'vue-router'
import SectionTabs from '@/components/SectionTabs.vue'
import KnowledgeMapView from './KnowledgeMapView.vue'
import GraphView from './GraphView.vue'
const route=useRoute(),tab=computed(()=>route.query.tab==='graph'?'graph':'map')
</script>
<template><div><SectionTabs base="/exam408/knowledge" :active="tab" label="知识体系分区" :tabs="[{key:'map',label:'知识思维导图'},{key:'graph',label:'知识关系图谱'}]"/><GraphView v-if="tab==='graph'"/><KnowledgeMapView v-else/></div></template>
