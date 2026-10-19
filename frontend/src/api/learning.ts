import { api } from './client'
import { asList, normalizeDashboard } from './normalize'
import type { ChatResult, Dashboard, Document, KnowledgeBase, KnowledgeGraph, KnowledgePoint, LearningEvent, Mastery, Recommendation, Subject } from './types'

async function list<T>(path: string): Promise<T[]> { return asList((await api.get<T[] | null>(path)).data) }
const id = encodeURIComponent
export const learningApi = {
  async assistantStatus() { return (await api.get<{mode: 'unavailable' | 'demo' | 'model'; enabled: boolean; message: string}>('/exam408/assistant/status')).data },
  async dashboard() { return normalizeDashboard((await api.get<Dashboard>('/exam408/dashboard')).data) },
  bases: () => list<KnowledgeBase>('/knowledge-bases'),
  async graph(base: string) { return (await api.get<KnowledgeGraph>(`/knowledge-bases/${id(base)}/graph`)).data },
  async createBase(name: string, description: string) { return (await api.post<KnowledgeBase>('/knowledge-bases', { name, description })).data },
  documents: (base: string) => list<Document>(`/knowledge-bases/${id(base)}/documents`),
  async upload(base: string, file: File, resourceType: string) {
    const body = new FormData(); body.append('file', file); body.append('resource_type', resourceType)
    return api.post(`/knowledge-bases/${id(base)}/documents`, body)
  },
  removeDocument: (document: string) => api.delete(`/documents/${id(document)}`),
  reprocess: (document: string) => api.post(`/documents/${id(document)}/reprocess`),
  mapPoint: (document: string, point: string) => api.post(`/documents/${id(document)}/knowledge-points/${id(point)}`),
  subjects: () => list<Subject>('/exam408/subjects'),
  points: () => list<KnowledgePoint>('/exam408/knowledge-points'),
  async point(point: string) {
    const { data } = await api.get<{ knowledge_point: KnowledgePoint; resources: Document[] | null; mastery: Mastery | null; recent_events: LearningEvent[] | null }>(`/exam408/knowledge-points/${id(point)}`)
    return { ...data, resources: asList(data.resources), recent_events: asList(data.recent_events) }
  },
  viewPoint: (point: KnowledgePoint) => api.post('/exam408/learning-events', { KnowledgePointID: point.ID, SubjectID: point.SubjectID, EventType: 'knowledge_point_view', Metadata: { source: 'knowledge_page' } }),
  weaknesses: () => list<Mastery>('/exam408/weaknesses'),
  recommendations: () => list<Recommendation>('/exam408/recommendations'),
  async generateRecommendations() { return asList((await api.post<Recommendation[] | null>('/exam408/recommendations/generate')).data) },
  async chat(base: string, conversation: string, question: string) {
    const { data } = await api.post<ChatResult>('/exam408/assistant/chat', { KnowledgeBaseID: base, ConversationID: conversation, Question: question })
    return { ...data, Sources: asList(data.Sources), KnowledgePoints: asList(data.KnowledgePoints) }
  },
}
