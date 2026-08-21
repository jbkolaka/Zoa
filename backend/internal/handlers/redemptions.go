package handlers

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/httpx"
	"zoa/backend/internal/middleware"
	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// RedemptionHandler serves the redemption lifecycle (Phase 4): spending points
// for a code, the user's own history, and the partner-side code check.
type RedemptionHandler struct {
	redemptions *store.RedemptionStore
}

// NewRedemptionHandler builds the redemption handler.
func NewRedemptionHandler(redemptions *store.RedemptionStore) *RedemptionHandler {
	return &RedemptionHandler{redemptions: redemptions}
}

// --- response shapes (docs/API_CONTRACT.md § Phase 4) ---

// voucherSnapshot is the voucher a redemption was bought from.
//
// Deliberately not voucherResponse: that carries `affordable`, computed against
// the caller's balance. Here it would be meaningless in a history listing and
// actively wrong in a verify response, where the caller is the cashier — an
// affordability verdict about the wrong person's points.
type voucherSnapshot struct {
	ID             int64      `json:"id"`
	PartnerID      int64      `json:"partner_id"`
	Title          string     `json:"title"`
	PointsCost     int64      `json:"points_cost"`
	DiscountType   string     `json:"discount_type"`
	DiscountValue  float64    `json:"discount_value"`
	ExpiryDays     int64      `json:"expiry_days"`
	StockRemaining *int64     `json:"stock_remaining"`
	Active         bool       `json:"active"`
	Partner        partnerRef `json:"partner"`
}

func newVoucherSnapshot(v store.VoucherWithPartner) voucherSnapshot {
	return voucherSnapshot{
		ID:             v.ID,
		PartnerID:      v.PartnerID,
		Title:          v.Title,
		PointsCost:     v.PointsCost,
		DiscountType:   v.DiscountType,
		DiscountValue:  v.DiscountValue,
		ExpiryDays:     v.ExpiryDays,
		StockRemaining: v.StockRemaining,
		Active:         v.Active,
		Partner: partnerRef{
			ID:      v.Partner.ID,
			Name:    v.Partner.Name,
			LogoURL: v.Partner.LogoURL,
			Active:  v.Partner.Active,
		},
	}
}

// redemptionResponse mirrors the redemptions table, plus the two values the
// client cannot compute for itself: the expiry and the QR payload.
type redemptionResponse struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	VoucherID      int64      `json:"voucher_id"`
	RedemptionCode string     `json:"redemption_code"`
	Status         string     `json:"status"`
	IssuedAt       time.Time  `json:"issued_at"`
	UsedAt         *time.Time `json:"used_at"`
	VerifiedBy     *int64     `json:"verified_by"`

	// Expiry is derived (issued_at + voucher.expiry_days), not stored — see
	// store.RedemptionWithContext.Expiry.
	Expiry    time.Time `json:"expiry"`
	QRPayload string    `json:"qr_payload"`

	// Voucher travels inside each item of a history listing, so "My Redemptions"
	// renders in one call. Omitted where the voucher is already a sibling key — on
	// a create or verify response — rather than serialising the same object twice.
	Voucher *voucherSnapshot `json:"voucher,omitempty"`
}

func newRedemptionResponse(r *store.RedemptionWithContext) redemptionResponse {
	return redemptionResponse{
		ID:             r.ID,
		UserID:         r.UserID,
		VoucherID:      r.VoucherID,
		RedemptionCode: r.RedemptionCode,
		Status:         r.Status,
		IssuedAt:       r.IssuedAt,
		UsedAt:         r.UsedAt,
		VerifiedBy:     r.VerifiedBy,
		Expiry:         r.Expiry(),
		QRPayload:      r.QRPayload(),
	}
}

// newRedemptionListItem is the same shape with the voucher embedded.
func newRedemptionListItem(r *store.RedemptionWithContext) redemptionResponse {
	item := newRedemptionResponse(r)
	snapshot := newVoucherSnapshot(r.Voucher)
	item.Voucher = &snapshot
	return item
}

type createRedemptionRequest struct {
	// A pointer so a missing field is distinguishable from 0. A plain int64 would
	// read an absent voucher_id as id 0 and answer "no such reward" for a request
	// that never named one.
	VoucherID *int64 `json:"voucher_id"`
}

