import type {DailyPlan,StudyTask} from '../api/foundation'
export interface PlanAPI {dailyPlan:()=>Promise<DailyPlan>;tasks:()=>Promise<StudyTask[]>;generate:()=>Promise<unknown>}
// Repair a missing plan for an already assessed learner; an empty saved plan is
// different from a failed request and must not trigger a generation loop.
export async function loadStudyPlan(api:PlanAPI,generateMissing=false,regenerate=false){
 let plan=await api.dailyPlan()
 if(regenerate||(generateMissing&&(!plan.ID||['408-learning-v2','408-rules-v1'].includes(plan.Strategy)))){await api.generate();plan=await api.dailyPlan()}
 const tasks=(await api.tasks()).filter(t=>t.PlanDate===plan.Date)
 return {plan,tasks}
}
export function learningReason(p:{Tested:number;Correct:number;Importance:number;Doubts:number}){
 const result=p.Tested?`最近 ${p.Tested} 个有效题组答对 ${p.Correct} 个${p.Correct<p.Tested?'，需要复查错题':'，继续巩固'}`:'尚未测评，建议补充诊断'
 return `${result}；参考重要性 ${p.Importance.toFixed(1)}/5${p.Doubts?`；另有 ${p.Doubts} 条待确认疑问（不等于答错）`:''}。`
}
