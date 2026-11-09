<script setup lang="ts">
import {computed} from 'vue'
import {useRoute} from 'vue-router'
import SectionTabs from '@/components/SectionTabs.vue'
import AssessmentView from './AssessmentView.vue'
import DashboardView from './DashboardView.vue'
const route=useRoute(),tab=computed(()=>route.query.tab==='statistics'?'statistics':'assessment')
</script>
<template><div><SectionTabs base="/exam408/assessment" :active="tab" label="测评与学情分区" :tabs="[{key:'assessment',label:'前测、练习与报告'},{key:'statistics',label:'学情统计'}]"/><DashboardView v-if="tab==='statistics'"/><AssessmentView v-else/></div></template>
