-- 004_seed_partners_vouchers.sql
-- Phase 3 — partner and voucher catalogue seed.
--
-- Seeded by migration rather than by the -seed-demo flag because the catalogue is
-- reference data, not demo accounts: an empty voucher list is a broken feature,
-- whereas a missing demo login is only an inconvenience. Every insert is
-- idempotent on a natural key so re-running is safe.
--
-- Point costs are set against real earning rates (002_seed_material_rates): PET
-- pays 25/kg, so a 420-point voucher is about 17 kg of bottles — a few weeks of
-- household recycling. Costs that round to something a user can reach in one or
-- two collections keep the reward legible; a 5,000-point voucher reads as
-- unreachable and stops motivating.

INSERT INTO partners (name, logo_url, active)
SELECT 'Naivas Supermarket', NULL, 1
WHERE NOT EXISTS (SELECT 1 FROM partners WHERE name = 'Naivas Supermarket');

INSERT INTO partners (name, logo_url, active)
SELECT 'Quickmart', NULL, 1
WHERE NOT EXISTS (SELECT 1 FROM partners WHERE name = 'Quickmart');

INSERT INTO partners (name, logo_url, active)
SELECT 'Java House', NULL, 1
WHERE NOT EXISTS (SELECT 1 FROM partners WHERE name = 'Java House');

-- Vouchers. partner_id is resolved by name rather than hardcoded to 1/2/3, so
-- this migration is correct even if the partners table already had rows.
--
-- The spread is deliberate: the cheapest voucher is reachable from a single
-- decent collection so a new user has something to aim at, and the range spans
-- both discount types (percentage and fixed) so the Phase 4 redemption screen has
-- both shapes to render.

INSERT INTO vouchers (partner_id, title, points_cost, discount_type, discount_value, expiry_days, stock_remaining, active)
SELECT p.id, 'KSh 100 off your shopping', 150, 'fixed', 100, 30, 200, 1
FROM partners p WHERE p.name = 'Naivas Supermarket'
  AND NOT EXISTS (SELECT 1 FROM vouchers WHERE title = 'KSh 100 off your shopping');

INSERT INTO vouchers (partner_id, title, points_cost, discount_type, discount_value, expiry_days, stock_remaining, active)
SELECT p.id, '10% off your basket', 420, 'percentage', 10, 30, 50, 1
FROM partners p WHERE p.name = 'Naivas Supermarket'
  AND NOT EXISTS (SELECT 1 FROM vouchers WHERE title = '10% off your basket');

INSERT INTO vouchers (partner_id, title, points_cost, discount_type, discount_value, expiry_days, stock_remaining, active)
SELECT p.id, 'KSh 250 off groceries', 380, 'fixed', 250, 45, 120, 1
FROM partners p WHERE p.name = 'Quickmart'
  AND NOT EXISTS (SELECT 1 FROM vouchers WHERE title = 'KSh 250 off groceries');

INSERT INTO vouchers (partner_id, title, points_cost, discount_type, discount_value, expiry_days, stock_remaining, active)
SELECT p.id, '15% off fresh produce', 600, 'percentage', 15, 30, NULL, 1
FROM partners p WHERE p.name = 'Quickmart'
  AND NOT EXISTS (SELECT 1 FROM vouchers WHERE title = '15% off fresh produce');

-- stock_remaining NULL means unlimited (docs/06 §2), exercised here so the
-- catalogue and the Phase 4 stock check both have a real case to handle.
INSERT INTO vouchers (partner_id, title, points_cost, discount_type, discount_value, expiry_days, stock_remaining, active)
SELECT p.id, 'Free tea or coffee', 200, 'fixed', 350, 60, NULL, 1
FROM partners p WHERE p.name = 'Java House'
  AND NOT EXISTS (SELECT 1 FROM vouchers WHERE title = 'Free tea or coffee');

INSERT INTO vouchers (partner_id, title, points_cost, discount_type, discount_value, expiry_days, stock_remaining, active)
SELECT p.id, 'KSh 500 off a meal for two', 900, 'fixed', 500, 45, 40, 1
FROM partners p WHERE p.name = 'Java House'
  AND NOT EXISTS (SELECT 1 FROM vouchers WHERE title = 'KSh 500 off a meal for two');

-- An inactive voucher, so "inactive is invisible" is covered by real data rather
-- than only by a test fixture: it must not appear in the catalogue or resolve by
-- id, and Phase 4 must refuse to redeem it.
INSERT INTO vouchers (partner_id, title, points_cost, discount_type, discount_value, expiry_days, stock_remaining, active)
SELECT p.id, 'Expired launch promo', 100, 'fixed', 50, 1, 0, 0
FROM partners p WHERE p.name = 'Java House'
  AND NOT EXISTS (SELECT 1 FROM vouchers WHERE title = 'Expired launch promo');

-- The catalogue is always read with its partner joined and filtered on active, so
-- the index matches that access pattern rather than partner_id alone.
CREATE INDEX IF NOT EXISTS idx_vouchers_active_cost
    ON vouchers(active, points_cost);
