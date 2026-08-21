// Package store holds the database queries. `db` owns the connection and
// migrations; `store` owns the SQL; `handlers` own HTTP. Keeping queries out of
// handlers means the transactional rules (points always move through
// points_ledger) live in one place rather than being restated per endpoint.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"zoa/backend/internal/models"
)

// ErrNotFound is returned when a row does not exist. Handlers translate this to
// a 404 without leaking which lookup failed.
var ErrNotFound = errors.New("not found")

// ErrPhoneTaken is returned when a registration collides with an existing
// account, detected via the UNIQUE constraint on users.phone_number.
var ErrPhoneTaken = errors.New("phone number already registered")

// UserStore reads and writes the users table.
type UserStore struct {
	db *sql.DB
}

// NewUserStore builds a user store over conn.
func NewUserStore(conn *sql.DB) *UserStore {
	return &UserStore{db: conn}
}

// userColumns is the canonical select list, in the order scanUser expects.
const userColumns = `id, phone_number, name, password_hash, role, points_balance, created_at`

// Create inserts a new user and returns the stored row.
//
// The caller supplies an already-hashed password and an already-normalised
// phone number; this layer does not know how either is produced.
func (s *UserStore) Create(ctx context.Context, phone, name, passwordHash, role string) (*models.User, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users (phone_number, name, password_hash, role)
		VALUES (?, ?, ?, ?)`,
		phone, name, passwordHash, role,
	)
	if err != nil {
		// Rather than pre-checking for an existing phone number — which races
		// with a concurrent registration — let the UNIQUE index decide and
		// translate the violation here.
		if isUniqueViolation(err) {
			return nil, ErrPhoneTaken
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return s.ByID(ctx, id)
}

// ByID loads a user by primary key.
func (s *UserStore) ByID(ctx context.Context, id int64) (*models.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// ByPhone loads a user by their (normalised) phone number — the login lookup.
func (s *UserStore) ByPhone(ctx context.Context, phone string) (*models.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE phone_number = ?`, phone)
	return scanUser(row)
}

// Count returns the number of registered users, used by /admin/stats and to
// decide whether demo seeding has already run.
func (s *UserStore) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanUser reads one user row, mapping "no rows" to ErrNotFound.
func scanUser(row rowScanner) (*models.User, error) {
	var user models.User
	err := row.Scan(
		&user.ID,
		&user.PhoneNumber,
		&user.Name,
		&user.PasswordHash,
		&user.Role,
		&user.PointsBalance,
		&user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &user, nil
}

// isUniqueViolation reports whether err came from a UNIQUE constraint.
//
// Matched on message text because modernc.org/sqlite does not expose a typed
// constraint error; the driver's message is stable ("UNIQUE constraint failed:
// users.phone_number").
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "UNIQUE CONSTRAINT FAILED")
}
