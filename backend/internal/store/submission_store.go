package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"zoa/backend/internal/models"
)

// ErrAlreadyFinalized is returned when a submission has already reached a
// terminal state. Handlers translate it to 409 — it is what stops a double-tap
// from crediting points twice (FR-16's sibling guarantee for submissions).
var ErrAlreadyFinalized = errors.New("submission has already been finalized")

// ErrNoRate is returned when no material_rates row exists for a material type.
// Crediting zero points silently would be worse: the user would see a verified
// submission worth nothing and have no way to tell it was a config gap.
var ErrNoRate = errors.New("no points rate configured for this material")

// SubmissionStore reads and writes submissions, and owns the points-crediting
// transaction.
type SubmissionStore struct {
	db *sql.DB
}

// NewSubmissionStore builds a submission store over conn.
func NewSubmissionStore(conn *sql.DB) *SubmissionStore {
	return &SubmissionStore{db: conn}
}

// SubmissionWithContext is a submission plus the joined display data the client
// needs: who submitted it, and what it earned.
type SubmissionWithContext struct {
	models.Submission

	// UserName is the submitter's name — the collector queue needs a person, not
	// a user id.
	UserName string `json:"-"`

	// PointsAwarded is the ledger total for this submission, nil until verified.
	PointsAwarded *int64 `json:"-"`
}

// submissionColumns is the canonical select list, in the order scanSubmission
// expects. The two trailing joined columns are not part of the table.
const submissionColumns = `
	s.id, s.user_id, s.material_type, s.estimated_qty_kg, s.verified_qty_kg,
	s.location, s.status, s.collector_id, s.created_at, s.verified_at,
	s.predicted_category, s.predicted_confidence, s.source_type,
	u.name,
	(SELECT SUM(l.points_delta) FROM points_ledger l WHERE l.submission_id = s.id)`

const submissionFrom = ` FROM submissions s JOIN users u ON u.id = s.user_id`

// CreateInput describes a new submission.
//
// A struct rather than a parameter list: Phase 2.5 added three optional fields,
// and five positional arguments of which three are nullable is an easy call to
// get silently wrong. Mirrors VerifyInput below.
type CreateInput struct {
	UserID         int64
	MaterialType   string
	EstimatedQtyKg float64
	Location       *string

	// --- Phase 2.5, all optional ---

	// PredictedCategory and PredictedConfidence record what the classifier said,
	// stored alongside MaterialType rather than instead of it (FR-22).
	PredictedCategory   *string
	PredictedConfidence *float64

	// SourceType is 'residential' or 'hotel'; required for organics (FR-24),
	// enforced by the handler.
	SourceType *string
}

// Create inserts a new submission in the `pending` state.
//
// Status is not a parameter: the client never sets one (App Flow §3), so there is
// no path by which a caller could create an already-verified submission.
func (s *SubmissionStore) Create(
	ctx context.Context,
	in CreateInput,
) (*SubmissionWithContext, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO submissions
			(user_id, material_type, estimated_qty_kg, location, status,
			 predicted_category, predicted_confidence, source_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.UserID, in.MaterialType, in.EstimatedQtyKg, in.Location, models.SubmissionPending,
		in.PredictedCategory, in.PredictedConfidence, in.SourceType,
	)
	if err != nil {
		return nil, fmt.Errorf("insert submission: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return s.ByID(ctx, id)
}

// ByID loads one submission with its joined context.
func (s *SubmissionStore) ByID(ctx context.Context, id int64) (*SubmissionWithContext, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+submissionColumns+submissionFrom+` WHERE s.id = ?`, id)
	return scanSubmission(row)
}

// ListFilter narrows a submission listing.
type ListFilter struct {
	// UserID limits results to one submitter. Zero means no restriction, which
	// only collector and admin callers are allowed to ask for.
	UserID int64

	// Status limits results to one status. Empty means all.
	Status string

	Limit  int
	Offset int
}

// List returns submissions newest-first, plus the total matching the filter
// (ignoring limit/offset) so the client can show a real count.
func (s *SubmissionStore) List(ctx context.Context, filter ListFilter) ([]*SubmissionWithContext, int64, error) {
	var conditions []string
	var args []any

	if filter.UserID != 0 {
		conditions = append(conditions, "s.user_id = ?")
		args = append(args, filter.UserID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "s.status = ?")
		args = append(args, filter.Status)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*)`+submissionFrom+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count submissions: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// Ordered by created_at then id: CURRENT_TIMESTAMP has one-second resolution
	// in SQLite, so several submissions in the same second would otherwise come
	// back in an arbitrary order and appear to shuffle between refreshes.
	query := `SELECT` + submissionColumns + submissionFrom + where +
		` ORDER BY s.created_at DESC, s.id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list submissions: %w", err)
	}
	defer rows.Close()

	var submissions []*SubmissionWithContext
	for rows.Next() {
		submission, err := scanSubmission(rows)
		if err != nil {
			return nil, 0, err
		}
		submissions = append(submissions, submission)
	}
	return submissions, total, rows.Err()
}

