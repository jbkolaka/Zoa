package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"zoa/backend/internal/models"
)

// ErrInsufficientPoints is returned when a redemption would take a balance below
// zero. Handlers translate it to 409; the contract also requires the string
// `insufficient_points` in the message, so a client can tell "you cannot afford
// this" apart from the other 409 on the same endpoint.
var ErrInsufficientPoints = errors.New("insufficient_points")

// ErrOutOfStock is returned when a voucher's last unit went to someone else.
// Distinct from ErrInsufficientPoints because nothing about the user's balance is
// wrong — retrying will not help, and the copy has to say so.
var ErrOutOfStock = errors.New("voucher is out of stock")

// QRPayloadPrefix is the scheme the QR code encodes. A deep link rather than the
// bare UUID, so a scanner that understands it can open the verification screen
// directly instead of leaving a cashier to retype 36 characters.
const QRPayloadPrefix = "zoa://redeem/"

// CodeNotRedeemableError explains why a code could not become `used`.
//
// A typed error rather than another sentinel because the 409 prose has to name a
// timestamp — "code already used on <used_at>" is what settles an argument at the
// till, and a bare sentinel cannot carry one. Handlers match it with errors.As.
type CodeNotRedeemableError struct {
	// Status is what blocked the transition: `used` or `expired`.
	Status string

	// UsedAt is when the code was spent. Set only when Status is `used`, and even
	// then it may be nil — the loser of a concurrent verify holds a pre-transaction
	// snapshot — so the message falls back rather than printing a zero time.
	UsedAt *time.Time

	// ExpiredAt is the derived expiry. Set only when Status is `expired`.
	ExpiredAt time.Time
}

func (e *CodeNotRedeemableError) Error() string {
	if e.Status == models.RedemptionUsed && e.UsedAt != nil {
		return fmt.Sprintf("code already used on %s", e.UsedAt.Format(time.RFC3339))
	}
	return fmt.Sprintf("code is %s", e.Status)
}

// RedemptionStore owns the two transactions Phase 4 rests on: the atomic redeem,
// and the guarded issued → used transition.
type RedemptionStore struct {
	db *sql.DB
}

// NewRedemptionStore builds a redemption store over conn.
func NewRedemptionStore(conn *sql.DB) *RedemptionStore {
	return &RedemptionStore{db: conn}
}

// RedemptionWithContext is a redemption plus everything shown beside it: the
// voucher it bought (with its partner), and the owner's name — a cashier
// confirms a person, not a user id.
type RedemptionWithContext struct {
	models.Redemption

	Voucher VoucherWithPartner `json:"-"`

	UserName string `json:"-"`
}

// Expiry is issued_at plus the voucher's expiry_days.
//
// Derived rather than stored: the schema has no expiry column, and deriving it
// keeps an admin editing expiry_days from retroactively rewriting codes that are
// already in users' hands.
func (r *RedemptionWithContext) Expiry() time.Time {
	return r.IssuedAt.AddDate(0, 0, int(r.Voucher.ExpiryDays))
}

// QRPayload is the deep link encoded into the QR shown at the till.
func (r *RedemptionWithContext) QRPayload() string {
	return QRPayloadPrefix + r.RedemptionCode
}

// IsExpired reports whether the code is past its expiry as of now. The boundary
// counts as expired: a code valid "for 30 days" should not still work the instant
// day 30 has elapsed.
func (r *RedemptionWithContext) IsExpired(now time.Time) bool {
	return !now.Before(r.Expiry())
}

// redemptionColumns is the canonical select list, in the order scanRedemption
// expects. The voucher, partner and owner columns are joined, not part of the
// table.
const redemptionColumns = `
	r.id, r.user_id, r.voucher_id, r.redemption_code, r.status,
	r.issued_at, r.used_at, r.verified_by,
	v.id, v.partner_id, v.title, v.points_cost, v.discount_type,
	v.discount_value, v.expiry_days, v.stock_remaining, v.active,
	p.id, p.name, p.logo_url, p.active,
	u.name`

// redemptionFrom deliberately carries no `active = 1` filter, unlike the
// catalogue's voucherFrom + activeOnly pair. An issued code was paid for; a
// retailer later leaving the programme must not strand a user standing at a till
// holding one.
const redemptionFrom = ` FROM redemptions r
	JOIN vouchers v ON v.id = r.voucher_id
	JOIN partners p ON p.id = v.partner_id
	JOIN users    u ON u.id = r.user_id`

// ByID loads one redemption with its joined context.
func (s *RedemptionStore) ByID(ctx context.Context, id int64) (*RedemptionWithContext, error) {
	return scanRedemption(s.db.QueryRowContext(ctx,
		`SELECT`+redemptionColumns+redemptionFrom+` WHERE r.id = ?`, id))
}

// ByCode loads one redemption by its code — the partner-side lookup.
func (s *RedemptionStore) ByCode(ctx context.Context, code string) (*RedemptionWithContext, error) {
	return scanRedemption(s.db.QueryRowContext(ctx,
		`SELECT`+redemptionColumns+redemptionFrom+` WHERE r.redemption_code = ?`, code))
}

