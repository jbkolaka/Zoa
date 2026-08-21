-- 001_init.sql
-- Zoa — initial schema.
-- Tables, columns, types and indexes follow docs/06_Backend_Schema.md exactly.
-- Boolean columns are INTEGER (0/1); SQLite has no native boolean type.

CREATE TABLE IF NOT EXISTS users (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    phone_number   TEXT UNIQUE NOT NULL,
    name           TEXT NOT NULL,
    password_hash  TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'user'
                   CHECK (role IN ('user', 'collector', 'partner_staff', 'admin')),
    points_balance INTEGER NOT NULL DEFAULT 0,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS submissions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL REFERENCES users(id),
    material_type    TEXT NOT NULL,
    estimated_qty_kg REAL,
    verified_qty_kg  REAL,
    location         TEXT,
    status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'collected', 'verified', 'rejected')),
    collector_id     INTEGER REFERENCES users(id),
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    verified_at      DATETIME
);

CREATE TABLE IF NOT EXISTS material_rates (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    material_type  TEXT UNIQUE NOT NULL,
    points_per_kg  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS partners (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT NOT NULL,
    logo_url TEXT,
    active   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS vouchers (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    partner_id      INTEGER NOT NULL REFERENCES partners(id),
    title           TEXT NOT NULL,
    points_cost     INTEGER NOT NULL,
    discount_type   TEXT NOT NULL CHECK (discount_type IN ('percentage', 'fixed')),
    discount_value  REAL NOT NULL,
    expiry_days     INTEGER NOT NULL,
    stock_remaining INTEGER,
    active          INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS redemptions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    voucher_id      INTEGER NOT NULL REFERENCES vouchers(id),
    redemption_code TEXT UNIQUE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'issued'
                    CHECK (status IN ('issued', 'used', 'expired')),
    issued_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    used_at         DATETIME,
    verified_by     INTEGER REFERENCES users(id)
);

-- points_ledger is declared after redemptions so its FK target exists.
CREATE TABLE IF NOT EXISTS points_ledger (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id),
    submission_id INTEGER REFERENCES submissions(id),
    redemption_id INTEGER REFERENCES redemptions(id),
    points_delta  INTEGER NOT NULL,
    reason        TEXT NOT NULL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes exactly as specified in docs/06_Backend_Schema.md §3.
CREATE UNIQUE INDEX IF NOT EXISTS idx_redemptions_code   ON redemptions(redemption_code);
CREATE        INDEX IF NOT EXISTS idx_submissions_user   ON submissions(user_id);
CREATE        INDEX IF NOT EXISTS idx_submissions_status ON submissions(status);
CREATE        INDEX IF NOT EXISTS idx_points_ledger_user ON points_ledger(user_id);
CREATE        INDEX IF NOT EXISTS idx_vouchers_partner   ON vouchers(partner_id);