// VerifyInput describes a collector's decision about a submission.
type VerifyInput struct {
	// TargetStatus is `verified`, `rejected` or `collected`.
	TargetStatus string

	// VerifiedQtyKg is the weight the collector measured. Required for
	// `verified`, ignored otherwise.
	VerifiedQtyKg float64

	// MaterialType overrides the submitted type when the collector (or the AI)
	// got it wrong. Empty keeps the existing value.
	MaterialType string

	CollectorID int64
}

// VerifyResult reports what the transaction did.
type VerifyResult struct {
	Submission    *SubmissionWithContext
	PointsAwarded int64
	PointsBalance int64
	RateApplied   int64
	MaterialType  string
}

// Verify transitions a submission and, for `verified`, credits points — all in
// one transaction.
//
// The ordering matters and is the core data-integrity requirement (TRD §3):
// status transition, ledger entry and balance update either all land or none do.
// Points are always credited to the submission's owner, never to the collector
// performing the verification.
func (s *SubmissionStore) Verify(ctx context.Context, submissionID int64, input VerifyInput) (*VerifyResult, error) {
	// BEGIN IMMEDIATE takes the write lock up front rather than on first write
	// (docs/06_Backend_Schema.md §4). Without it, two concurrent verifications can
	// both begin, both read, and one fails at COMMIT having already done work.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin verify transaction: %w", err)
	}
	defer tx.Rollback() // no-op after a successful commit

	existing, err := scanSubmission(tx.QueryRowContext(ctx,
		`SELECT`+submissionColumns+submissionFrom+` WHERE s.id = ?`, submissionID))
	if err != nil {
		return nil, err
	}

	materialType := existing.MaterialType
	if input.MaterialType != "" {
		materialType = input.MaterialType
	}

	var result *VerifyResult
	switch input.TargetStatus {
	case models.SubmissionVerified:
		result, err = verifyAndCredit(ctx, tx, existing, materialType, input)
	case models.SubmissionCollected, models.SubmissionRejected:
		result, err = transitionOnly(ctx, tx, submissionID, materialType, input)
	default:
		return nil, fmt.Errorf("unsupported target status %q", input.TargetStatus)
	}
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit verify transaction: %w", err)
	}

	// Re-read outside the transaction so the response carries exactly what was
	// persisted, including database-generated timestamps.
	reloaded, err := s.ByID(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	result.Submission = reloaded
	return result, nil
}

