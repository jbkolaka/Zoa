-- 003_classification.sql
-- Phase 2.5 — AI material classification.
--
-- Both the AI's guess and the collector's verified value are kept (FR-22): the
-- predicted columns are never overwritten at verification, so predicted-vs-
-- verified accuracy stays measurable over time instead of being destroyed by the
-- correction that proves it wrong. `material_type` remains the operative value.
--
-- Added by ALTER rather than by editing 001: existing databases are migrated in
-- place, so a demo DB does not have to be rebuilt.

-- The taxonomy key the classifier predicted, e.g. 'pet'. Not constrained to the
-- taxonomy by CHECK: the set lives in Go (models.MaterialTaxonomy) and is
-- validated there, and a stale CHECK would silently reject a taxonomy addition.
ALTER TABLE submissions ADD COLUMN predicted_category TEXT;

-- Model confidence in [0,1]. NULL when no photo was supplied or the classifier
-- degraded — distinct from 0.0, which would mean "predicted, with no confidence".
ALTER TABLE submissions ADD COLUMN predicted_confidence REAL;

-- Waste origin (FR-4a / FR-24). Required for the organic group, NULL elsewhere;
-- hotel kitchen volumes differ enough from household ones to affect routing.
-- CHECK passes on NULL, so rows predating this migration remain valid.
ALTER TABLE submissions ADD COLUMN source_type TEXT
    CHECK (source_type IS NULL OR source_type IN ('residential', 'hotel'));

-- The collector queue and the accuracy report both filter on "was this
-- predicted?", which is a small minority of rows early on.
CREATE INDEX IF NOT EXISTS idx_submissions_predicted
    ON submissions(predicted_category)
    WHERE predicted_category IS NOT NULL;
