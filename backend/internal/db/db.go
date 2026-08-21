// Package db owns the SQLite connection and schema migration.
package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, no CGO (see docs/03_Tech_Stack.md §3)
)

// Open connects to the SQLite database at path and applies the pragmas the
// schema doc requires, then verifies the connection is live.
//
// Pragmas, per docs/06_Backend_Schema.md §4:
//   - foreign_keys=ON   SQLite does not enforce FKs by default.
//   - busy_timeout      SQLite serialises writes; without this, a concurrent
//     writer fails instantly with SQLITE_BUSY instead of waiting its turn.
//   - journal_mode=WAL  Lets reads proceed during a write, which is what makes
//     the demo tolerable on a single-file DB.
//
// _txlock=immediate makes database/sql issue `BEGIN IMMEDIATE` instead of a
// deferred `BEGIN`, as §4 requires for the points and redemption transactions.
// A deferred transaction only takes the write lock at its first write, so two
// concurrent verifications could both read, both decide to credit, and one would
// fail at COMMIT having already done half its work.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_txlock=immediate&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
		path,
	)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	// SQLite tolerates exactly one writer. Capping the pool at a single
	// connection turns "database is locked" errors into a queue, which is the
	// behaviour we want for the redemption transaction (docs/06 §4).
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(time.Hour)

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}

	if err := verifyPragmas(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// verifyPragmas asserts foreign key enforcement actually took effect. A silently
// ignored pragma would mean the FK constraints in the schema are decorative.
func verifyPragmas(conn *sql.DB) error {
	var fk int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("foreign_keys pragma is off (got %d) — FK constraints would not be enforced", fk)
	}
	return nil
}
