package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"zoa/backend/internal/models"
)

// VoucherStore reads the partner voucher catalogue.
//
// Read-only: redeeming is Phase 4's atomic transaction and lives with the
// redemption store, so nothing here mutates stock or balances.
type VoucherStore struct {
	db *sql.DB
}

// NewVoucherStore builds a voucher store over conn.
func NewVoucherStore(conn *sql.DB) *VoucherStore {
	return &VoucherStore{db: conn}
}

// VoucherWithPartner is a voucher plus the partner it belongs to.
//
// The partner travels with the voucher because the catalogue is useless without
// it — "10% off your basket" means nothing without a shop name — and fetching it
// separately would be one request per row (docs/API_CONTRACT.md § Phase 3
// embeds it for exactly this reason).
type VoucherWithPartner struct {
	models.Voucher

	Partner models.Partner `json:"partner"`
}

// voucherColumns is the canonical select list, in the order scanVoucher expects.
const voucherColumns = `
	v.id, v.partner_id, v.title, v.points_cost, v.discount_type,
	v.discount_value, v.expiry_days, v.stock_remaining, v.active,
	p.id, p.name, p.logo_url, p.active`

const voucherFrom = ` FROM vouchers v JOIN partners p ON p.id = v.partner_id`

// activeOnly is the catalogue's baseline visibility rule.
//
// A voucher is only listable when it is active *and* its partner is: a retailer
// leaving the programme takes its whole offer list with it, and filtering on the
// voucher alone would keep advertising discounts nobody will honour.
const activeOnly = ` v.active = 1 AND p.active = 1`

// VoucherFilter narrows a catalogue listing.
type VoucherFilter struct {
	// PartnerID limits results to one partner. Zero means all partners.
	PartnerID int64

	// AffordableFor, when non-nil, keeps only vouchers costing at most this many
	// points. A pointer rather than an int64 so "affordable to someone with zero
	// points" stays expressible and distinct from "no filter".
	AffordableFor *int64
}

// List returns the visible catalogue, cheapest first.
func (s *VoucherStore) List(ctx context.Context, filter VoucherFilter) ([]*VoucherWithPartner, error) {
	conditions := []string{activeOnly}
	var args []any

	if filter.PartnerID != 0 {
		conditions = append(conditions, "v.partner_id = ?")
		args = append(args, filter.PartnerID)
	}
	if filter.AffordableFor != nil {
		conditions = append(conditions, "v.points_cost <= ?")
		args = append(args, *filter.AffordableFor)
	}

	// Out-of-stock vouchers are hidden rather than shown greyed out: stock is
	// per-voucher and a user who taps one only to be refused at redemption has
	// been misled by the catalogue. NULL means unlimited, so it must pass.
	conditions = append(conditions, "(v.stock_remaining IS NULL OR v.stock_remaining > 0)")

	// Cheapest first, then id: this puts what the user can afford soonest at the
	// top, and the id tiebreak keeps equal-cost vouchers from shuffling between
	// requests the way an unstable sort would.
	query := `SELECT` + voucherColumns + voucherFrom +
		` WHERE ` + strings.Join(conditions, " AND ") +
		` ORDER BY v.points_cost ASC, v.id ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
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

// ByID loads one visible voucher.
//
// Returns ErrNotFound for a voucher that is missing, inactive, or whose partner
// is inactive — the same answer in every case, so an inactive offer cannot be
// distinguished from a nonexistent one by probing ids.
func (s *VoucherStore) ByID(ctx context.Context, id int64) (*VoucherWithPartner, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+voucherColumns+voucherFrom+` WHERE v.id = ? AND`+activeOnly, id)

	return scanVoucher(row)
}

// Partners returns every active partner, for a filter control.
func (s *VoucherStore) Partners(ctx context.Context) ([]*models.Partner, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, logo_url, active FROM partners WHERE active = 1 ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list partners: %w", err)
	}
	defer rows.Close()

	var partners []*models.Partner
	for rows.Next() {
		var partner models.Partner
		if err := rows.Scan(&partner.ID, &partner.Name, &partner.LogoURL, &partner.Active); err != nil {
			return nil, fmt.Errorf("scan partner: %w", err)
		}
		partners = append(partners, &partner)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate partners: %w", err)
	}
	return partners, nil
}

// scanVoucher reads one row in voucherColumns order.
func scanVoucher(row rowScanner) (*VoucherWithPartner, error) {
	var v VoucherWithPartner
	err := row.Scan(
		&v.ID,
		&v.PartnerID,
		&v.Title,
		&v.PointsCost,
		&v.DiscountType,
		&v.DiscountValue,
		&v.ExpiryDays,
		&v.StockRemaining,
		&v.Active,
		&v.Partner.ID,
		&v.Partner.Name,
		&v.Partner.LogoURL,
		&v.Partner.Active,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan voucher: %w", err)
	}
	return &v, nil
}
