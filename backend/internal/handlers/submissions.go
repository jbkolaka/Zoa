package handlers

import (
	"errors"
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

// maxQtyKg is a sanity ceiling on a single submission. A hotel kitchen's daily
// food waste is realistically in the tens of kilograms, so anything past this is
// far more likely a misplaced decimal point (450 for 4.5) than a real load.
const maxQtyKg = 5000

// SubmissionHandler serves the submission lifecycle.
type SubmissionHandler struct {
	submissions *store.SubmissionStore
}

// NewSubmissionHandler builds the submission handler.
func NewSubmissionHandler(submissions *store.SubmissionStore) *SubmissionHandler {
	return &SubmissionHandler{submissions: submissions}
}

// --- response shapes (docs/API_CONTRACT.md § Phase 2) ---

// submissionResponse mirrors the submissions table, plus two joined extras:
// `user` so the collector queue can name a person, and `points_awarded` so the
// status screen can show what a verification earned.
type submissionResponse struct {
	ID             int64        `json:"id"`
	UserID         int64        `json:"user_id"`
	MaterialType   string       `json:"material_type"`
	EstimatedQtyKg *float64     `json:"estimated_qty_kg"`
	VerifiedQtyKg  *float64     `json:"verified_qty_kg"`
	Location       *string      `json:"location"`
	Status         string       `json:"status"`
	CollectorID    *int64       `json:"collector_id"`
	CreatedAt      time.Time    `json:"created_at"`
	VerifiedAt     *time.Time   `json:"verified_at"`
	User           submitterRef `json:"user"`
	PointsAwarded  *int64       `json:"points_awarded"`

	// --- Phase 2.5 ---
	// Surfaced so the collector's verify sheet can show what the AI guessed and
	// flag its own disagreement, which is the cross-check the feature exists for.
	PredictedCategory   *string  `json:"predicted_category"`
	PredictedConfidence *float64 `json:"predicted_confidence"`
	SourceType          *string  `json:"source_type"`
}

// submitterRef is a minimal user reference. Name only — a collector needs someone
// to ask for, but no other user's phone number needs to travel with a listing.
type submitterRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func newSubmissionResponse(s *store.SubmissionWithContext) submissionResponse {
	return submissionResponse{
		ID:             s.ID,
		UserID:         s.UserID,
		MaterialType:   s.MaterialType,
		EstimatedQtyKg: s.EstimatedQtyKg,
		VerifiedQtyKg:  s.VerifiedQtyKg,
		Location:       s.Location,
		Status:         s.Status,
		CollectorID:    s.CollectorID,
		CreatedAt:      s.CreatedAt,
		VerifiedAt:     s.VerifiedAt,
		User:           submitterRef{ID: s.UserID, Name: s.UserName},
		PointsAwarded:  s.PointsAwarded,

		PredictedCategory:   s.PredictedCategory,
		PredictedConfidence: s.PredictedConfidence,
		SourceType:          s.SourceType,
	}
}

type createSubmissionRequest struct {
	MaterialType   string   `json:"material_type"`
	EstimatedQtyKg *float64 `json:"estimated_qty_kg"`
	Location       *string  `json:"location"`

	// --- Phase 2.5 (docs/API_CONTRACT.md § Phase 2.5), all optional ---

	// PredictedCategory / PredictedConfidence are echoed back by the client from
	// a prior /submissions/classify call. Advisory: they are recorded for the
	// accuracy metric (FR-22) and never override MaterialType, which is what the
	// user actually confirmed.
	PredictedCategory   *string  `json:"predicted_category"`
	PredictedConfidence *float64 `json:"predicted_confidence"`

	// SourceType is required when MaterialType is organic (FR-24).
	SourceType *string `json:"source_type"`
}

type verifySubmissionRequest struct {
	Status        string   `json:"status"`
	VerifiedQtyKg *float64 `json:"verified_qty_kg"`
	MaterialType  string   `json:"material_type"`
}

// Create handles POST /submissions.
func (h *SubmissionHandler) Create(c *gin.Context) {
	user := middleware.MustCurrentUser(c)

	var req createSubmissionRequest
	if !bindJSON(c, &req) {
		return
	}

	fields := map[string]string{}

	if req.MaterialType == "" {
		fields["material_type"] = "choose a material type"
	} else if !models.IsValidMaterialType(req.MaterialType) {
		fields["material_type"] = "not a recognised material type"
	}

	// Distinguishing "absent" from "zero" is why this is a pointer: a plain
	// float64 would read a missing field as 0 and report the wrong message.
	switch {
	case req.EstimatedQtyKg == nil:
		fields["estimated_qty_kg"] = "enter an estimated weight"
	case *req.EstimatedQtyKg <= 0:
		fields["estimated_qty_kg"] = "weight must be greater than 0"
	case *req.EstimatedQtyKg > maxQtyKg:
		fields["estimated_qty_kg"] = "weight looks too large — check the decimal point"
	}

	location := trimmedOrNil(req.Location, 200)

	// --- Phase 2.5 fields ---

	// The prediction is recorded, not trusted: an unrecognised category is
	// dropped rather than rejected, because a stale client guess must never block
	// a submission the user has already confirmed manually (FR-23).
	var predictedCategory *string
	var predictedConfidence *float64
	if req.PredictedCategory != nil {
		if key := strings.TrimSpace(*req.PredictedCategory); models.IsValidMaterialType(key) {
			predictedCategory = &key
			if req.PredictedConfidence != nil {
				conf := clamp01(*req.PredictedConfidence)
				predictedConfidence = &conf
			}
		}
	}

	// Source type: mandatory for organics, rejected outright when malformed —
	// unlike the prediction, this is a real user answer and a wrong value would
	// silently misroute collection.
	sourceType, sourceErr := resolveSourceType(req.SourceType, req.MaterialType)
	if sourceErr != "" {
		fields["source_type"] = sourceErr
	}

	if len(fields) > 0 {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields", fields)
		return
	}

	submission, err := h.submissions.Create(c.Request.Context(), store.CreateInput{
		UserID:              user.ID,
		MaterialType:        req.MaterialType,
		EstimatedQtyKg:      *req.EstimatedQtyKg,
		Location:            location,
		PredictedCategory:   predictedCategory,
		PredictedConfidence: predictedConfidence,
		SourceType:          sourceType,
	})
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not save your submission")
		return
	}

	c.JSON(http.StatusCreated, newSubmissionResponse(submission))
}

