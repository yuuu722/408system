import {api} from './client'
import type {Assessment} from './foundation'
import type {ChatResult} from './types'
export interface Memory {Key:string;PointID:string;Kind:'preference'|'goal'|'doubt';Content:string;Resolved:boolean;UpdatedAt?:string}
export interface FrequencySource {ID?:string;PointID:string;Organization:string;Title:string;URL:string;Excerpt:string;StartYear:number;EndYear:number;Exams:number;Appearances:number}
export interface PointInsight {ID:string;Code:string;Name:string;SubjectID:string;SubjectName:string;ParentID:string;Description:string;State:string;Tested:number;Correct:number;AvailableQuestions:number;SubtreeQuestions?:number;ExamYears:number;CorpusYears:number;Doubts:number;Accuracy:number|null;Importance:number;ImportanceBasis:string;Priority:number;Sources:FrequencySource[]}
export interface Overview {Points:PointInsight[];Memories:Memory[];NeedsPretest:boolean;Notice:string}
export interface CoachSession {KnowledgeBaseID:string;Feedback?:ChatResult;ID:string;PointID:string;Teacher:string;Assessment:Assessment;Lesson:ChatResult;CreatedAt:string}
export const teachers=[{id:'encouraging',name:'轻松鼓励老师',hint:'耐心讲清，循序练习，不因答错否定你。'},{id:'socratic',name:'循序引导老师',hint:'重视前提、步骤和检查问题。'},{id:'direct',name:'重点直讲老师',hint:'先抓关键概念、适用条件和易错点。'}]
export const coaching={
 overview:async():Promise<Overview>=>(await api.get('/exam408/learning/overview')).data,
 memory:(m:Memory)=>api.put(`/exam408/learning/memories/${encodeURIComponent(m.Key)}`,m),
 forget:(key:string)=>api.delete(`/exam408/learning/memories/${encodeURIComponent(key)}`),
 sessions:async():Promise<CoachSession[]>=>(await api.get('/exam408/coaching')).data??[],
 start:async(body:{PointID:string;KnowledgeBaseID:string;Teacher:string}):Promise<CoachSession>=>(await api.post('/exam408/coaching',body,{timeout:240000})).data,
 explain:async(id:string):Promise<ChatResult>=>(await api.post(`/exam408/coaching/${id}/explain`,{},{timeout:240000})).data,
 source:(v:FrequencySource)=>api.post('/admin/frequency-sources',v)
}

export interface PointStudy {PreferredKnowledgeBaseID?:string;PointNames?:Record<string,string>;Point:import('./types').KnowledgePoint;Ancestors:import('./types').KnowledgePoint[];Children:import('./types').KnowledgePoint[];Knowledge:import('./types').Citation[];Example:import('./foundation').Asset|null;Exercises:import('./foundation').Asset[];Mistakes:{AssessmentID:string;Question:import('./foundation').Asset;Result:{Selected:number;Answer:number;Explanation:string;AnswerOrigin?:string;Confirmation?:string};SubmittedAt:string}[];DirectQuestions:number;SubtreeQuestions:number;Missing:string[]}
export const pointStudyApi={load:async(id:string):Promise<PointStudy>=>(await api.get(`/exam408/knowledge-points/${encodeURIComponent(id)}/study`)).data}
