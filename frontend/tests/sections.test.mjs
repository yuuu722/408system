import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
const read=p=>readFileSync(new URL('../src/'+p,import.meta.url),'utf8')
test('legacy teacher links retain query and legacy assessment tabs route to independent pages',()=>{
 const r=read('router/index.ts')
 assert.match(r,/path:'exam408\/learning',redirect:to=>\(\{path:'\/exam408\/tasks',query:\{\.\.\.to.query,tab:'teacher'/)
 assert.match(r,/const \{tab,\.\.\.query\}=to.query/)
 assert.match(r,/tab==='weakness'\?'\/exam408\/weakness':'\/exam408\/statistics'/)
})
test('assessment, weakness and statistics are independent top navigation destinations',()=>{
 const layout=read('components/AppLayout.vue').split('const studentItems = [')[1].split('const items=')[0]
 assert.equal((layout.match(/path:/g)||[]).length,9)
 for(const path of ['assessment','weakness','statistics'])assert.ok(layout.includes(`path:'/exam408/${path}'`))
 assert.ok(!read('views/AssessmentHubView.vue').includes('<SectionTabs'))
})
test('goals, assessment statistics and practice generation have single editing homes',()=>{assert.ok(read('views/SettingsView.vue').includes('v-model.number="profile.GoalScore"'));assert.ok(!read('views/AssessmentView.vue').includes('v-model.number="profile.GoalScore"'));assert.ok(!read('views/SettingsView.vue').includes('<h2>学情检测</h2>'));assert.ok(!read('views/TasksView.vue').includes('<GeneratePractice'));assert.ok(read('views/AssetsView.vue').includes('<GeneratePractice'))})
test('teacher page selects a lesson while knowledge hub owns the full map',()=>{assert.ok(!read('views/LearningStudioView.vue').includes('<KnowledgeMindMap'));assert.ok(read('views/KnowledgeMapView.vue').includes('<KnowledgeMindMap'));assert.ok(read('views/KnowledgeHubView.vue').includes('知识关系图谱'))})