// resolveSourceType validates the source type against the material's group.
//
// Returns the value to store and a field-level error message ("" when valid).
// Organic waste must declare hotel-vs-residential (FR-24) because the two differ
// in volume and routing; other groups may omit it, and a stray value there is
// dropped rather than stored, so the column means one thing only.
func resolveSourceType(raw *string, materialType string) (*string, string) {
	organic := models.GroupForMaterial(materialType) == models.GroupOrganic

	var value string
	if raw != nil {
		value = strings.ToLower(strings.TrimSpace(*raw))
	}

	if value == "" {
		if organic {
			return nil, "tell us whether this came from a home or a hotel"
		}
		return nil, ""
	}

	if value != models.SourceResidential && value != models.SourceHotel {
		return nil, "choose either residential or hotel"
	}
	if !organic {
		// Not an error: a client that asks every time is harmless, but the column
		// stays meaningful only if non-organic rows leave it NULL.
		return nil, ""
	}
	return &value, ""
}

// clamp01 forces a confidence into [0,1] so a malformed client value cannot be
// stored as "140% confident".
func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

// Get handles GET /submissions/:id.
func (h *SubmissionHandler) Get(c *gin.Context) {
	user := middleware.MustCurrentUser(c)

	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	submission, err := h.submissions.ByID(c.Request.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		h.failNotFound(c)
		return
	}
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load that submission")
		return
	}

	// Someone else's submission answers exactly as a missing one does, so ids
	// cannot be probed to learn what exists (contract: ownership scoping is
	// silent, 404 rather than 403).
	if submission.UserID != user.ID && !isCollector(user) {
		h.failNotFound(c)
		return
	}

	c.JSON(http.StatusOK, newSubmissionResponse(submission))
}