// verifyAndCredit performs the crediting transition.
func verifyAndCredit(
	ctx context.Context,
	tx *sql.Tx,
	existing *SubmissionWithContext,
	materialType string,
	input VerifyInput,
) (*VerifyResult, error) {
	var pointsPerKg int64
	err := tx.QueryRowContext(ctx,
		`SELECT points_per_kg FROM material_rates WHERE material_type = ?`, materialType,
	).Scan(&pointsPerKg)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNoRate, materialType)
	}
	if err != nil {
		return nil, fmt.Errorf("read material rate: %w", err)
	}

	// Points are integers; weights are not. Round rather than truncate, so 4.98kg
	// of PET is not quietly worth the same as 4.0kg.
	points := int64(math.Round(input.VerifiedQtyKg * float64(pointsPerKg)))

	// The conditional UPDATE is the concurrency guard. Two simultaneous verifies
	// both reach here, but only one matches `status IN ('pending','collected')`;
	// the loser sees RowsAffected == 0 and is rejected without crediting.
	updated, err := tx.ExecContext(ctx, `
		UPDATE submissions
		   SET status = ?, verified_qty_kg = ?, material_type = ?,
		       collector_id = ?, verified_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status IN (?, ?)`,
		models.SubmissionVerified, input.VerifiedQtyKg, materialType,
		input.CollectorID, existing.ID,
		models.SubmissionPending, models.SubmissionCollected,
	)
	if err != nil {
		return nil, fmt.Errorf("update submission: %w", err)
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if affected != 1 {
		return nil, ErrAlreadyFinalized
	}

	// The ledger is the source of truth for balances; users.points_balance is a
	// denormalised read cache (docs/06 §2). Both are written here so they cannot
	// drift.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO points_ledger (user_id, submission_id, points_delta, reason)
		VALUES (?, ?, ?, ?)`,
		existing.UserID, existing.ID, points, models.ReasonSubmissionVerified,
	); err != nil {
		return nil, fmt.Errorf("insert ledger entry: %w", err)
	}

	// Relative update rather than reading the balance and writing a computed
	// value: the arithmetic happens in the database, so it stays correct even if
	// another transaction touched the row.
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET points_balance = points_balance + ? WHERE id = ?`,
		points, existing.UserID,
	); err != nil {
		return nil, fmt.Errorf("update points balance: %w", err)
	}

	var balance int64
	if err := tx.QueryRowContext(ctx,
		`SELECT points_balance FROM users WHERE id = ?`, existing.UserID,
	).Scan(&balance); err != nil {
		return nil, fmt.Errorf("read points balance: %w", err)
	}

	return &VerifyResult{
		PointsAwarded: points,
		PointsBalance: balance,
		RateApplied:   pointsPerKg,
		MaterialType:  materialType,
	}, nil
}

// transitionOnly handles `collected` and `rejected` — status changes that credit
// nothing.
func transitionOnly(
	ctx context.Context,
	tx *sql.Tx,
	submissionID int64,
	materialType string,
	input VerifyInput,
) (*VerifyResult, error) {
	// `collected` may only follow `pending`; `rejected` may follow either
	// non-terminal state. Neither may act on an already-verified submission,
	// which would otherwise strip points that have been spent.
	allowed := []any{models.SubmissionPending, models.SubmissionCollected}
	if input.TargetStatus == models.SubmissionCollected {
		allowed = []any{models.SubmissionPending}
	}

	args := []any{input.TargetStatus, materialType, input.CollectorID, submissionID}
	args = append(args, allowed...)

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(allowed)), ",")

	updated, err := tx.ExecContext(ctx, `
		UPDATE submissions
		   SET status = ?, material_type = ?, collector_id = ?
		 WHERE id = ? AND status IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("update submission: %w", err)
	}

	affected, err := updated.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if affected != 1 {
		return nil, ErrAlreadyFinalized
	}

	var balance int64
	if err := tx.QueryRowContext(ctx,
		`SELECT u.points_balance FROM users u
		  JOIN submissions s ON s.user_id = u.id WHERE s.id = ?`, submissionID,
	).Scan(&balance); err != nil {
		return nil, fmt.Errorf("read points balance: %w", err)
	}

	return &VerifyResult{
		PointsBalance: balance,
		MaterialType:  materialType,
	}, nil
}

// scanSubmission reads one submission row plus its two joined columns.
func scanSubmission(row rowScanner) (*SubmissionWithContext, error) {
	var s SubmissionWithContext
	err := row.Scan(
		&s.ID,
		&s.UserID,
		&s.MaterialType,
		&s.EstimatedQtyKg,
		&s.VerifiedQtyKg,
		&s.Location,
		&s.Status,
		&s.CollectorID,
		&s.CreatedAt,
		&s.VerifiedAt,
		&s.PredictedCategory,
		&s.PredictedConfidence,
		&s.SourceType,
		&s.UserName,
		&s.PointsAwarded,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan submission: %w", err)
	}
	return &s, nil
}
