<script setup lang="ts">
import {computed} from 'vue'
import {useRoute} from 'vue-router'
import SectionTabs from '@/components/SectionTabs.vue'
import TasksView from './TasksView.vue'
import LearningStudioView from './LearningStudioView.vue'
const route=useRoute(),tab=computed(()=>route.query.tab==='teacher'?'teacher':'today')
</script>
<template><div><SectionTabs base="/exam408/tasks" :active="tab" label="学习任务分区" :tabs="[{key:'today',label:'今日任务'},{key:'teacher',label:'老师与学习方案'}]"/><LearningStudioView v-if="tab==='teacher'"/><TasksView v-else/></div></template>
