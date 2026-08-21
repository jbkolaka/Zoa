package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/httpx"
	"zoa/backend/internal/models"
)

// HealthHandler serves the liveness/readiness endpoint that Phase 0 delivers.
type HealthHandler struct {
	db      *sql.DB
	env     string
	version string
	started time.Time
}

// NewHealthHandler builds the health handler.
func NewHealthHandler(db *sql.DB, env, version string, started time.Time) *HealthHandler {
	return &HealthHandler{db: db, env: env, version: version, started: started}
}

type healthResponse struct {
	Status        string       `json:"status"`
	Service       string       `json:"service"`
	Version       string       `json:"version"`
	Env           string       `json:"env"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	Time          time.Time    `json:"time"`
	Database      databaseInfo `json:"database"`
}

type databaseInfo struct {
	Connected         bool   `json:"connected"`
	MigrationsApplied int    `json:"migrations_applied"`
	SchemaVersion     string `json:"schema_version"`
}

// Health handles GET /health.
//
// It reports 200 only when the database actually answers a query — a process
// that is up but cannot reach its DB is not healthy, and the Flutter splash
// screen needs to distinguish those two cases.
func (h *HealthHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	info, err := h.databaseInfo(ctx)
	if err != nil {
		httpx.Fail(c, http.StatusServiceUnavailable, httpx.CodeUnavailable,
			"database unreachable: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, healthResponse{
		Status:        "ok",
		Service:       "zoa-api",
		Version:       h.version,
		Env:           h.env,
		UptimeSeconds: int64(time.Since(h.started).Seconds()),
		Time:          time.Now().UTC(),
		Database:      info,
	})
}

func (h *HealthHandler) databaseInfo(ctx context.Context) (databaseInfo, error) {
	var count int
	var latest sql.NullString

	err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*), MAX(version) FROM schema_migrations`).Scan(&count, &latest)
	if err != nil {
		return databaseInfo{}, err
	}

	return databaseInfo{
		Connected:         true,
		MigrationsApplied: count,
		SchemaVersion:     latest.String,
	}, nil
}

// Meta handles GET /meta — the material taxonomy plus its configured points
// rates, so the Flutter material selector is driven by the server rather than a
// hardcoded client list that could drift from material_rates (FR-9).
func (h *HealthHandler) Meta(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	rates, err := h.materialRates(ctx)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not load material rates")
		return
	}

	type materialEntry struct {
		models.MaterialType
		PointsPerKg int64 `json:"points_per_kg"`
	}

	materials := make([]materialEntry, 0, len(models.MaterialTaxonomy))
	for _, m := range models.MaterialTaxonomy {
		materials = append(materials, materialEntry{MaterialType: m, PointsPerKg: rates[m.Key]})
	}

	c.JSON(http.StatusOK, gin.H{
		"materials": materials,
		"statuses": gin.H{
			"submission": []string{
				models.SubmissionPending, models.SubmissionCollected,
				models.SubmissionVerified, models.SubmissionRejected,
			},
			"redemption": []string{
				models.RedemptionIssued, models.RedemptionUsed, models.RedemptionExpired,
			},
		},
	})
}

func (h *HealthHandler) materialRates(ctx context.Context) (map[string]int64, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT material_type, points_per_kg FROM material_rates`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rates := make(map[string]int64)
	for rows.Next() {
		var key string
		var pts int64
		if err := rows.Scan(&key, &pts); err != nil {
			return nil, err
		}
		rates[key] = pts
	}
	return rates, rows.Err()
}
