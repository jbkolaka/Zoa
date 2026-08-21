package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/httpx"
	"zoa/backend/internal/middleware"
	"zoa/backend/internal/store"
)

// VoucherHandler serves the partner voucher catalogue (Phase 3).
type VoucherHandler struct {
	vouchers *store.VoucherStore
	users    *store.UserStore
}

// NewVoucherHandler builds the voucher handler.
func NewVoucherHandler(vouchers *store.VoucherStore, users *store.UserStore) *VoucherHandler {
	return &VoucherHandler{vouchers: vouchers, users: users}
}

// --- response shapes (docs/API_CONTRACT.md § Phase 3) ---

// voucherResponse is a voucher with its partner embedded and affordability
// resolved.
type voucherResponse struct {
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

	// Affordable is computed server-side against the caller's live balance.
	//
	// Deliberately not left to the client: Phase 4 deducts points against the
	// same comparison, and a client that decided affordability itself would
	// eventually disagree with the server about what is redeemable — showing an
	// enabled button that then fails.
	Affordable bool `json:"affordable"`
}

type partnerRef struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url"`
	Active  bool    `json:"active"`
}

func newVoucherResponse(v *store.VoucherWithPartner, balance int64) voucherResponse {
	return voucherResponse{
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
		Affordable: balance >= v.PointsCost,
	}
}

// List handles GET /vouchers.
func (h *VoucherHandler) List(c *gin.Context) {
	user := middleware.MustCurrentUser(c)

	// Read from the database rather than from the JWT: the token carries the
	// balance as it was at sign-in, and a user who just earned points would
	// otherwise see a stale catalogue until they signed out and back in.
	balance, err := h.balanceOf(c, user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load your points balance")
		return
	}

	filter := store.VoucherFilter{}

	if raw := c.Query("partner_id"); raw != "" {
		partnerID, convErr := strconv.ParseInt(raw, 10, 64)
		if convErr != nil || partnerID <= 0 {
			httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
				"that partner filter is not valid",
				map[string]string{"partner_id": "must be a positive whole number"})
			return
		}
		filter.PartnerID = partnerID
	}

	// Only `affordable=true` filters. Any other value is treated as "no filter"
	// rather than rejected, so a client sending `affordable=false` to mean "show
	// everything" gets what it meant.
	if c.Query("affordable") == "true" {
		filter.AffordableFor = &balance
	}

	vouchers, err := h.vouchers.List(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load the voucher catalogue")
		return
	}

	// Built with make so an empty catalogue serialises as [] rather than null —
	// the client iterates this without a null check.
	items := make([]voucherResponse, 0, len(vouchers))
	for _, voucher := range vouchers {
		items = append(items, newVoucherResponse(voucher, balance))
	}

	c.JSON(http.StatusOK, gin.H{
		"vouchers": items,
		// Returned alongside so the catalogue screen can show "you have N points"
		// without a second call to /me, and so affordability is explainable.
		"points_balance": balance,
	})
}

// Get handles GET /vouchers/:id.
func (h *VoucherHandler) Get(c *gin.Context) {
	user := middleware.MustCurrentUser(c)

	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	balance, err := h.balanceOf(c, user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load your points balance")
		return
	}

	voucher, err := h.vouchers.ByID(c.Request.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "no such voucher")
		return
	}
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load that voucher")
		return
	}

	c.JSON(http.StatusOK, newVoucherResponse(voucher, balance))
}

// Partners handles GET /partners — the catalogue's filter list.
func (h *VoucherHandler) Partners(c *gin.Context) {
	partners, err := h.vouchers.Partners(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load partners")
		return
	}

	items := make([]partnerRef, 0, len(partners))
	for _, partner := range partners {
		items = append(items, partnerRef{
			ID:      partner.ID,
			Name:    partner.Name,
			LogoURL: partner.LogoURL,
			Active:  partner.Active,
		})
	}

	c.JSON(http.StatusOK, gin.H{"partners": items})
}

// balanceOf reads the caller's current points balance.
func (h *VoucherHandler) balanceOf(c *gin.Context, userID int64) (int64, error) {
	current, err := h.users.ByID(c.Request.Context(), userID)
	if err != nil {
		return 0, err
	}
	return current.PointsBalance, nil
}
