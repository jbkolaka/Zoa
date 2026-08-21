package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"zoa/backend/internal/models"
)

// ErrPartnerMissing is returned when a voucher references a partner that does not
// exist. Distinct from ErrNotFound so a handler can report it as a field error on
// partner_id rather than as a 404 for the voucher itself.
var ErrPartnerMissing = errors.New("partner does not exist")

// ErrNothingToUpdate is returned when a patch carries no fields. Refused rather
// than treated as a no-op, because an empty body is far more likely a client bug
// than a deliberate request to change nothing.
var ErrNothingToUpdate = errors.New("no fields to update")

// CatalogStore owns administrative writes to partners and vouchers (Phase 5).
//
// Separate from VoucherStore, which is the read path the app uses and is filtered
// to active rows: everything here sees inactive rows too, because an admin who
// deactivated something must still be able to find it and turn it back on.
//
// Deletes are soft — `active = 0` — since vouchers are referenced by issued
// redemptions and a hard delete would break a code a user is holding.
type CatalogStore struct {
	db *sql.DB
}

// NewCatalogStore builds a catalog store over conn.
func NewCatalogStore(conn *sql.DB) *CatalogStore {
	return &CatalogStore{db: conn}
}

// --- partners ---

const partnerColumns = `id, name, logo_url, active`

// PartnerPatch describes a partner create or update.
//
// Every field is a pointer so a PATCH can carry only what changes; Create requires
// Name and defaults the rest.
type PartnerPatch struct {
	Name    *string
	LogoURL *string
	Active  *bool

	// LogoURLSet marks whether logo_url should change at all. A plain pointer cannot
	// distinguish "leave alone" from "clear it", and clearing a logo is a real
	// operation.
	LogoURLSet bool
}

// ListPartners returns every partner, inactive included, newest last.
func (s *CatalogStore) ListPartners(ctx context.Context) ([]*models.Partner, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+partnerColumns+` FROM partners ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list partners: %w", err)
	}
	defer rows.Close()

	var partners []*models.Partner
	for rows.Next() {
		partner, err := scanPartner(rows)
		if err != nil {
			return nil, err
		}
		partners = append(partners, partner)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partners: %w", err)
	}
	return partners, nil
}

// PartnerByID loads one partner regardless of its active flag.
func (s *CatalogStore) PartnerByID(ctx context.Context, id int64) (*models.Partner, error) {
	return scanPartner(s.db.QueryRowContext(ctx,
		`SELECT `+partnerColumns+` FROM partners WHERE id = ?`, id))
}

// CreatePartner inserts a partner.
func (s *CatalogStore) CreatePartner(ctx context.Context, in PartnerPatch) (*models.Partner, error) {
	// Active defaults to true: a partner is created because it is joining the
	// programme, so requiring a second call to switch it on would be busywork.
	active := true
	if in.Active != nil {
		active = *in.Active
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO partners (name, logo_url, active) VALUES (?, ?, ?)`,
		in.Name, in.LogoURL, active,
	)
	if err != nil {
		return nil, fmt.Errorf("insert partner: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return s.PartnerByID(ctx, id)
}

// UpdatePartner applies a partial update.
func (s *CatalogStore) UpdatePartner(ctx context.Context, id int64, in PartnerPatch) (*models.Partner, error) {
	var sets []string
	var args []any

	if in.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *in.Name)
	}
	if in.LogoURLSet {
		sets = append(sets, "logo_url = ?")
		args = append(args, in.LogoURL)
	}
	if in.Active != nil {
		sets = append(sets, "active = ?")
		args = append(args, *in.Active)
	}
	if len(sets) == 0 {
		return nil, ErrNothingToUpdate
	}

	args = append(args, id)
	result, err := s.db.ExecContext(ctx,
		`UPDATE partners SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("update partner: %w", err)
	}
	if err := mustAffectOne(result); err != nil {
		return nil, err
	}
	return s.PartnerByID(ctx, id)
}

// DeactivatePartner is the soft delete.
//
// Idempotent, and it deliberately does not touch the partner's vouchers: the
// catalogue already requires both the voucher and its partner to be active, so
// withdrawing a retailer hides its whole offer list without rewriting rows that
// issued redemptions still point at.
func (s *CatalogStore) DeactivatePartner(ctx context.Context, id int64) (*models.Partner, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE partners SET active = 0 WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("deactivate partner: %w", err)
	}
	if err := mustAffectOne(result); err != nil {
		return nil, err
	}
	return s.PartnerByID(ctx, id)
}

func scanPartner(row rowScanner) (*models.Partner, error) {
	var partner models.Partner
	err := row.Scan(&partner.ID, &partner.Name, &partner.LogoURL, &partner.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan partner: %w", err)
	}
	return &partner, nil
}

// --- vouchers ---

// VoucherPatch describes a voucher create or update.
type VoucherPatch struct {
	PartnerID     *int64
	Title         *string
	PointsCost    *int64
	DiscountType  *string
	DiscountValue *float64
	ExpiryDays    *int64
	Active        *bool

	// StockRemainingSet marks whether stock should change at all; StockRemaining is
	// then the new value, with nil meaning *unlimited*. Two fields rather than one
	// pointer because "leave stock alone" and "make this unlimited" are different
	// requests and a single pointer cannot tell them apart.
	StockRemainingSet bool
	StockRemaining    *int64
}

// ListVouchersForAdmin returns every voucher with its partner, inactive included.
func (s *CatalogStore) ListVouchersForAdmin(ctx context.Context) ([]*VoucherWithPartner, error) {
	// No activeOnly filter, unlike VoucherStore.List — the point of this view is to
	// show what the catalogue is hiding.
	rows, err := s.db.QueryContext(ctx,
		`SELECT`+voucherColumns+voucherFrom+` ORDER BY v.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list vouchers: %w", err)
	}
	defer rows.Close()

	var vouchers []*VoucherWithPartner
	for rows.Next() {
		voucher, err := scanVoucher(rows)
		if err != nil {
			return nil, err
		}
		vouchers = append(vouchers, voucher)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vouchers: %w", err)
	}
	return vouchers, nil
}

