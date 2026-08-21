package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// openTestDB gives each test its own database file, so tests cannot interfere
// with one another or with the developer's app.db.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestMigrateCreatesSchemaFromDoc asserts the migration produces exactly the
// tables and indexes named in docs/06_Backend_Schema.md. If a column or index
// is renamed, this fails — which is the point: the schema doc is the contract.
func TestMigrateCreatesSchemaFromDoc(t *testing.T) {
	conn := openTestDB(t)

	if _, err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// §2 Table Definitions.
	wantTables := map[string][]string{
		"users": {
			"id", "phone_number", "name", "password_hash", "role",
			"points_balance", "created_at",
		},
		"submissions": {
			"id", "user_id", "material_type", "estimated_qty_kg",
			"verified_qty_kg", "location", "status", "collector_id",
			"created_at", "verified_at",
			// Added by 003_classification.sql for Phase 2.5 (docs/06 §2.1).
			"predicted_category", "predicted_confidence", "source_type",
		},
		"material_rates": {"id", "material_type", "points_per_kg"},
		"points_ledger": {
			"id", "user_id", "submission_id", "redemption_id", "points_delta",
			"reason", "created_at",
		},
		"partners": {"id", "name", "logo_url", "active"},
		"vouchers": {
			"id", "partner_id", "title", "points_cost", "discount_type",
			"discount_value", "expiry_days", "stock_remaining", "active",
		},
		"redemptions": {
			"id", "user_id", "voucher_id", "redemption_code", "status",
			"issued_at", "used_at", "verified_by",
		},
	}

	for table, wantCols := range wantTables {
		gotCols, err := columnsOf(conn, table)
		if err != nil {
			t.Errorf("table %q: %v", table, err)
			continue
		}
		for _, col := range wantCols {
			if !gotCols[col] {
				t.Errorf("table %q is missing column %q", table, col)
			}
		}
		if len(gotCols) != len(wantCols) {
			t.Errorf("table %q has %d columns, doc specifies %d",
				table, len(gotCols), len(wantCols))
		}
	}

	// §3 Key Constraints & Indexes.
	wantIndexes := []string{
		"idx_redemptions_code",
		"idx_submissions_user",
		"idx_submissions_status",
		"idx_points_ledger_user",
		"idx_vouchers_partner",
	}
	for _, index := range wantIndexes {
		var name string
		err := conn.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
			index,
		).Scan(&name)
		if err != nil {
			t.Errorf("missing index %q: %v", index, err)
		}
	}
}

// TestSeedCoversFullTaxonomy checks every TRD §2.6 material has a rate, since a
// submission of an unrated material would credit zero points.
func TestSeedCoversFullTaxonomy(t *testing.T) {
	conn := openTestDB(t)
	if _, err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	wantKeys := []string{
		"pet", "hdpe", "ldpe", "pp", "ps", "other_plastic",
		"cardboard", "mixed_paper",
		"glass_clear", "glass_colored",
		"aluminum", "steel_tin",
		"food_waste", "garden_waste",
	}

	for _, key := range wantKeys {
		var points int
		err := conn.QueryRow(
			`SELECT points_per_kg FROM material_rates WHERE material_type=?`, key,
		).Scan(&points)
		if err != nil {
			t.Errorf("no rate seeded for %q: %v", key, err)
			continue
		}
		if points <= 0 {
			t.Errorf("rate for %q is %d, want > 0", key, points)
		}
	}

	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM material_rates`).Scan(&count); err != nil {
		t.Fatalf("count rates: %v", err)
	}
	if count != len(wantKeys) {
		t.Errorf("material_rates has %d rows, want %d", count, len(wantKeys))
	}
}

// TestMigrateIsIdempotent covers the boot path: migrations run on every start,
// so a second run must be a no-op rather than an error or a duplicate insert.
func TestMigrateIsIdempotent(t *testing.T) {
	conn := openTestDB(t)

	first, err := Migrate(conn)
	if err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("first Migrate applied nothing")
	}

	second, err := Migrate(conn)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second Migrate re-applied %v, want none", second)
	}
}

// TestMigrateDoesNotResetEditedRates guards FR-9: rates are admin-configurable,
// so a redeploy must not silently revert an operator's change.
func TestMigrateDoesNotResetEditedRates(t *testing.T) {
	conn := openTestDB(t)
	if _, err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := conn.Exec(
		`UPDATE material_rates SET points_per_kg = 999 WHERE material_type = 'pet'`,
	); err != nil {
		t.Fatalf("update rate: %v", err)
	}

	if _, err := Migrate(conn); err != nil {
		t.Fatalf("re-Migrate: %v", err)
	}

	var points int
	if err := conn.QueryRow(
		`SELECT points_per_kg FROM material_rates WHERE material_type='pet'`,
	).Scan(&points); err != nil {
		t.Fatalf("read rate: %v", err)
	}
	if points != 999 {
		t.Errorf("edited rate was reset to %d, want it preserved at 999", points)
	}
}

// TestForeignKeysAreEnforced verifies the pragma actually took effect. SQLite
// ignores FK constraints unless it is switched on per connection, which would
// make every REFERENCES clause in the schema decorative.
func TestForeignKeysAreEnforced(t *testing.T) {
	conn := openTestDB(t)
	if _, err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	_, err := conn.Exec(
		`INSERT INTO submissions (user_id, material_type) VALUES (9999, 'pet')`,
	)
	if err == nil {
		t.Fatal("inserted a submission for a non-existent user; FK not enforced")
	}
}

// TestStatusCheckConstraints verifies the lifecycle values from FR-5 and FR-14
// are enforced by the database, not only by application code.
func TestStatusCheckConstraints(t *testing.T) {
	conn := openTestDB(t)
	if _, err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := conn.Exec(
		`INSERT INTO users (phone_number, name, password_hash) VALUES ('+254700000000', 'Test', 'x')`,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err := conn.Exec(
		`INSERT INTO submissions (user_id, material_type, status) VALUES (1, 'pet', 'nonsense')`,
	)
	if err == nil {
		t.Error("accepted an invalid submission status; CHECK constraint missing")
	}

	_, err = conn.Exec(
		`INSERT INTO users (phone_number, name, password_hash, role) VALUES ('+254700000001', 'Test', 'x', 'wizard')`,
	)
	if err == nil {
		t.Error("accepted an invalid role; CHECK constraint missing")
	}
}

// columnsOf returns the column names of a table as a set.
func columnsOf(conn *sql.DB, table string) (map[string]bool, error) {
	rows, err := conn.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, sql.ErrNoRows
	}
	return cols, nil
}
