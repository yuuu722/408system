package repository

// Stable aliases preserve the meaning of pre-deduplication attempts even after
// a later manual edit. The underlying assessment/results are never rewritten.
const canonicalEventFamilySQL = `COALESCE(
 CASE WHEN metadata->>'family_id' LIKE 'public-question-v1:%' THEN metadata->>'family_id' END,
 (SELECT canonical_family_id FROM question_family_aliases WHERE legacy_family_id=COALESCE(NULLIF(metadata->>'family_id',''),metadata->>'question_id')),
 (SELECT NULLIF(canonical.payload->>'FamilyID','') FROM learning_assets canonical WHERE canonical.id::text=metadata->>'question_id'),
 NULLIF(metadata->>'family_id',''),metadata->>'question_id')`
const canonicalSnapshotFamilySQL = `COALESCE(
 CASE WHEN q->'Payload'->>'FamilyID' LIKE 'public-question-v1:%' THEN q->'Payload'->>'FamilyID' END,
 (SELECT canonical_family_id FROM question_family_aliases WHERE legacy_family_id=COALESCE(NULLIF(q->'Payload'->>'FamilyID',''),q->>'ID')),
 (SELECT NULLIF(old.payload->>'FamilyID','') FROM learning_assets old WHERE old.id::text=q->>'ID'),
 NULLIF(q->'Payload'->>'FamilyID',''),q->>'ID')`
