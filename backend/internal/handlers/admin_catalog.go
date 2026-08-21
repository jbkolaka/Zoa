package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/httpx"
	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// maxTitleLen bounds free text so a paste accident cannot fill a card with a novel.
const maxTitleLen = 120

// Optional distinguishes "absent from the body" from "present and null".
//
// A plain *T cannot: both arrive as nil, so `{"stock_remaining": null}` — make this
// voucher unlimited — is indistinguishable from omitting the field, which means
// leave stock alone. Both are real PATCH requests, so the difference has to survive
// decoding.
type Optional[T any] struct {
	// Set is true when the key appeared in the body at all.
	Set bool

	// Value is nil when the key appeared with an explicit null.
	Value *T
}

// UnmarshalJSON records that the key was present, then decodes it.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true

	if string(data) == "null" {
		o.Value = nil
		return nil
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

// CatalogAdminHandler serves partner and voucher administration (Phase 5).
type CatalogAdminHandler struct {
	catalog *store.CatalogStore
}

// NewCatalogAdminHandler builds the catalog admin handler.
func NewCatalogAdminHandler(catalog *store.CatalogStore) *CatalogAdminHandler {
	return &CatalogAdminHandler{catalog: catalog}
}

// --- partners ---

type partnerRequest struct {
	Name    *string          `json:"name"`
	LogoURL Optional[string] `json:"logo_url"`
	Active  *bool            `json:"active"`
}

// ListPartners handles GET /admin/partners.
func (h *CatalogAdminHandler) ListPartners(c *gin.Context) {
	partners, err := h.catalog.ListPartners(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load partners")
		return
	}

	// Inactive partners included, unlike GET /partners — the admin view exists to
	// show what the catalogue is hiding.
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

// GetPartner handles GET /admin/partners/:id.
func (h *CatalogAdminHandler) GetPartner(c *gin.Context) {
	id, ok := parseAdminID(c, "partner")
	if !ok {
		return
	}

	partner, err := h.catalog.PartnerByID(c.Request.Context(), id)
	if h.failed(c, err, "partner") {
		return
	}
	c.JSON(http.StatusOK, newPartnerRef(partner))
}

// CreatePartner handles POST /admin/partners.
func (h *CatalogAdminHandler) CreatePartner(c *gin.Context) {
	var req partnerRequest
	if !bindJSON(c, &req) {
		return
	}

	name := trimmedOrNil(req.Name, maxTitleLen)
	if name == nil {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields",
			map[string]string{"name": "give the partner a name"})
		return
	}

	partner, err := h.catalog.CreatePartner(c.Request.Context(), store.PartnerPatch{
		Name:       name,
		LogoURL:    req.LogoURL.Value,
		LogoURLSet: req.LogoURL.Set,
		Active:     req.Active,
	})
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not create that partner")
		return
	}
	c.JSON(http.StatusCreated, newPartnerRef(partner))
}

// UpdatePartner handles PATCH /admin/partners/:id.
func (h *CatalogAdminHandler) UpdatePartner(c *gin.Context) {
	id, ok := parseAdminID(c, "partner")
	if !ok {
		return
	}

	var req partnerRequest
	if !bindJSON(c, &req) {
		return
	}

	patch := store.PartnerPatch{
		LogoURL:    req.LogoURL.Value,
		LogoURLSet: req.LogoURL.Set,
		Active:     req.Active,
	}

	// A name may be changed but not blanked — every screen renders it, and an empty
	// partner name would leave a voucher attributed to nobody.
	if req.Name != nil {
		name := trimmedOrNil(req.Name, maxTitleLen)
		if name == nil {
			httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
				"please correct the highlighted fields",
				map[string]string{"name": "a partner name cannot be empty"})
			return
		}
		patch.Name = name
	}

	partner, err := h.catalog.UpdatePartner(c.Request.Context(), id, patch)
	if h.failed(c, err, "partner") {
		return
	}
	c.JSON(http.StatusOK, newPartnerRef(partner))
}