// List handles GET /submissions.
//
// A plain user sees only their own submissions regardless of what they ask for;
// a collector or admin sees everyone's, which is what makes `?status=pending`
// the collector queue.
func (h *SubmissionHandler) List(c *gin.Context) {
	user := middleware.MustCurrentUser(c)

	filter := store.ListFilter{
		Status: c.Query("status"),
		Limit:  atoiOr(c.Query("limit"), 50),
		Offset: atoiOr(c.Query("offset"), 0),
	}

	if !isCollector(user) {
		// Not a filter the caller can widen — set after reading the query so a
		// user_id parameter cannot override it.
		filter.UserID = user.ID
	} else if mine := c.Query("user_id"); mine != "" {
		filter.UserID = int64(atoiOr(mine, 0))
	}

	if filter.Status != "" && !isValidSubmissionStatus(filter.Status) {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"unknown status filter",
			map[string]string{"status": "must be pending, collected, verified or rejected"})
		return
	}

	submissions, total, err := h.submissions.List(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load submissions")
		return
	}

	// Serialise an empty result as [] rather than null — a client iterating the
	// field should not have to special-case a missing list.
	items := make([]submissionResponse, 0, len(submissions))
	for _, submission := range submissions {
		items = append(items, newSubmissionResponse(submission))
	}

	c.JSON(http.StatusOK, gin.H{"submissions": items, "total": total})
}

// Verify handles PATCH /submissions/:id/verify — the points-crediting step.
func (h *SubmissionHandler) Verify(c *gin.Context) {
	collector := middleware.MustCurrentUser(c)

	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	var req verifySubmissionRequest
	if !bindJSON(c, &req) {
		return
	}

	target := req.Status
	if target == "" {
		target = models.SubmissionVerified
	}

	fields := map[string]string{}

	switch target {
	case models.SubmissionVerified:
		switch {
		case req.VerifiedQtyKg == nil:
			fields["verified_qty_kg"] = "enter the weight you measured"
		case *req.VerifiedQtyKg <= 0:
			fields["verified_qty_kg"] = "weight must be greater than 0"
		case *req.VerifiedQtyKg > maxQtyKg:
			fields["verified_qty_kg"] = "weight looks too large — check the decimal point"
		}
	case models.SubmissionCollected, models.SubmissionRejected:
		// No weight needed: neither transition credits anything.
	default:
		fields["status"] = "must be verified, collected or rejected"
	}

	if req.MaterialType != "" && !models.IsValidMaterialType(req.MaterialType) {
		fields["material_type"] = "not a recognised material type"
	}

	if len(fields) > 0 {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields", fields)
		return
	}

	input := store.VerifyInput{
		TargetStatus: target,
		MaterialType: req.MaterialType,
		CollectorID:  collector.ID,
	}
	if req.VerifiedQtyKg != nil {
		input.VerifiedQtyKg = *req.VerifiedQtyKg
	}

	result, err := h.submissions.Verify(c.Request.Context(), id, input)
	switch {
	case errors.Is(err, store.ErrNotFound):
		h.failNotFound(c)
		return
	case errors.Is(err, store.ErrAlreadyFinalized):
		// The submission moved on between the client loading the queue and acting
		// on it — most often another collector got there first.
		httpx.Fail(c, http.StatusConflict, httpx.CodeConflict,
			"this submission has already been handled — reload the queue")
		return
	case errors.Is(err, store.ErrNoRate):
		httpx.Fail(c, http.StatusConflict, httpx.CodeConflict,
			"no points rate is configured for that material — an admin must add one")
		return
	case err != nil:
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not verify that submission")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"submission":     newSubmissionResponse(result.Submission),
		"points_awarded": result.PointsAwarded,
		"points_balance": result.PointsBalance,
		"rate_applied": gin.H{
			"material_type": result.MaterialType,
			"points_per_kg": result.RateApplied,
		},
	})
}

func (h *SubmissionHandler) failNotFound(c *gin.Context) {
	httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "no such submission")
}

// --- helpers ---

// isCollector reports whether the user may see and act on other people's
// submissions.
func isCollector(user *models.User) bool {
	return user.Role == models.RoleCollector || user.Role == models.RoleAdmin
}

func isValidSubmissionStatus(status string) bool {
	switch status {
	case models.SubmissionPending, models.SubmissionCollected,
		models.SubmissionVerified, models.SubmissionRejected:
		return true
	}
	return false
}

// parseIDParam reads a positive integer :id, failing the request if it is not one.
func parseIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "no such submission")
		return 0, false
	}
	return id, true
}

func atoiOr(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

// trimmedOrNil normalises an optional free-text field: whitespace-only becomes
// nil rather than an empty string, so "no location given" is one value in the
// database instead of two.
func trimmedOrNil(raw *string, maxLen int) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > maxLen {
		trimmed = trimmed[:maxLen]
	}
	return &trimmed
}
