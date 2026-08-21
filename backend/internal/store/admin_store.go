package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"zoa/backend/internal/models"
)

// AdminStore serves the platform-wide read models behind /admin (Phase 5).
//
// Read-only and deliberately separate from the stores that own writes: nothing
// here should ever be able to move points or transition a status, so it has no
// method that could.
type AdminStore struct {
	db *sql.DB
}

// NewAdminStore builds an admin store over conn.
func NewAdminStore(conn *sql.DB) *AdminStore {
	return &AdminStore{db: conn}
}

// Stats is the platform overview from docs/API_CONTRACT.md § Phase 5.
type Stats struct {
	Users          UserStats
	Submissions    SubmissionStats
	Points         PointsStats
	Redemptions    RedemptionStats
	Classification ClassificationStats
}

// UserStats counts accounts by role.
type UserStats struct {
	Total int64

	// ByRole carries every role in the taxonomy, including those with no accounts,
	// so a caller charting this never has to distinguish "zero" from "key absent".
	ByRole map[string]int64
}

// SubmissionStats counts submissions and the weight actually collected.
type SubmissionStats struct {
	Total    int64
	ByStatus map[string]int64

	// TotalVerifiedKg sums verified_qty_kg over verified submissions only — the
	// weight a collector actually measured, not what users estimated. This is the
	// number that can be claimed as diverted from landfill.
	TotalVerifiedKg float64
}

// PointsStats is the ledger, aggregated.
type PointsStats struct {
	// TotalIssued is the sum of positive deltas; TotalSpent the absolute value of
	// the negative ones. Both are read from points_ledger rather than from
	// users.points_balance, because the ledger is the source of truth and this is
	// the view that would expose a drift between them.
	TotalIssued int64
	TotalSpent  int64

	// Outstanding is points earned and not yet spent — the platform's liability.
	// Equal to the sum of every users.points_balance when the ledger and the cache
	// agree, which is what TestStatsPointsMatchBalances asserts.
	Outstanding int64
}

// RedemptionStats counts codes by state.
type RedemptionStats struct {
	Total    int64
	ByStatus map[string]int64
}

// ClassificationStats is the FR-22 payoff: how often the model agreed with the
// human who checked its work.
type ClassificationStats struct {
	// PredictionsMade counts submissions that carry a prediction at all.
	PredictionsMade int64

	// VerifiedAgainst counts those a collector has since verified — the only ones
	// with a trustworthy answer to compare against. A pending submission has no
	// verdict yet, so counting it either way would distort the figure.
	VerifiedAgainst int64

	// Correct counts verified submissions whose prediction survived: the collector
	// looked at the material and did not change material_type away from
	// predicted_category. This works precisely because a correction overwrites
	// material_type and leaves predicted_category alone (FR-22).
	Correct int64

	// Accuracy is Correct / VerifiedAgainst, or nil when nothing has been verified
	// yet. Nil rather than 0.0 on purpose — an untested model is not a model that
	// is wrong every time, and rendering "0% accurate" from an empty database would
	// be the worst possible way to present this feature.
	Accuracy *float64
}

// Stats computes the whole overview.
//
// Several small queries rather than one wide join: each aggregate has its own
// grain (accounts, submissions, ledger rows, codes) and forcing them together
// would multiply rows and silently inflate the counts.
func (s *AdminStore) Stats(ctx context.Context) (*Stats, error) {
	stats := &Stats{}

	var err error
	if stats.Users, err = s.userStats(ctx); err != nil {
		return nil, err
	}
	if stats.Submissions, err = s.submissionStats(ctx); err != nil {
		return nil, err
	}
	if stats.Points, err = s.pointsStats(ctx); err != nil {
		return nil, err
	}
	if stats.Redemptions, err = s.redemptionStats(ctx); err != nil {
		return nil, err
	}
	if stats.Classification, err = s.classificationStats(ctx); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *AdminStore) userStats(ctx context.Context) (UserStats, error) {
	out := UserStats{ByRole: zeroed(
		models.RoleUser, models.RoleCollector, models.RolePartnerStaff, models.RoleAdmin,
	)}

	counts, err := s.countBy(ctx, `SELECT role, COUNT(*) FROM users GROUP BY role`)
	if err != nil {
		return out, fmt.Errorf("count users by role: %w", err)
	}
	for role, count := range counts {
		out.ByRole[role] = count
		out.Total += count
	}
	return out, nil
}