// Create handles POST /redemptions — the points-spending step.
func (h *RedemptionHandler) Create(c *gin.Context) {
	user := middleware.MustCurrentUser(c)

	var req createRedemptionRequest
	if !bindJSON(c, &req) {
		return
	}

	if req.VoucherID == nil || *req.VoucherID <= 0 {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please choose a reward to redeem",
			map[string]string{"voucher_id": "must be a positive whole number"})
		return
	}

	result, err := h.redemptions.Redeem(c.Request.Context(), user.ID, *req.VoucherID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "no such reward")
		return
	case errors.Is(err, store.ErrInsufficientPoints):
		// The contract requires `insufficient_points` in the message, so a client can
		// tell this apart from the out-of-stock 409 on the same endpoint. Nothing was
		// deducted — the guard refuses before any write lands.
		httpx.Fail(c, http.StatusConflict, httpx.CodeConflict,
			"insufficient_points — you do not have enough points for this reward yet")
		return
	case errors.Is(err, store.ErrOutOfStock):
		httpx.Fail(c, http.StatusConflict, httpx.CodeConflict,
			"this reward has just run out — your points were not spent")
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not redeem that reward")
		return
	}

	redemption := result.Redemption

	c.JSON(http.StatusCreated, gin.H{
		// code, qr_payload and expiry are the exact keys App Flow §1 specifies for
		// this response; everything below them is additive.
		"code":       redemption.RedemptionCode,
		"qr_payload": redemption.QRPayload(),
		"expiry":     redemption.Expiry(),

		"redemption":     newRedemptionResponse(redemption),
		"voucher":        newVoucherSnapshot(redemption.Voucher),
		"points_spent":   result.PointsSpent,
		"points_balance": result.PointsBalance,
	})
}

// List handles GET /redemptions — the caller's own history, newest first.
func (h *RedemptionHandler) List(c *gin.Context) {
	user := middleware.MustCurrentUser(c)

	// Own history only, and not widenable by query parameter — unlike
	// /submissions, which a collector may read across users. A redemption code is
	// bearer-like: anyone who can read it can spend it, so no role gets a listing
	// of someone else's.
	redemptions, err := h.redemptions.ListForUser(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load your codes")
		return
	}

	// Built with make so an empty history serialises as [] rather than null — the
	// client iterates this without a null check.
	items := make([]redemptionResponse, 0, len(redemptions))
	for _, redemption := range redemptions {
		items = append(items, newRedemptionListItem(redemption))
	}

	c.JSON(http.StatusOK, gin.H{"redemptions": items})
}

// Verify handles POST /redemptions/:code/verify — the partner-side check.
//
// Idempotent by design (FR-16): the store guards the transition inside its
// transaction, so a double-tap produces one success and one 409 rather than
// marking a code used twice.
func (h *RedemptionHandler) Verify(c *gin.Context) {
	staff := middleware.MustCurrentUser(c)

	// Trimmed because this is typed or pasted by a person at a till, and a trailing
	// space from a paste should not read as an unknown code.
	code := strings.TrimSpace(c.Param("code"))
	if code == "" {
		h.failUnknownCode(c)
		return
	}

	redemption, err := h.redemptions.Verify(c.Request.Context(), code, staff.ID)

	var notRedeemable *store.CodeNotRedeemableError
	switch {
	case errors.Is(err, store.ErrNotFound):
		h.failUnknownCode(c)
		return
	case errors.As(err, &notRedeemable):
		httpx.Fail(c, http.StatusConflict, httpx.CodeConflict, refusalMessage(notRedeemable))
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not check that code")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     redemption.Status,
		"redemption": newRedemptionResponse(redemption),
		"voucher":    newVoucherSnapshot(redemption.Voucher),
		// submitterRef is reused rather than redefined: {id, name} is exactly the
		// shape the contract specifies here, and a cashier needs a person's name and
		// nothing else about them.
		"user":    submitterRef{ID: redemption.UserID, Name: redemption.UserName},
		"message": acceptedMessage(redemption.Voucher),
	})
}

func (h *RedemptionHandler) failUnknownCode(c *gin.Context) {
	httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound,
		"that code is not one of ours — check it and try again")
}

// refusalMessage turns a refusal into something a cashier can act on.
//
// Both branches end in an instruction, not just a diagnosis: the person reading
// this is mid-transaction with a customer waiting, and "do not apply the
// discount" is the part that matters.
func refusalMessage(err *store.CodeNotRedeemableError) string {
	if err.Status == models.RedemptionExpired {
		return "this code expired on " + err.ExpiredAt.Format("2 Jan 2006") +
			" — do not apply the discount"
	}
	if err.UsedAt != nil {
		return "code already used on " + err.UsedAt.Format("2 Jan 2006 at 15:04") +
			" — do not apply the discount"
	}
	return "code already used — do not apply the discount"
}

// acceptedMessage tells the cashier exactly what to do at the till.
func acceptedMessage(v store.VoucherWithPartner) string {
	return "Code accepted — apply " + discountLabel(v) + " at checkout."
}

// discountLabel renders a discount as a short phrase: `10% off`, `KSh 100 off`.
// Mirrors the client's Voucher.discountLabel so both halves of the product phrase
// a discount identically.
func discountLabel(v store.VoucherWithPartner) string {
	if v.DiscountType == models.DiscountPercentage {
		return trimZeros(v.DiscountValue) + "% off"
	}
	return "KSh " + trimZeros(v.DiscountValue) + " off"
}

// trimZeros drops a redundant decimal so a whole-number discount reads
// "KSh 100 off" rather than "KSh 100.0 off".
func trimZeros(value float64) string {
	if value == math.Trunc(value) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