// ListForUser returns one user's redemptions, newest first.
func (s *RedemptionStore) ListForUser(ctx context.Context, userID int64) ([]*RedemptionWithContext, error) {
	// Ordered by issued_at then id: CURRENT_TIMESTAMP has one-second resolution in
	// SQLite, so two codes issued in the same second would otherwise come back in
	// an arbitrary order and appear to shuffle between refreshes.
	rows, err := s.db.QueryContext(ctx,
		`SELECT`+redemptionColumns+redemptionFrom+
			` WHERE r.user_id = ? ORDER BY r.issued_at DESC, r.id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list redemptions: %w", err)
	}
	defer rows.Close()

	var redemptions []*RedemptionWithContext
	for rows.Next() {
		redemption, err := scanRedemption(rows)
		if err != nil {
			return nil, err
		}
		redemptions = append(redemptions, redemption)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate redemptions: %w", err)
	}
	return redemptions, nil
}

// RedeemResult reports what the redemption transaction did.
type RedeemResult struct {
	Redemption    *RedemptionWithContext
	PointsSpent   int64
	PointsBalance int64
}

// Redeem spends points on a voucher and issues a code — all in one transaction.
//
// The ordering is the core data-integrity requirement for Phase 4: balance check,
// deduction, stock decrement, code generation, redemption row and ledger entry
// either all land or none do. A failure anywhere rolls the deduction back, so
// there is no state in which the points are gone and no code exists.
func (s *RedemptionStore) Redeem(ctx context.Context, userID, voucherID int64) (*RedeemResult, error) {
	// BEGIN IMMEDIATE, via the _txlock=immediate DSN in db.Open — docs/06 §4 asks
	// for it here specifically. A deferred transaction only takes the write lock at
	// its first write, so two concurrent redemptions could both read an affordable
	// balance before either had deducted.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin redeem transaction: %w", err)
	}
	defer tx.Rollback() // no-op after a successful commit

	// active-filtered, reusing the catalogue's own visibility rule: an inactive
	// voucher, or one whose partner has left, answers exactly as a missing one does
	// — so an offer that will not be honoured cannot be redeemed by knowing its id.
	voucher, err := scanVoucher(tx.QueryRowContext(ctx,
		`SELECT`+voucherColumns+voucherFrom+` WHERE v.id = ? AND`+activeOnly, voucherID))
	if err != nil {
		return nil, err
	}

	// The conditional UPDATE is the concurrency guard, and the reason the balance
	// is never read-then-written: two simultaneous redemptions both reach here, but
	// only one can still satisfy `points_balance >= cost`. The loser sees
	// RowsAffected == 0 and is refused before any code exists. Doing the arithmetic
	// in the database also keeps it correct if another transaction touched the row.
	deducted, err := tx.ExecContext(ctx, `
		UPDATE users
		   SET points_balance = points_balance - ?
		 WHERE id = ? AND points_balance >= ?`,
		voucher.PointsCost, userID, voucher.PointsCost,
	)
	if err != nil {
		return nil, fmt.Errorf("deduct points: %w", err)
	}
	affected, err := deducted.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if affected != 1 {
		return nil, ErrInsufficientPoints
	}

	// Stock, same guard. NULL means unlimited (docs/06 §2) and SQLite evaluates
	// NULL - 1 as NULL, so this one statement covers both cases: a limited voucher
	// decrements, an unlimited one stays unlimited.
	stocked, err := tx.ExecContext(ctx, `
		UPDATE vouchers
		   SET stock_remaining = stock_remaining - 1
		 WHERE id = ? AND active = 1
		   AND (stock_remaining IS NULL OR stock_remaining > 0)`,
		voucher.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("decrement stock: %w", err)
	}
	affected, err = stocked.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if affected != 1 {
		return nil, ErrOutOfStock
	}

	// A v4 UUID, per docs/06 §2. Uniqueness is enforced by idx_redemptions_code as
	// well, so a collision fails the insert and rolls the whole transaction back
	// rather than issuing a duplicate code.
	code := uuid.NewString()

	// Status is not a parameter: the client never sets one (App Flow §3), so there
	// is no path by which a caller could issue an already-used code.
	inserted, err := tx.ExecContext(ctx, `
		INSERT INTO redemptions (user_id, voucher_id, redemption_code, status)
		VALUES (?, ?, ?, ?)`,
		userID, voucher.ID, code, models.RedemptionIssued,
	)
	if err != nil {
		return nil, fmt.Errorf("insert redemption: %w", err)
	}
	redemptionID, err := inserted.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	// The ledger is the source of truth for balances and users.points_balance is a
	// denormalised read cache (docs/06 §2), so both move inside this transaction
	// and cannot drift. The delta is negative — this is a spend, and SUM over the
	// ledger must still equal the balance afterwards.
	//
	// Written after the insert so redemption_id resolves against a real row:
	// foreign keys are enforced (PRAGMA foreign_keys=ON), so the reverse order
	// would fail.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO points_ledger (user_id, redemption_id, points_delta, reason)
		VALUES (?, ?, ?, ?)`,
		userID, redemptionID, -voucher.PointsCost, models.ReasonVoucherRedeemed,
	); err != nil {
		return nil, fmt.Errorf("insert ledger entry: %w", err)
	}

	var balance int64
	if err := tx.QueryRowContext(ctx,
		`SELECT points_balance FROM users WHERE id = ?`, userID,
	).Scan(&balance); err != nil {
		return nil, fmt.Errorf("read points balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit redeem transaction: %w", err)
	}

	// Re-read outside the transaction so the response carries exactly what was
	// persisted — including the database-generated issued_at the expiry derives
	// from.
	reloaded, err := s.ByID(ctx, redemptionID)
	if err != nil {
		return nil, err
	}

	return &RedeemResult{
		Redemption:    reloaded,
		PointsSpent:   voucher.PointsCost,
		PointsBalance: balance,
	}, nil
}