// DeletePartner handles DELETE /admin/partners/:id — a soft delete.
func (h *CatalogAdminHandler) DeletePartner(c *gin.Context) {
	id, ok := parseAdminID(c, "partner")
	if !ok {
		return
	}

	partner, err := h.catalog.DeactivatePartner(c.Request.Context(), id)
	if h.failed(c, err, "partner") {
		return
	}

	// 200 with the row rather than 204: this is a deactivation, and returning the
	// record makes it visible that it still exists and can be switched back on.
	c.JSON(http.StatusOK, gin.H{
		"partner": newPartnerRef(partner),
		"message": "Partner deactivated. Its vouchers are hidden from the catalogue " +
			"but existing codes still work.",
	})
}

func newPartnerRef(partner *models.Partner) partnerRef {
	return partnerRef{
		ID:      partner.ID,
		Name:    partner.Name,
		LogoURL: partner.LogoURL,
		Active:  partner.Active,
	}
}

// --- vouchers ---

type voucherRequest struct {
	PartnerID      *int64          `json:"partner_id"`
	Title          *string         `json:"title"`
	PointsCost     *int64          `json:"points_cost"`
	DiscountType   *string         `json:"discount_type"`
	DiscountValue  *float64        `json:"discount_value"`
	ExpiryDays     *int64          `json:"expiry_days"`
	StockRemaining Optional[int64] `json:"stock_remaining"`
	Active         *bool           `json:"active"`
}

// ListVouchers handles GET /admin/vouchers.
func (h *CatalogAdminHandler) ListVouchers(c *gin.Context) {
	vouchers, err := h.catalog.ListVouchersForAdmin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load vouchers")
		return
	}

	// voucherSnapshot rather than voucherResponse: `affordable` is a fact about a
	// particular user's balance and means nothing in an admin listing.
	items := make([]voucherSnapshot, 0, len(vouchers))
	for _, voucher := range vouchers {
		items = append(items, newVoucherSnapshot(*voucher))
	}
	c.JSON(http.StatusOK, gin.H{"vouchers": items})
}

// GetVoucher handles GET /admin/vouchers/:id.
func (h *CatalogAdminHandler) GetVoucher(c *gin.Context) {
	id, ok := parseAdminID(c, "voucher")
	if !ok {
		return
	}

	voucher, err := h.catalog.VoucherByIDForAdmin(c.Request.Context(), id)
	if h.failed(c, err, "voucher") {
		return
	}
	c.JSON(http.StatusOK, newVoucherSnapshot(*voucher))
}

// CreateVoucher handles POST /admin/vouchers.
func (h *CatalogAdminHandler) CreateVoucher(c *gin.Context) {
	var req voucherRequest
	if !bindJSON(c, &req) {
		return
	}

	fields := map[string]string{}

	if req.PartnerID == nil || *req.PartnerID <= 0 {
		fields["partner_id"] = "choose which partner this belongs to"
	}
	title := trimmedOrNil(req.Title, maxTitleLen)
	if title == nil {
		fields["title"] = "give the reward a title"
	}
	if req.PointsCost == nil {
		fields["points_cost"] = "set what this costs in points"
	}
	if req.DiscountType == nil {
		fields["discount_type"] = "must be percentage or fixed"
	}
	if req.DiscountValue == nil {
		fields["discount_value"] = "set the discount amount"
	}
	if req.ExpiryDays == nil {
		fields["expiry_days"] = "set how long a code stays valid"
	}

	// Range checks run on whatever was supplied, so one round trip reports every
	// problem rather than revealing them one at a time.
	mergeVoucherRangeErrors(fields, req)

	if len(fields) > 0 {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields", fields)
		return
	}

	voucher, err := h.catalog.CreateVoucher(c.Request.Context(), store.VoucherPatch{
		PartnerID:         req.PartnerID,
		Title:             title,
		PointsCost:        req.PointsCost,
		DiscountType:      normalisedDiscountType(req.DiscountType),
		DiscountValue:     req.DiscountValue,
		ExpiryDays:        req.ExpiryDays,
		StockRemaining:    req.StockRemaining.Value,
		StockRemainingSet: req.StockRemaining.Set,
		Active:            req.Active,
	})
	if errors.Is(err, store.ErrPartnerMissing) {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields",
			map[string]string{"partner_id": "no partner with that id"})
		return
	}
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not create that reward")
		return
	}
	c.JSON(http.StatusCreated, newVoucherSnapshot(*voucher))
}

