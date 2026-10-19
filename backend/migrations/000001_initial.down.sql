BEGIN;
DROP TABLE IF EXISTS exam408_recommendations, exam408_user_mastery, exam408_learning_events,
 messages, conversations, exam408_entity_mappings, knowledge_relations, knowledge_entities,
 exam408_resource_mappings, exam408_knowledge_point_relations, exam408_knowledge_points,
 exam408_subjects, document_chunks, documents, knowledge_bases, users CASCADE;
COMMIT;

