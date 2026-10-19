ALTER TABLE documents ADD COLUMN IF NOT EXISTS progress integer NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100);
ALTER TABLE documents ADD COLUMN IF NOT EXISTS processing_stage text NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN IF NOT EXISTS processed_chunks integer NOT NULL DEFAULT 0;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS total_chunks integer NOT NULL DEFAULT 0;
UPDATE documents SET progress=100,processing_stage='completed' WHERE status='ready';