func (s *AdminStore) submissionStats(ctx context.Context) (SubmissionStats, error) {
	out := SubmissionStats{ByStatus: zeroed(
		models.SubmissionPending, models.SubmissionCollected,
		models.SubmissionVerified, models.SubmissionRejected,
	)}

	counts, err := s.countBy(ctx, `SELECT status, COUNT(*) FROM submissions GROUP BY status`)
	if err != nil {
		return out, fmt.Errorf("count submissions by status: %w", err)
	}
	for status, count := range counts {
		out.ByStatus[status] = count
		out.Total += count
	}

	// COALESCE because SUM over no rows is NULL, not 0 — scanning that into a
	// float64 would fail rather than reporting an empty platform.
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(verified_qty_kg), 0) FROM submissions WHERE status = ?`,
		models.SubmissionVerified,
	).Scan(&out.TotalVerifiedKg); err != nil {
		return out, fmt.Errorf("sum verified weight: %w", err)
	}

	// Rounded to one decimal: the underlying column is REAL and summing floats
	// produces things like 214.60000000000002, which reads as a bug in a dashboard.
	out.TotalVerifiedKg = math.Round(out.TotalVerifiedKg*10) / 10
	return out, nil
}

func (s *AdminStore) pointsStats(ctx context.Context) (PointsStats, error) {
	var out PointsStats

	// One pass with conditional sums rather than two queries, so both figures come
	// from the same snapshot of the ledger and cannot disagree.
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN points_delta > 0 THEN points_delta ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN points_delta < 0 THEN -points_delta ELSE 0 END), 0)
		FROM points_ledger`,
	).Scan(&out.TotalIssued, &out.TotalSpent); err != nil {
		return out, fmt.Errorf("sum points ledger: %w", err)
	}

	out.Outstanding = out.TotalIssued - out.TotalSpent
	return out, nil
}

func (s *AdminStore) redemptionStats(ctx context.Context) (RedemptionStats, error) {
	out := RedemptionStats{ByStatus: zeroed(
		models.RedemptionIssued, models.RedemptionUsed, models.RedemptionExpired,
	)}

	counts, err := s.countBy(ctx, `SELECT status, COUNT(*) FROM redemptions GROUP BY status`)
	if err != nil {
		return out, fmt.Errorf("count redemptions by status: %w", err)
	}
	for status, count := range counts {
		out.ByStatus[status] = count
		out.Total += count
	}
	return out, nil
}

// classificationStats measures the model against the collectors who checked it.
func (s *AdminStore) classificationStats(ctx context.Context) (ClassificationStats, error) {
	var out ClassificationStats

	// All three counts in one pass over the same rows. The comparison that matters
	// is predicted_category = material_type *after* verification: the collector
	// corrects material_type and predicted_category is deliberately left alone, so
	// inequality is exactly "the model was wrong and a human said so".
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = ? AND predicted_category = material_type
			                  THEN 1 ELSE 0 END), 0)
		FROM submissions
		WHERE predicted_category IS NOT NULL`,
		models.SubmissionVerified, models.SubmissionVerified,
	).Scan(&out.PredictionsMade, &out.VerifiedAgainst, &out.Correct); err != nil {
		return out, fmt.Errorf("count classification accuracy: %w", err)
	}

	if out.VerifiedAgainst > 0 {
		// Three decimals, matching the contract's worked example. Left nil when there
		// is no denominator rather than dividing by zero into +Inf or reporting 0.0,
		// which would read as "the model is always wrong".
		accuracy := math.Round(float64(out.Correct)/float64(out.VerifiedAgainst)*1000) / 1000
		out.Accuracy = &accuracy
	}
	return out, nil
}

// countBy runs a two-column `SELECT key, COUNT(*) … GROUP BY key` into a map.
func (s *AdminStore) countBy(ctx context.Context, query string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int64{}
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return counts, rows.Err()
}

// zeroed builds a map with every key present at 0.
//
// So that a caller reading stats.ByStatus["collected"] gets 0 rather than having
// to tell a missing key apart from a real zero — the contract's example shows
// every status listed even when none exist.
func zeroed(keys ...string) map[string]int64 {
	out := make(map[string]int64, len(keys))
	for _, key := range keys {
		out[key] = 0
	}
	return out
}
