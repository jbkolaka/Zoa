// Package models defines the domain entities. Every struct mirrors a table in
// docs/06_Backend_Schema.md one-to-one: same field set, same names, same types.
// JSON tags use the column names so the API contract and the schema stay
// readable against each other.
package models

import "time"

// ---------- enums ----------

// Roles (docs/02_Technical_Requirements.md FR-3).
const (
	RoleUser         = "user"
	RoleCollector    = "collector"
	RolePartnerStaff = "partner_staff"
	RoleAdmin        = "admin"
)

// Submission status lifecycle (FR-5). Transitions are server-enforced only.
const (
	SubmissionPending   = "pending"
	SubmissionCollected = "collected"
	SubmissionVerified  = "verified"
	SubmissionRejected  = "rejected"
)

// Redemption status lifecycle (FR-14). 'used' and 'expired' are terminal.
const (
	RedemptionIssued  = "issued"
	RedemptionUsed    = "used"
	RedemptionExpired = "expired"
)

// Discount types (FR-11).
const (
	DiscountPercentage = "percentage"
	DiscountFixed      = "fixed"
)

// Points ledger reasons — the audit trail's "why" (docs/06 §2 points_ledger).
const (
	ReasonSubmissionVerified = "submission_verified"
	ReasonVoucherRedeemed    = "voucher_redeemed"
)

// MaterialTaxonomy is the closed set of material types from TRD §2.6, ordered
// by group. These are the only valid submissions.material_type values.
var MaterialTaxonomy = []MaterialType{
	{Key: "pet", Group: "plastics", Label: "PET bottles"},
	{Key: "hdpe", Group: "plastics", Label: "HDPE containers & jerricans"},
	{Key: "ldpe", Group: "plastics", Label: "LDPE bags & film"},
	{Key: "pp", Group: "plastics", Label: "PP caps & tubs"},
	{Key: "ps", Group: "plastics", Label: "PS foam & rigid"},
	{Key: "other_plastic", Group: "plastics", Label: "Other / mixed plastic"},
	{Key: "cardboard", Group: "paper", Label: "Cardboard"},
	{Key: "mixed_paper", Group: "paper", Label: "Mixed / office paper"},
	{Key: "glass_clear", Group: "glass", Label: "Clear glass"},
	{Key: "glass_colored", Group: "glass", Label: "Coloured glass"},
	{Key: "aluminum", Group: "metal", Label: "Aluminium cans"},
	{Key: "steel_tin", Group: "metal", Label: "Steel & tin"},
	{Key: "food_waste", Group: "organic", Label: "Food waste"},
	{Key: "garden_waste", Group: "organic", Label: "Garden & yard waste"},
}

// MaterialType is one entry in the taxonomy.
type MaterialType struct {
	Key   string `json:"key"`
	Group string `json:"group"`
	Label string `json:"label"`
}

// IsValidMaterialType reports whether key is in the taxonomy.
func IsValidMaterialType(key string) bool {
	for _, m := range MaterialTaxonomy {
		if m.Key == key {
			return true
		}
	}
	return false
}

// ---------- entities ----------

// User maps to the users table. PasswordHash is never serialised.
type User struct {
	ID            int64     `json:"id"`
	PhoneNumber   string    `json:"phone_number"`
	Name          string    `json:"name"`
	PasswordHash  string    `json:"-"`
	Role          string    `json:"role"`
	PointsBalance int64     `json:"points_balance"`
	CreatedAt     time.Time `json:"created_at"`
}

// Submission maps to the submissions table.
// Nullable columns are pointers so they serialise as JSON null rather than a
// zero value that would read as "0 kg verified" or "collected by user 0".
type Submission struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	MaterialType   string     `json:"material_type"`
	EstimatedQtyKg *float64   `json:"estimated_qty_kg"`
	VerifiedQtyKg  *float64   `json:"verified_qty_kg"`
	Location       *string    `json:"location"`
	Status         string     `json:"status"`
	CollectorID    *int64     `json:"collector_id"`
	CreatedAt      time.Time  `json:"created_at"`
	VerifiedAt     *time.Time `json:"verified_at"`

	// --- Phase 2.5 (migration 003) ---

	// PredictedCategory is what the classifier guessed, preserved even after a
	// collector corrects MaterialType — that disagreement is the accuracy
	// metric (FR-22), so it must survive being proven wrong.
	PredictedCategory *string `json:"predicted_category"`

	// PredictedConfidence is the model's confidence in [0,1]. Nil when no photo
	// was classified, which is distinct from 0.0.
	PredictedConfidence *float64 `json:"predicted_confidence"`

	// SourceType is 'residential' or 'hotel' (FR-4a), required for organics.
	SourceType *string `json:"source_type"`
}

// Source types for submissions (FR-4a).
const (
	SourceResidential = "residential"
	SourceHotel       = "hotel"
)

// GroupOrganic is the taxonomy group whose submissions must carry a source type.
const GroupOrganic = "organic"

// GroupForMaterial returns the taxonomy group for a material key, or "".
func GroupForMaterial(key string) string {
	for _, m := range MaterialTaxonomy {
		if m.Key == key {
			return m.Group
		}
	}
	return ""
}

// MaterialRate maps to the material_rates table (admin-configurable, FR-9).
type MaterialRate struct {
	ID           int64  `json:"id"`
	MaterialType string `json:"material_type"`
	PointsPerKg  int64  `json:"points_per_kg"`
}

// PointsLedgerEntry maps to the points_ledger table — the source of truth for
// balances (docs/06 §2). Exactly one of SubmissionID / RedemptionID is set.
type PointsLedgerEntry struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	SubmissionID *int64    `json:"submission_id"`
	RedemptionID *int64    `json:"redemption_id"`
	PointsDelta  int64     `json:"points_delta"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// Partner maps to the partners table. Active is 0/1 in SQLite, bool in JSON.
type Partner struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url"`
	Active  bool    `json:"active"`
}

// Voucher maps to the vouchers table. StockRemaining nil means unlimited.
type Voucher struct {
	ID             int64   `json:"id"`
	PartnerID      int64   `json:"partner_id"`
	Title          string  `json:"title"`
	PointsCost     int64   `json:"points_cost"`
	DiscountType   string  `json:"discount_type"`
	DiscountValue  float64 `json:"discount_value"`
	ExpiryDays     int64   `json:"expiry_days"`
	StockRemaining *int64  `json:"stock_remaining"`
	Active         bool    `json:"active"`
}

// Redemption maps to the redemptions table.
type Redemption struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	VoucherID      int64      `json:"voucher_id"`
	RedemptionCode string     `json:"redemption_code"`
	Status         string     `json:"status"`
	IssuedAt       time.Time  `json:"issued_at"`
	UsedAt         *time.Time `json:"used_at"`
	VerifiedBy     *int64     `json:"verified_by"`
}
