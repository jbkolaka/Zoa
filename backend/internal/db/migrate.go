package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

// Migrations are embedded so the binary carries its own schema — no loose .sql
// files to ship alongside the container image.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies every migration not yet recorded in schema_migrations, in
// filename order, each inside its own transaction. Already-applied migrations
// are skipped, so this is safe to run on every boot.
//
// Adding a migration means dropping a new NNN_name.sql into migrations/ — this
// is how Phase 2.5 will add predicted_category / predicted_confidence /
// source_type to submissions without rewriting 001.
func Migrate(conn *sql.DB) ([]string, error) {
	if _, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(conn)
	if err != nil {
		return nil, err
	}

	names, err := migrationNames()
	if err != nil {
		return nil, err
	}

	var ran []string
	for _, name := range names {
		if applied[name] {
			continue
		}
		if err := applyOne(conn, name); err != nil {
			return ran, err
		}
		ran = append(ran, name)
	}
	return ran, nil
}

func appliedVersions(conn *sql.DB) (map[string]bool, error) {
	rows, err := conn.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // zero-padded numeric prefixes make lexical order correct
	return names, nil
}

// applyOne runs a single migration file and records it, atomically. If the SQL
// fails the transaction rolls back and the version is not recorded, so a fixed
// migration can simply be re-run.
func applyOne(conn *sql.DB, name string) error {
	stmts, err := migrationFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer tx.Rollback() // no-op once committed

	if _, err := tx.Exec(string(stmts)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}