// VoucherByIDForAdmin loads one voucher regardless of active flags.
func (s *CatalogStore) VoucherByIDForAdmin(ctx context.Context, id int64) (*VoucherWithPartner, error) {
	return scanVoucher(s.db.QueryRowContext(ctx,
		`SELECT`+voucherColumns+voucherFrom+` WHERE v.id = ?`, id))
}

// CreateVoucher inserts a voucher.
func (s *CatalogStore) CreateVoucher(ctx context.Context, in VoucherPatch) (*VoucherWithPartner, error) {
	// Checked up front so a bad partner_id is a field error rather than an opaque
	// foreign-key failure. Foreign keys are enforced, so this is belt-and-braces —
	// but the braces are what produce a usable message.
	if err := s.requirePartner(ctx, *in.PartnerID); err != nil {
		return nil, err
	}

	active := true
	if in.Active != nil {
		active = *in.Active
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO vouchers
			(partner_id, title, points_cost, discount_type, discount_value,
			 expiry_days, stock_remaining, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		*in.PartnerID, *in.Title, *in.PointsCost, *in.DiscountType, *in.DiscountValue,
		*in.ExpiryDays, in.StockRemaining, active,
	)
	if err != nil {
		return nil, fmt.Errorf("insert voucher: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return s.VoucherByIDForAdmin(ctx, id)
}

// UpdateVoucher applies a partial update.
//
// points_cost is editable, and deliberately has no effect on codes already issued:
// a redemption records what it cost in points_ledger at the time, so repricing an
// offer cannot retroactively change what someone paid.
func (s *CatalogStore) UpdateVoucher(ctx context.Context, id int64, in VoucherPatch) (*VoucherWithPartner, error) {
	if in.PartnerID != nil {
		if err := s.requirePartner(ctx, *in.PartnerID); err != nil {
			return nil, err
		}
	}

	var sets []string
	var args []any

	appendSet := func(column string, value any) {
		sets = append(sets, column+" = ?")
		args = append(args, value)
	}

	if in.PartnerID != nil {
		appendSet("partner_id", *in.PartnerID)
	}
	if in.Title != nil {
		appendSet("title", *in.Title)
	}
	if in.PointsCost != nil {
		appendSet("points_cost", *in.PointsCost)
	}
	if in.DiscountType != nil {
		appendSet("discount_type", *in.DiscountType)
	}
	if in.DiscountValue != nil {
		appendSet("discount_value", *in.DiscountValue)
	}
	if in.ExpiryDays != nil {
		appendSet("expiry_days", *in.ExpiryDays)
	}
	if in.StockRemainingSet {
		// in.StockRemaining may be nil, which writes NULL — unlimited.
		appendSet("stock_remaining", in.StockRemaining)
	}
	if in.Active != nil {
		appendSet("active", *in.Active)
	}
	if len(sets) == 0 {
		return nil, ErrNothingToUpdate
	}

	args = append(args, id)
	result, err := s.db.ExecContext(ctx,
		`UPDATE vouchers SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("update voucher: %w", err)
	}
	if err := mustAffectOne(result); err != nil {
		return nil, err
	}
	return s.VoucherByIDForAdmin(ctx, id)
}

// DeactivateVoucher is the soft delete.
//
// Never a hard delete: redemptions.voucher_id points at this row, and an issued
// code has to keep resolving to the offer it bought long after the offer is
// withdrawn.
func (s *CatalogStore) DeactivateVoucher(ctx context.Context, id int64) (*VoucherWithPartner, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE vouchers SET active = 0 WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("deactivate voucher: %w", err)
	}
	if err := mustAffectOne(result); err != nil {
		return nil, err
	}
	return s.VoucherByIDForAdmin(ctx, id)
}

// requirePartner reports ErrPartnerMissing when id names no partner.
func (s *CatalogStore) requirePartner(ctx context.Context, id int64) error {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM partners WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPartnerMissing
	}
	if err != nil {
		return fmt.Errorf("check partner %d: %w", id, err)
	}
	return nil
}

// mustAffectOne turns "no such row" into ErrNotFound.
//
// An UPDATE that matched nothing is the same situation as a failed lookup, and the
// handler should answer 404 rather than reporting success for a row that does not
// exist.
func mustAffectOne(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected != 1 {
		return ErrNotFound
	}
	return nil
}
