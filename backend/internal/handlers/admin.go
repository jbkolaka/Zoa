package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/httpx"
	"zoa/backend/internal/store"
)

// AdminHandler serves the platform overview (Phase 5).
type AdminHandler struct {
	admin *store.AdminStore
}

// NewAdminHandler builds the admin handler.
func NewAdminHandler(admin *store.AdminStore) *AdminHandler {
	return &AdminHandler{admin: admin}
}

// Stats handles GET /admin/stats.
//
// Serialised as gin.H rather than through a stack of response structs: the payload
// is five nested read-only aggregates used by exactly one caller, and five extra
// types would restate the store's shape without adding a decision. The handler
// still owns the wire format, which is the convention that matters.
func (h *AdminHandler) Stats(c *gin.Context) {
	stats, err := h.admin.Stats(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load platform statistics")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": gin.H{
			"total":   stats.Users.Total,
			"by_role": stats.Users.ByRole,
		},
		"submissions": gin.H{
			"total":             stats.Submissions.Total,
			"by_status":         stats.Submissions.ByStatus,
			"total_verified_kg": stats.Submissions.TotalVerifiedKg,
		},
		"points": gin.H{
			"total_issued": stats.Points.TotalIssued,
			"total_spent":  stats.Points.TotalSpent,
			"outstanding":  stats.Points.Outstanding,
		},
		"redemptions": gin.H{
			"total":     stats.Redemptions.Total,
			"by_status": stats.Redemptions.ByStatus,
		},
		"classification": gin.H{
			"predictions_made": stats.Classification.PredictionsMade,
			"verified_against": stats.Classification.VerifiedAgainst,
			"correct":          stats.Classification.Correct,
			// null, not 0, until something has been verified — see
			// store.ClassificationStats.Accuracy. A client must render the empty case
			// as "no data yet" rather than as a score.
			"accuracy": stats.Classification.Accuracy,
		},
	})
}
