export interface KnowledgeBase{ID:string;UserID:string;Name:string;Description:string;IsPublic:boolean;CreatedAt:string;UpdatedAt:string}
export interface GraphNode{ID:string;Name:string;EntityType:string;Description:string;KnowledgePointID:string;EvidenceChunkID:string;EvidenceExcerpt:string;EvidenceDocumentID?:string;EvidenceDocumentTitle?:string;Source:string;Confidence:number;CreatedAt:string}
export interface GraphEdge{ID:string;FromEntityID:string;ToEntityID:string;RelationType:string;EvidenceChunkID:string;EvidenceExcerpt:string;EvidenceDocumentID?:string;EvidenceDocumentTitle?:string;Source:string;Confidence:number;CreatedAt:string}
export interface KnowledgeGraph{Warning?:string;Storage?:string;KnowledgeBaseID:string;IsPublic:boolean;Nodes:GraphNode[];Edges:GraphEdge[]}
export interface Document{ActiveRevisionID?:string;Progress?:number;ProcessingStage?:string;ProcessedChunks?:number;TotalChunks?:number;ID:string;KnowledgeBaseID:string;OriginalFilename:string;MimeType:string;Size:number;ResourceType:string;Status:'uploaded'|'processing'|'ready'|'failed';ErrorMessage:string;CreatedAt:string}
export interface Subject{ID:string;Code:string;Name:string;SortOrder:number}
export interface KnowledgePoint{ID:string;SubjectID:string;ParentID:string;Code:string;Name:string;Description:string;SubjectName:string;Keywords:string[]}
export interface Mastery{KnowledgePointID:string;KnowledgePointName:string;SubjectName:string;MasteryScore:number;WeaknessScore:number;Confidence:number;InteractionCount:number;UpdatedAt:string}
export interface Recommendation{ID:string;KnowledgePointID:string;KnowledgePointName:string;SubjectName:string;Score:number;Reason:string;RecommendedAction:string}
export interface SourceRegion{Page:number;Segment:number;BlockID:string;Start:number;End:number;Box?:number[]}
export interface Citation{Page?:number;RevisionID?:string;Regions?:SourceRegion[];LocationQuality?:string;DocumentID:string;DocumentTitle:string;ChunkID:string;Location:string;Excerpt:string;Score:number}
export interface ChatResult{RelatedQuestions?:import('./foundation').Asset[];Reasoning?:RetrievalTrace;Answer:string;ConversationID:string;Sources:Citation[];KnowledgePoints:KnowledgePoint[]}
export interface Dashboard{Subjects:{ID:string;Code:string;Name:string;Progress:number;KnowledgePointCount:number;TouchedCount:number}[];Weaknesses:Mastery[];Recommendations:Recommendation[];RecentDocuments:Document[];KnowledgeBaseCount:number;ReadyDocumentCount:number}
export interface LearningEvent{ID:string;EventType:string;CreatedAt:string;Metadata:Record<string,unknown>}

export interface RetrievalTrace {Mode:string;Subquestions:string[];Paths:{Nodes:GraphNode[];Edges:GraphEdge[]}[];Notice:string}
