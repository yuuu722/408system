<script setup lang="ts">
import {computed} from 'vue'
import {useRoute} from 'vue-router'
import SectionTabs from '@/components/SectionTabs.vue'
import AssessmentView from './AssessmentView.vue'
import WeaknessView from './WeaknessView.vue'
import DashboardView from './DashboardView.vue'
const route=useRoute(),tab=computed(()=>['weakness','statistics'].includes(String(route.query.tab))?String(route.query.tab):'assessment')
</script>
<template><div><SectionTabs base="/exam408/assessment" :active="tab" label="测评学情分区" :tabs="[{key:'assessment',label:'前测、练习与报告'},{key:'weakness',label:'薄弱点复习'},{key:'statistics',label:'学情统计'}]"/><WeaknessView v-if="tab==='weakness'"/><DashboardView v-else-if="tab==='statistics'"/><AssessmentView v-else/></div></template>