// UpdateVoucher handles PATCH /admin/vouchers/:id.
func (h *CatalogAdminHandler) UpdateVoucher(c *gin.Context) {
	id, ok := parseAdminID(c, "voucher")
	if !ok {
		return
	}

	var req voucherRequest
	if !bindJSON(c, &req) {
		return
	}

	fields := map[string]string{}
	mergeVoucherRangeErrors(fields, req)

	var title *string
	if req.Title != nil {
		title = trimmedOrNil(req.Title, maxTitleLen)
		if title == nil {
			fields["title"] = "a title cannot be empty"
		}
	}

	if len(fields) > 0 {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields", fields)
		return
	}

	voucher, err := h.catalog.UpdateVoucher(c.Request.Context(), id, store.VoucherPatch{
		PartnerID:         req.PartnerID,
		Title:             title,
		PointsCost:        req.PointsCost,
		DiscountType:      normalisedDiscountType(req.DiscountType),
		DiscountValue:     req.DiscountValue,
		ExpiryDays:        req.ExpiryDays,
		StockRemaining:    req.StockRemaining.Value,
		StockRemainingSet: req.StockRemaining.Set,
		Active:            req.Active,
	})
	switch {
	case errors.Is(err, store.ErrPartnerMissing):
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields",
			map[string]string{"partner_id": "no partner with that id"})
		return
	case errors.Is(err, store.ErrNothingToUpdate):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeValidation,
			"send at least one field to change")
		return
	}
	if h.failed(c, err, "voucher") {
		return
	}
	c.JSON(http.StatusOK, newVoucherSnapshot(*voucher))
}

// DeleteVoucher handles DELETE /admin/vouchers/:id — a soft delete.
func (h *CatalogAdminHandler) DeleteVoucher(c *gin.Context) {
	id, ok := parseAdminID(c, "voucher")
	if !ok {
		return
	}

	voucher, err := h.catalog.DeactivateVoucher(c.Request.Context(), id)
	if h.failed(c, err, "voucher") {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"voucher": newVoucherSnapshot(*voucher),
		"message": "Reward withdrawn from the catalogue. Codes already issued for it " +
			"remain valid.",
	})
}

// mergeVoucherRangeErrors validates the numeric and enum fields that were supplied.
func mergeVoucherRangeErrors(fields map[string]string, req voucherRequest) {
	if req.PointsCost != nil && *req.PointsCost <= 0 {
		fields["points_cost"] = "must be greater than 0"
	}
	if req.ExpiryDays != nil && *req.ExpiryDays <= 0 {
		fields["expiry_days"] = "must be at least 1 day"
	}
	if req.DiscountValue != nil && *req.DiscountValue <= 0 {
		fields["discount_value"] = "must be greater than 0"
	}

	if req.DiscountType != nil {
		switch strings.ToLower(strings.TrimSpace(*req.DiscountType)) {
		case models.DiscountPercentage:
			// A percentage over 100 would be a free basket plus change, which is far
			// more likely a typo than an offer.
			if req.DiscountValue != nil && *req.DiscountValue > 100 {
				fields["discount_value"] = "a percentage cannot exceed 100"
			}
		case models.DiscountFixed:
		default:
			fields["discount_type"] = "must be percentage or fixed"
		}
	}

	// Negative stock is not "sold out", it is a bad write. NULL is unlimited and 0 is
	// sold out, both legitimate.
	if req.StockRemaining.Set && req.StockRemaining.Value != nil && *req.StockRemaining.Value < 0 {
		fields["stock_remaining"] = "cannot be negative — use null for unlimited"
	}
}

// normalisedDiscountType lower-cases a supplied discount type so the CHECK
// constraint sees the canonical value.
func normalisedDiscountType(raw *string) *string {
	if raw == nil {
		return nil
	}
	value := strings.ToLower(strings.TrimSpace(*raw))
	return &value
}

// failed maps a store error onto a response. Returns true when it handled one.
func (h *CatalogAdminHandler) failed(c *gin.Context, err error, noun string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound):
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "no such "+noun)
	case errors.Is(err, store.ErrNothingToUpdate):
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeValidation,
			"send at least one field to change")
	default:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not complete that change")
	}
	return true
}

// parseAdminID reads a positive integer :id, naming the resource in the 404 so an
// admin tool reports which lookup failed.
func parseAdminID(c *gin.Context, noun string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "no such "+noun)
		return 0, false
	}
	return id, true
}