// Verify transitions a code from `issued` to `used`.
//
// Idempotent by design (FR-16): the transition is guarded inside the transaction,
// so two simultaneous verifications produce exactly one success and one conflict.
// This is the anti-double-spend guarantee the demo rests on.
func (s *RedemptionStore) Verify(ctx context.Context, code string, verifierID int64) (*RedemptionWithContext, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin verify transaction: %w", err)
	}
	defer tx.Rollback()

	existing, err := scanRedemption(tx.QueryRowContext(ctx,
		`SELECT`+redemptionColumns+redemptionFrom+` WHERE r.redemption_code = ?`, code))
	if err != nil {
		return nil, err
	}

	switch existing.Status {
	case models.RedemptionUsed:
		return nil, &CodeNotRedeemableError{
			Status: models.RedemptionUsed,
			UsedAt: existing.UsedAt,
		}
	case models.RedemptionExpired:
		return nil, &CodeNotRedeemableError{
			Status:    models.RedemptionExpired,
			ExpiredAt: existing.Expiry(),
		}
	}

	// Past expiry, the row is transitioned rather than merely refused, so a stale
	// code stops reading as `issued` the first time anyone looks at it
	// (docs/API_CONTRACT.md § Phase 4). That transition has to survive, so this
	// path commits and *then* reports the conflict.
	if existing.IsExpired(time.Now()) {
		if _, err := tx.ExecContext(ctx,
			`UPDATE redemptions SET status = ? WHERE id = ? AND status = ?`,
			models.RedemptionExpired, existing.ID, models.RedemptionIssued,
		); err != nil {
			return nil, fmt.Errorf("expire redemption: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit expiry transition: %w", err)
		}
		return nil, &CodeNotRedeemableError{
			Status:    models.RedemptionExpired,
			ExpiredAt: existing.Expiry(),
		}
	}

	// The status read above only shapes the message; *this* is the guarantee. Two
	// simultaneous verifies both read `issued`, but only one still matches
	// `status = 'issued'` here — the loser is refused, so one code cannot be spent
	// twice.
	updated, err := tx.ExecContext(ctx, `
		UPDATE redemptions
		   SET status = ?, used_at = CURRENT_TIMESTAMP, verified_by = ?
		 WHERE redemption_code = ? AND status = ?`,
		models.RedemptionUsed, verifierID, code, models.RedemptionIssued,
	)
	if err != nil {
		return nil, fmt.Errorf("update redemption: %w", err)
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if affected != 1 {
		// Lost the race. UsedAt comes from the pre-transaction snapshot and so is
		// nil here; the handler's message falls back to the undated wording rather
		// than issuing another query purely for prose.
		return nil, &CodeNotRedeemableError{
			Status: models.RedemptionUsed,
			UsedAt: existing.UsedAt,
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit verify transaction: %w", err)
	}

	// Re-read so the response carries the database-generated used_at.
	return s.ByCode(ctx, code)
}

// scanRedemption reads one redemption row plus its joined voucher, partner and
// owner columns.
func scanRedemption(row rowScanner) (*RedemptionWithContext, error) {
	var r RedemptionWithContext
	err := row.Scan(
		&r.ID,
		&r.UserID,
		&r.VoucherID,
		&r.RedemptionCode,
		&r.Status,
		&r.IssuedAt,
		&r.UsedAt,
		&r.VerifiedBy,
		&r.Voucher.ID,
		&r.Voucher.PartnerID,
		&r.Voucher.Title,
		&r.Voucher.PointsCost,
		&r.Voucher.DiscountType,
		&r.Voucher.DiscountValue,
		&r.Voucher.ExpiryDays,
		&r.Voucher.StockRemaining,
		&r.Voucher.Active,
		&r.Voucher.Partner.ID,
		&r.Voucher.Partner.Name,
		&r.Voucher.Partner.LogoURL,
		&r.Voucher.Partner.Active,
		&r.UserName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan redemption: %w", err)
	}
	return &r, nil
}
