import { api } from './client'
import { errorMessage } from './errors'
import type { ChatResult,Citation } from './types'

export interface Asset { IsPublic?:boolean; ID:string;Kind:'question'|'video'|'material';PointID:string;Title:string;Status:string;Revision:number;Payload:{EstimatedMinutes?:number;DifficultyOrigin?:string;QuestionOrigin?:string;FamilyID?:string;ParentAssetID?:string;Evidence?:Citation[];AnswerOrigin?:string;Issue?:string;ExtractionKey?:string;Images?:string[];Stem:string;Options:string[];Answer:number;Explanation:string;Source:string;URL:string;DocumentID:string;Page:number;StartSeconds:number;EndSeconds:number;Difficulty:number} }
export interface Profile { GoalScore:number;DailyMinutes:number;ExamDate:string;Stage:string;Preference:string }
export interface Result { Total:number;Correct:number;Counted?:number;Items:{AnswerOrigin?:string;Confirmation?:string;Counted?:boolean;ID:string;PointID:string;Selected:number;Answer:number;Correct:boolean;Explanation:string}[] }
export interface Assessment { ID:string;Kind:string;Questions:Asset[];Result:Result|null;CreatedAt:string }
export interface RadarSubject { ID:string;Name:string;Tested:number;Correct:number;CoveredPoints:number;TotalPoints:number;Accuracy:number|null }
export interface StudyTask { PlanDate?:string;EstimatedMinutes?:number;DurationSource?:string;ID:string;AssetID:string;Title:string;Kind:string;PointID:string;Reason:string;URL:string;DocumentID:string;Page:number;StartSeconds:number;EndSeconds:number;Completed:boolean }
export interface DailyPlan {ID:string;Date:string;Strategy:string;BudgetMinutes:number;AllocatedMinutes:number}
export interface Conversation { ID:string;KnowledgeBaseID:string;Title:string;UpdatedAt:string }
export interface SavedMessage { ID:string;Role:'user'|'assistant';Content:string;Citations:ChatResult['Sources'] }
export const foundation = {
 async assets():Promise<Asset[]> {return (await api.get('/exam408/assets')).data ?? []},
 async saveAsset(a:Asset):Promise<Asset>{return (await (a.ID?api.put(`/exam408/assets/${a.ID}`,a):api.post('/exam408/assets',a))).data},
 removeAsset:(id:string)=>api.delete(`/exam408/assets/${id}`),
 async profile():Promise<Profile|null>{return (await api.get('/exam408/profile')).data},
 saveProfile:(p:Profile)=>api.put('/exam408/profile',p),
 async assessments():Promise<Assessment[]>{return (await api.get('/exam408/assessments')).data??[]},
 async start(kind:string,point='',asset=''):Promise<Assessment>{return (await api.post('/exam408/assessments',{Kind:kind,PointID:point,AssetID:asset})).data},
 async submit(id:string,answers:Record<string,number>):Promise<Result>{return (await api.post(`/exam408/assessments/${id}/submit`,{Answers:answers})).data},
 async confirm(aid:string,qid:string,decision:string):Promise<Result>{return (await api.post(`/exam408/assessments/${aid}/questions/${qid}/confirm`,{Decision:decision})).data},
 async infer(aid:string,qid:string):Promise<Result>{return (await api.post(`/exam408/assessments/${aid}/questions/${qid}/infer`)).data},
 async structure(id:string,revision=''):Promise<ParsedDocument>{return (await api.get(`/documents/${id}/structure`,{params:{revision}})).data},
 async radar():Promise<RadarSubject[]>{return (await api.get('/exam408/radar')).data??[]},
 async tasks():Promise<StudyTask[]>{return (await api.get('/exam408/tasks')).data??[]},
 async dailyPlan():Promise<DailyPlan>{return (await api.get('/exam408/tasks/plan')).data},
 async generatePractice(input:{RequestID:string;KnowledgeBaseID:string;PointID:string;ParentAssetID:string;Count:number;AllowUngrounded:boolean}):Promise<Asset[]>{return (await api.post('/exam408/practice/generate',input)).data??[]},
 generate:()=>api.post('/exam408/tasks/generate'),
 complete:(id:string)=>api.post(`/exam408/tasks/${id}/complete`),
 async conversations():Promise<Conversation[]>{return (await api.get('/conversations')).data??[]},
 async messages(id:string):Promise<SavedMessage[]>{return (await api.get(`/conversations/${id}/messages`)).data??[]},
 rename:(id:string,title:string)=>api.patch(`/conversations/${id}`,{Title:title}),
 removeConversation:(id:string)=>api.delete(`/conversations/${id}`),
}
export async function openDocument(id:string,page=0,revision=''){
 const target=window.open('about:blank','_blank');if(target)target.opener=null
 try{const r=await api.get(`/documents/${encodeURIComponent(id)}/file`,{responseType:'blob',params:{revision}});const url=URL.createObjectURL(r.data);if(target)target.location.href=url+(page>0?`#page=${page}`:'');else{URL.revokeObjectURL(url);throw new Error('请允许弹出窗口后重试（Popup blocked）')};setTimeout(()=>URL.revokeObjectURL(url),300000)}catch(e){target?.close();throw e}
}
export async function streamChat(body:{KnowledgeBaseID:string;ConversationID:string;Question:string;IncludePersonal?:boolean},signal:AbortSignal,onEvent:(event:string,data:unknown)=>void){
 const response=await fetch(`${api.defaults.baseURL}/exam408/assistant/stream`,{method:'POST',headers:{'Content-Type':'application/json',Authorization:`Bearer ${localStorage.getItem('exam408_token')??''}`},body:JSON.stringify(body),signal})
 if(!response.ok){const data=await response.json().catch(()=>({}));if(response.status===401){localStorage.removeItem('exam408_token');location.href='/login'};throw new Error(errorMessage(data.error,response.status))}
 if(!response.body)throw new Error('无法读取回答（Stream unavailable）')
 const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='',done=false
 try{for(;;){const packet=await reader.read();if(packet.done)break;buffer+=decoder.decode(packet.value,{stream:true});let boundary:number
  while((boundary=buffer.indexOf('\n\n'))>=0){const block=buffer.slice(0,boundary);buffer=buffer.slice(boundary+2);const event=block.match(/^event: (.+)$/m)?.[1];const raw=block.match(/^data: (.+)$/m)?.[1];if(!event||!raw)continue;const data=JSON.parse(raw);if(event==='error')throw new Error(data.error);if(event==='done')done=true;onEvent(event,data)}
 };if(!done)throw new Error('连接中断，回答未完成，请重试（Answer incomplete）')}finally{reader.releaseLock()}
}

export interface StructuredPage {Number:number;Segment?:number;Width?:number;Height?:number;Rotation?:number;CoordinateSpace?:string;Text:string;Warnings:string[];Blocks:{ID:string;Kind:string;Text:string;Start?:number;End?:number;Image?:string;Box?:number[]}[]}
export interface ParsedDocument {State?:string;RevisionID?:string;Engine:string;Pages:StructuredPage[]}
