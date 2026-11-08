import test from 'node:test'
import assert from 'node:assert/strict'
import {loadStudyPlan,learningReason} from '../src/lib/studyPlan.ts'
const plan={ID:'p',Date:'2026-11-08',BudgetMinutes:60,AllocatedMinutes:5,Strategy:'test'}
test('completed pretest repairs missing daily plan and excludes historical tasks',async()=>{
 let generated=0,reads=0
 const api={dailyPlan:async()=>++reads===1?{...plan,ID:''}:plan,generate:async()=>{generated++},tasks:async()=>[{ID:'today',PlanDate:plan.Date},{ID:'old',PlanDate:'2026-11-07'}]}
 const result=await loadStudyPlan(api,true)
 assert.equal(generated,1);assert.deepEqual(result.tasks.map(t=>t.ID),['today'])
})
test('saved empty plan does not repeatedly regenerate; generation failures remain visible',async()=>{
 let generated=0
 const api={dailyPlan:async()=>plan,tasks:async()=>[],generate:async()=>{generated++;throw Error('offline')}}
 assert.deepEqual((await loadStudyPlan(api,true)).tasks,[]);assert.equal(generated,0)
 await assert.rejects(()=>loadStudyPlan(api,true,true),/offline/)
})
test('unassessed learner is not silently assigned a diagnostic plan',async()=>{
 let generated=0
 await loadStudyPlan({dailyPlan:async()=>({...plan,ID:''}),tasks:async()=>[],generate:async()=>generated++},false)
 assert.equal(generated,0)
})
test('recommendation reason separates sparse errors, importance and provisional doubts',()=>{
 const reason=learningReason({Tested:2,Correct:0,Importance:4.2,Doubts:1})
 assert.match(reason,/2 个有效题组答对 0 个/);assert.match(reason,/4.2\/5/);assert.match(reason,/不等于答错/)
 assert.match(learningReason({Tested:0,Correct:0,Importance:1,Doubts:0}),/尚未测评/)
})
