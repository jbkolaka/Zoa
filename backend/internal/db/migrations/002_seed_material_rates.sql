-- 002_seed_material_rates.sql
-- Seeds the material taxonomy from docs/02_Technical_Requirements.md §2.6.
--
-- The Backend Schema doc gives 'plastic' / 'paper' / 'glass' / 'metal' only as
-- examples; TRD §2.6 requires the specific taxonomy (resin type for plastics,
-- sub-type for organics), so these keys are the real value domain for
-- submissions.material_type.
--
-- INSERT OR IGNORE keeps this idempotent AND non-destructive: once an admin
-- edits a rate (FR-9), re-running migrations will not reset it.
--
-- points_per_kg is relative-value calibrated, not authoritative pricing:
-- aluminium and PET lead per-kg; organics are low per-kg but high per-volume.

INSERT OR IGNORE INTO material_rates (material_type, points_per_kg) VALUES
    -- Plastics
    ('pet',            25),  -- PET bottles — highest-value recyclable stream
    ('hdpe',           22),  -- HDPE containers / jerricans
    ('pp',             12),  -- PP caps / tubs
    ('ldpe',           10),  -- LDPE bags / film — low value, high volume
    ('ps',              6),  -- PS foam / rigid
    ('other_plastic',   5),  -- Other / mixed plastic
    -- Paper & cardboard
    ('cardboard',       8),
    ('mixed_paper',     6),
    -- Glass
    ('glass_clear',     5),
    ('glass_colored',   3),
    -- Metal
    ('aluminum',       40),  -- Cans — light to collect, high resale value
    ('steel_tin',      15),
    -- Organic waste
    ('food_waste',      2),  -- Hotel kitchens & households (see FR-4a / FR-24)
    ('garden_waste',    1);
